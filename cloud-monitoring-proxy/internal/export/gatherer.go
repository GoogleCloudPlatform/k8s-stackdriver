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

// Package export serves the cache over HTTP: a prometheus.Gatherer
// producing dto.MetricFamily directly from the store (a scrape never
// triggers GCM calls), plus health endpoints.
package export

import (
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/prometheus/model/labels"
	"google.golang.org/protobuf/proto"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/cloud-monitoring-proxy/internal/cache"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/cloud-monitoring-proxy/internal/translate"
)

// Gatherer adapts the cache to prometheus.Gatherer. Combine with the
// self-metrics registry via prometheus.Gatherers.
type Gatherer struct {
	store *cache.Store
	// emitTimestamps is read per scrape so config hot-reloads apply
	// without restart.
	emitTimestamps func() bool
}

var _ prometheus.Gatherer = (*Gatherer)(nil)

func NewGatherer(store *cache.Store, emitTimestamps func() bool) *Gatherer {
	return &Gatherer{store: store, emitTimestamps: emitTimestamps}
}

func (g *Gatherer) Gather() ([]*dto.MetricFamily, error) {
	withTS := g.emitTimestamps != nil && g.emitTimestamps()
	var out []*dto.MetricFamily
	g.store.Read(func(fams []*cache.Family) {
		out = make([]*dto.MetricFamily, 0, len(fams))
		for _, f := range fams {
			mf := &dto.MetricFamily{
				Name: proto.String(f.Name),
				Help: proto.String(f.Help),
				Type: familyType(f.Kind).Enum(),
			}
			for _, sr := range f.SortedSeries() {
				m := &dto.Metric{Label: labelPairs(sr)}
				switch f.Kind {
				case translate.KindCounter:
					m.Counter = &dto.Counter{Value: proto.Float64(sr.Value)}
				case translate.KindHistogram:
					m.Histogram = histogramDTO(sr.Hist)
				default:
					m.Gauge = &dto.Gauge{Value: proto.Float64(sr.Value)}
				}
				if withTS && !sr.PointEnd.IsZero() {
					m.TimestampMs = proto.Int64(sr.PointEnd.UnixMilli())
				}
				mf.Metric = append(mf.Metric, m)
			}
			if len(mf.Metric) > 0 {
				out = append(out, mf)
			}
		}
	})
	return out, nil
}

func familyType(k translate.Kind) dto.MetricType {
	switch k {
	case translate.KindCounter:
		return dto.MetricType_COUNTER
	case translate.KindHistogram:
		return dto.MetricType_HISTOGRAM
	default:
		return dto.MetricType_GAUGE
	}
}

func labelPairs(sr *cache.Series) []*dto.LabelPair {
	pairs := make([]*dto.LabelPair, 0, sr.Labels.Len())
	sr.Labels.Range(func(l labels.Label) {
		pairs = append(pairs, &dto.LabelPair{
			Name:  proto.String(l.Name),
			Value: proto.String(l.Value),
		})
	})
	return pairs
}

func histogramDTO(h *translate.HistogramValue) *dto.Histogram {
	if h == nil {
		return &dto.Histogram{SampleCount: proto.Uint64(0), SampleSum: proto.Float64(0)}
	}
	buckets := make([]*dto.Bucket, 0, len(h.UpperBounds))
	for i, ub := range h.UpperBounds {
		buckets = append(buckets, &dto.Bucket{
			UpperBound:      proto.Float64(ub),
			CumulativeCount: proto.Uint64(h.CumCounts[i]),
		})
	}
	return &dto.Histogram{
		SampleCount: proto.Uint64(h.Count),
		SampleSum:   proto.Float64(h.Sum),
		Bucket:      buckets,
	}
}
