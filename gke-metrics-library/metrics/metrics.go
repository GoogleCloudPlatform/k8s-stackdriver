// Package metrics contains methods for building Cloud Monitoring TimeSeries.
package metrics

import (
	"time"

	cpb "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	mgrpb "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	mdrpb "google.golang.org/genproto/googleapis/api/monitoredres"
	tpb "google.golang.org/protobuf/types/known/timestamppb"
)

// DescriptorOpts contains options for different metric descriptors.
type DescriptorOpts struct {
	// Name of metric, example: "kubernetes.io/node/cpu/total_cores"
	Name string
	// Description of metric
	Description string
	// Unit of metric, for detailed explanation, check `Unit` field of
	// http://godoc/3/google/api/metric_go_proto#MetricDescriptor
	Unit string
	// TODO(b/262585915): add schema field, validate schema and labels when creating timeseries.
	Labels []LabelDescriptor
}

// Equal returns whether this descriptor is the same as another.
func (d *DescriptorOpts) Equal(other *DescriptorOpts) bool {
	if d == nil && other == nil {
		return true
	}
	if d == nil || other == nil {
		return false
	}
	if d.Name != other.Name {
		return false
	}
	if d.Description != other.Description {
		return false
	}
	if d.Unit != other.Unit {
		return false
	}
	return labelDescriptorSlicesEqual(d.Labels, other.Labels)
}

// LabelDescriptor contains the label information for metric descriptors.
type LabelDescriptor struct {
	Name        string
	Description string
}

// labelDescriptorSlicesEqual returns true if two slices of label descriptors
// are equal, irrespective of ordering.
// Each slice is assumed to have unique label names.
func labelDescriptorSlicesEqual(a, b []LabelDescriptor) bool {
	if len(a) != len(b) {
		return false
	}
	// To avoid allocating two throwaway maps and slices, use brute-force check.
	// This is O(n^2), but metrics typically only have a handful of labels.
	for _, l1 := range a {
		found := false
		for _, l2 := range b {
			if l1.Name != l2.Name {
				continue
			}
			if l1.Description != l2.Description {
				return false
			}
			found = true
			break
		}
		if !found {
			return false
		}
	}
	return true
}

// CreateGaugePoint creates a GAUGE metric point.
func CreateGaugePoint(value *cpb.TypedValue, endTime time.Time) *mgrpb.Point {
	return &mgrpb.Point{
		Interval: &cpb.TimeInterval{
			EndTime: tpb.New(endTime),
		},
		Value: value,
	}
}

// CreateCumulativePoint creates a CUMULATIVE metric point.
func CreateCumulativePoint(value *cpb.TypedValue, startTime, endTime time.Time) *mgrpb.Point {
	return &mgrpb.Point{
		Interval: &cpb.TimeInterval{
			StartTime: tpb.New(startTime),
			EndTime:   tpb.New(endTime),
		},
		Value: value,
	}
}

// CreateDeltaPoint creates a DELTA metric point.
func CreateDeltaPoint(value *cpb.TypedValue, startTime, endTime time.Time) *mgrpb.Point {
	return &mgrpb.Point{
		Interval: &cpb.TimeInterval{
			StartTime: tpb.New(startTime),
			EndTime:   tpb.New(endTime),
		},
		Value: value,
	}
}

// CreateMonitoredResource creates a monitored resource from Resource.
func CreateMonitoredResource(resource Resource) *mdrpb.MonitoredResource {
	return &mdrpb.MonitoredResource{
		Type:   string(resource.schema),
		Labels: resource.labels,
	}
}

// CreateDoubleTypedValue creates a Double TypedValue.
func CreateDoubleTypedValue(value float64) *cpb.TypedValue {
	return &cpb.TypedValue{
		Value: &cpb.TypedValue_DoubleValue{
			DoubleValue: value,
		},
	}
}

// CreateInt64TypedValue creates an Int64 TypedValue.
func CreateInt64TypedValue(value int64) *cpb.TypedValue {
	return &cpb.TypedValue{
		Value: &cpb.TypedValue_Int64Value{
			Int64Value: value,
		},
	}
}
