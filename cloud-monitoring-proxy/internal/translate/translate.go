/*
Copyright 2026 Google Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package translate

import (
	"time"

	"github.com/prometheus/prometheus/model/labels"

	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"google.golang.org/genproto/googleapis/api/metric"
)

// Kind is the Prometheus family type a metric translates to.
type Kind int

const (
	KindGauge Kind = iota
	KindCounter
	KindHistogram
)

// HistogramValue is a classic cumulative Prometheus histogram.
type HistogramValue struct {
	Count uint64
	Sum   float64
	// UpperBounds are the finite bucket bounds; CumCounts[i] is the
	// cumulative count for le=UpperBounds[i]. The +Inf bucket is Count.
	UpperBounds []float64
	CumCounts   []uint64
}

// Sample is one translated series value.
type Sample struct {
	Labels labels.Labels
	Value  float64
	Hist   *HistogramValue // non-nil iff the family is KindHistogram
	// PointEnd is the GCM point's interval end (the honest timestamp).
	PointEnd time.Time
}

// FamilyUpdate is the result of translating one metric type's poll.
type FamilyUpdate struct {
	Name    string
	Help    string
	Kind    Kind
	Samples []Sample
}

// Stats counts what a Translate call had to drop or reset.
type Stats struct {
	// DroppedValueType counts series with STRING/MONEY/unsupported values.
	DroppedValueType int
	// NoPoints counts series that came back with zero points.
	NoPoints int
	// BucketReset counts DELTA distribution series whose bucket layout
	// changed mid-flight (accumulation restarted).
	BucketReset int
}

// Translator converts polled series and owns DELTA accumulation state.
// Not goroutine-safe: the poller serializes calls per Translator.
type Translator struct {
	deltas map[deltaKey]*deltaState
}

type deltaKey struct {
	metricType string
	labelsHash uint64
}

type deltaState struct {
	lastEnd  time.Time
	lastSeen time.Time
	sum      float64
	hist     *histAccum
}

func New() *Translator {
	return &Translator{deltas: map[deltaKey]*deltaState{}}
}

// Translate converts one metric type's series. rename overrides the
// converted name when non-empty.
func (t *Translator) Translate(md *metric.MetricDescriptor, series []*monitoringpb.TimeSeries, rename string, now time.Time) (FamilyUpdate, Stats) {
	name := rename
	if name == "" {
		name = PromName(md.GetType())
	}
	fam := FamilyUpdate{
		Name: name,
		Help: md.GetDescription(),
		Kind: familyKind(md),
	}
	var st Stats

	for _, ts := range series {
		if len(ts.GetPoints()) == 0 {
			st.NoPoints++
			continue
		}
		if !supportedValueType(ts, md) {
			st.DroppedValueType++
			continue
		}
		lset := buildLabels(ts)
		sample, ok := t.translateSeries(md, ts, lset, now, &st)
		if !ok {
			continue
		}
		fam.Samples = append(fam.Samples, sample)
	}
	return fam, st
}

// SweepStale drops DELTA accumulation state not seen since the cutoff, so
// series that disappear from GCM don't leak state. Returns entries removed.
func (t *Translator) SweepStale(cutoff time.Time) int {
	removed := 0
	for k, s := range t.deltas {
		if s.lastSeen.Before(cutoff) {
			delete(t.deltas, k)
			removed++
		}
	}
	return removed
}

func familyKind(md *metric.MetricDescriptor) Kind {
	if md.GetValueType() == metric.MetricDescriptor_DISTRIBUTION {
		return KindHistogram
	}
	switch md.GetMetricKind() {
	case metric.MetricDescriptor_CUMULATIVE, metric.MetricDescriptor_DELTA:
		return KindCounter
	default:
		return KindGauge
	}
}

func supportedValueType(ts *monitoringpb.TimeSeries, md *metric.MetricDescriptor) bool {
	vt := ts.GetValueType()
	if vt == metric.MetricDescriptor_VALUE_TYPE_UNSPECIFIED {
		vt = md.GetValueType()
	}
	switch vt {
	case metric.MetricDescriptor_BOOL, metric.MetricDescriptor_INT64,
		metric.MetricDescriptor_DOUBLE, metric.MetricDescriptor_DISTRIBUTION:
		return true
	default:
		return false
	}
}

// translateSeries produces the sample for one series. Points arrive most
// recent first (ListTimeSeries contract).
func (t *Translator) translateSeries(md *metric.MetricDescriptor, ts *monitoringpb.TimeSeries, lset labels.Labels, now time.Time, st *Stats) (Sample, bool) {
	points := ts.GetPoints()
	latest := points[0]
	end := latest.GetInterval().GetEndTime().AsTime()
	isDelta := effectiveKind(ts, md) == metric.MetricDescriptor_DELTA
	isDist := ts.GetValueType() == metric.MetricDescriptor_DISTRIBUTION ||
		(ts.GetValueType() == metric.MetricDescriptor_VALUE_TYPE_UNSPECIFIED && md.GetValueType() == metric.MetricDescriptor_DISTRIBUTION)

	switch {
	case isDist && isDelta:
		hv, ok := t.accumulateDeltaDist(md.GetType(), lset, points, now, st)
		if !ok {
			return Sample{}, false
		}
		return Sample{Labels: lset, Hist: hv, PointEnd: end}, true

	case isDist:
		hv := distToHistogram(latest.GetValue().GetDistributionValue())
		if hv == nil {
			st.DroppedValueType++
			return Sample{}, false
		}
		return Sample{Labels: lset, Hist: hv, PointEnd: end}, true

	case isDelta:
		state := t.state(md.GetType(), lset, now)
		// Accumulate unseen points oldest→newest.
		for i := len(points) - 1; i >= 0; i-- {
			pEnd := points[i].GetInterval().GetEndTime().AsTime()
			if !pEnd.After(state.lastEnd) {
				continue
			}
			v, ok := scalarValue(points[i])
			if !ok {
				st.DroppedValueType++
				return Sample{}, false
			}
			state.sum += v
			state.lastEnd = pEnd
		}
		return Sample{Labels: lset, Value: state.sum, PointEnd: state.lastEnd}, true

	default: // GAUGE / CUMULATIVE scalar: latest point as-is.
		v, ok := scalarValue(latest)
		if !ok {
			st.DroppedValueType++
			return Sample{}, false
		}
		return Sample{Labels: lset, Value: v, PointEnd: end}, true
	}
}

func effectiveKind(ts *monitoringpb.TimeSeries, md *metric.MetricDescriptor) metric.MetricDescriptor_MetricKind {
	if k := ts.GetMetricKind(); k != metric.MetricDescriptor_METRIC_KIND_UNSPECIFIED {
		return k
	}
	return md.GetMetricKind()
}

func (t *Translator) state(metricType string, lset labels.Labels, now time.Time) *deltaState {
	k := deltaKey{metricType: metricType, labelsHash: lset.Hash()}
	s, ok := t.deltas[k]
	if !ok {
		s = &deltaState{}
		t.deltas[k] = s
	}
	s.lastSeen = now
	return s
}

func scalarValue(p *monitoringpb.Point) (float64, bool) {
	switch v := p.GetValue().GetValue().(type) {
	case *monitoringpb.TypedValue_DoubleValue:
		return v.DoubleValue, true
	case *monitoringpb.TypedValue_Int64Value:
		return float64(v.Int64Value), true
	case *monitoringpb.TypedValue_BoolValue:
		if v.BoolValue {
			return 1, true
		}
		return 0, true
	default:
		return 0, false
	}
}
