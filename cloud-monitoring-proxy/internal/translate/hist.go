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
	"math"
	"slices"
	"time"

	"github.com/prometheus/prometheus/model/labels"

	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"google.golang.org/genproto/googleapis/api/distribution"
)

// distBounds extracts the finite upper bounds from GCM bucket options.
// GCM bucket i spans [lower_i, upper_i): bucket 0 is the underflow bucket
// (-Inf, bounds[0]); the last bucket is the overflow [bounds[n-1], +Inf).
func distBounds(opts *distribution.Distribution_BucketOptions) []float64 {
	switch o := opts.GetOptions().(type) {
	case *distribution.Distribution_BucketOptions_ExplicitBuckets:
		return o.ExplicitBuckets.GetBounds()
	case *distribution.Distribution_BucketOptions_LinearBuckets:
		n := int(o.LinearBuckets.GetNumFiniteBuckets())
		bounds := make([]float64, 0, n+1)
		for i := 0; i <= n; i++ {
			bounds = append(bounds, o.LinearBuckets.GetOffset()+float64(i)*o.LinearBuckets.GetWidth())
		}
		return bounds
	case *distribution.Distribution_BucketOptions_ExponentialBuckets:
		n := int(o.ExponentialBuckets.GetNumFiniteBuckets())
		bounds := make([]float64, 0, n+1)
		for i := 0; i <= n; i++ {
			bounds = append(bounds, o.ExponentialBuckets.GetScale()*math.Pow(o.ExponentialBuckets.GetGrowthFactor(), float64(i)))
		}
		return bounds
	default:
		return nil
	}
}

// rawBucketCounts returns per-bucket counts padded/truncated to
// len(bounds)+1 (underflow+finite buckets, then overflow at the end). GCM
// may trim trailing zero buckets.
func rawBucketCounts(d *distribution.Distribution, nBuckets int) []uint64 {
	counts := make([]uint64, nBuckets)
	for i, c := range d.GetBucketCounts() {
		if i >= nBuckets {
			break
		}
		counts[i] = uint64(c)
	}
	return counts
}

// distToHistogram converts one (cumulative-in-time or instantaneous)
// distribution point to a Prometheus histogram. GCM distributions carry
// mean rather than sum, so _sum = mean × count.
func distToHistogram(d *distribution.Distribution) *HistogramValue {
	if d == nil {
		return nil
	}
	bounds := distBounds(d.GetBucketOptions())
	if bounds == nil {
		return nil
	}
	// nBuckets = underflow + finite buckets; overflow is implied by Count.
	raw := rawBucketCounts(d, len(bounds)+1)
	hv := &HistogramValue{
		Count:       uint64(d.GetCount()),
		Sum:         d.GetMean() * float64(d.GetCount()),
		UpperBounds: bounds,
		CumCounts:   make([]uint64, len(bounds)),
	}
	var cum uint64
	for i := range bounds {
		cum += raw[i] // raw[i] counts values below bounds[i]
		hv.CumCounts[i] = cum
	}
	return hv
}

// histAccum accumulates DELTA distribution points into a running histogram.
type histAccum struct {
	bounds []float64
	raw    []uint64 // per-bucket, len(bounds)+1 (underflow + finite)
	count  uint64
	sum    float64
}

// accumulateDeltaDist folds unseen delta-distribution points into the
// series' accumulator and returns the current cumulative histogram.
func (t *Translator) accumulateDeltaDist(metricType string, lset labels.Labels, points []*monitoringpb.Point, now time.Time, st *Stats) (*HistogramValue, bool) {
	state := t.state(metricType, lset, now)
	for i := len(points) - 1; i >= 0; i-- {
		pEnd := points[i].GetInterval().GetEndTime().AsTime()
		if !pEnd.After(state.lastEnd) {
			continue
		}
		d := points[i].GetValue().GetDistributionValue()
		bounds := distBounds(d.GetBucketOptions())
		if bounds == nil {
			st.DroppedValueType++
			return nil, false
		}
		if state.hist == nil || !slices.Equal(state.hist.bounds, bounds) {
			if state.hist != nil {
				st.BucketReset++
			}
			state.hist = &histAccum{bounds: bounds, raw: make([]uint64, len(bounds)+1)}
		}
		raw := rawBucketCounts(d, len(bounds)+1)
		for j, c := range raw {
			state.hist.raw[j] += c
		}
		state.hist.count += uint64(d.GetCount())
		state.hist.sum += d.GetMean() * float64(d.GetCount())
		state.lastEnd = pEnd
	}
	if state.hist == nil {
		return nil, false
	}
	hv := &HistogramValue{
		Count:       state.hist.count,
		Sum:         state.hist.sum,
		UpperBounds: state.hist.bounds,
		CumCounts:   make([]uint64, len(state.hist.bounds)),
	}
	var cum uint64
	for i := range state.hist.bounds {
		cum += state.hist.raw[i]
		hv.CumCounts[i] = cum
	}
	return hv, true
}
