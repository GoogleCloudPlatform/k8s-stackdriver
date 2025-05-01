package metrics

import (
	"math/rand"
	"reflect"
	"testing"
	"time"

	cpb "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	mgrpb "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"github.com/google/go-cmp/cmp"
	"github.com/google/gofuzz"
	mdrpb "google.golang.org/genproto/googleapis/api/monitoredres"
	"google.golang.org/protobuf/testing/protocmp"
	tpb "google.golang.org/protobuf/types/known/timestamppb"
)

const (
	testMetricName        = "test-metric-name"
	testMetricDescription = "test-metric-description"
)

var (
	testStartTime = time.Date(2021, time.January, 1, 0, 0, 0, 0, time.UTC)
	testEndTime   = time.Date(2022, time.January, 1, 0, 0, 0, 0, time.UTC)
)

func TestDescriptorOptsEqual(t *testing.T) {
	for _, tc := range []struct {
		desc string
		a, b *DescriptorOpts
		want bool
	}{
		{
			desc: "nil vs nil",
			want: true,
		},
		{
			desc: "nil vs non-nil",
			a:    &DescriptorOpts{},
			want: false,
		},
		{
			desc: "empty vs empty",
			a:    &DescriptorOpts{},
			b:    &DescriptorOpts{},
			want: true,
		},
		{
			desc: "name mismatch",
			a: &DescriptorOpts{
				Name: "foo",
			},
			b: &DescriptorOpts{
				Name: "bar",
			},
			want: false,
		},
		{
			desc: "description mismatch",
			a: &DescriptorOpts{
				Description: "foo",
			},
			b: &DescriptorOpts{
				Description: "bar",
			},
			want: false,
		},
		{
			desc: "unit mismatch",
			a: &DescriptorOpts{
				Unit: "foo",
			},
			b: &DescriptorOpts{
				Unit: "bar",
			},
			want: false,
		},
		{
			desc: "labels length mismatch",
			a: &DescriptorOpts{
				Labels: []LabelDescriptor{{}},
			},
			b: &DescriptorOpts{
				Labels: []LabelDescriptor{{}, {}},
			},
			want: false,
		},
		{
			desc: "labels name mismatch",
			a: &DescriptorOpts{
				Labels: []LabelDescriptor{
					{Name: "foo", Description: "foo label"},
				},
			},
			b: &DescriptorOpts{
				Labels: []LabelDescriptor{
					{Name: "bar", Description: "foo label"},
				},
			},
			want: false,
		},
		{
			desc: "labels description mismatch",
			a: &DescriptorOpts{
				Labels: []LabelDescriptor{
					{Name: "foo", Description: "foo label"},
				},
			},
			b: &DescriptorOpts{
				Labels: []LabelDescriptor{
					{Name: "foo", Description: "bar label"},
				},
			},
			want: false,
		},
		{
			desc: "worked successful match",
			a: &DescriptorOpts{
				Name:        "foo",
				Description: "a metric about foo",
				Unit:        "foos/sec",
				Labels: []LabelDescriptor{
					{Name: "label1", Description: "the first label"},
					{Name: "label2", Description: "the second label"},
					{Name: "label3", Description: "the third label"},
				},
			},
			b: &DescriptorOpts{
				Name:        "foo",
				Description: "a metric about foo",
				Unit:        "foos/sec",
				Labels: []LabelDescriptor{
					// Different label order:
					{Name: "label1", Description: "the first label"},
					{Name: "label3", Description: "the third label"},
					{Name: "label2", Description: "the second label"},
				},
			},
			want: true,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			got := tc.a.Equal(tc.b)
			if got != tc.want {
				t.Errorf("got equal = %v want %v", got, tc.want)
			}
			reverse := tc.b.Equal(tc.a)
			if reverse != got {
				t.Errorf("not commutative: a.Equal(b)=%v yet b.Equal(a)=%v", got, reverse)
			}
		})
	}
}

func TestDescriptorOptsEqualFuzz(t *testing.T) {
	var a, b DescriptorOpts
	randSeed := time.Now().UnixNano()

	hasUniqueLabels := func(d *DescriptorOpts) bool {
		// Check if the labels have unique names.
		// If they don't, this isn't a valid metric descriptor.
		labelNames := make(map[string]struct{}, len(a.Labels))
		for _, label := range a.Labels {
			if _, duplicate := labelNames[label.Name]; duplicate {
				return false
			}
			labelNames[label.Name] = struct{}{}
		}
		return true
	}

	t.Run("equality", func(t *testing.T) {
		fuzzerA := fuzz.NewWithSeed(randSeed)
		fuzzerB := fuzz.NewWithSeed(randSeed)
		for i, checked := 0, 0; checked < 1000; i++ {
			// Since the two fuzzers were initialized from the same random seed, they should
			// generate exactly the same struct as each other.
			fuzzerA.Fuzz(&a)
			fuzzerB.Fuzz(&b)
			if !hasUniqueLabels(&a) {
				continue
			}

			if !a.Equal(&b) {
				t.Errorf("DescriptorOpts (seed %d, iteration %d): (%+v).Equal(%+v) does not hold", randSeed, i, a, b)
			}
			checked++
		}
	})

	t.Run("inequality", func(t *testing.T) {
		fuzzerA := fuzz.NewWithSeed(randSeed)
		fuzzerB := fuzz.NewWithSeed(randSeed + 1)
		for i, checked := 0, 0; checked < 1000; i++ {
			fuzzerA.Fuzz(&a)
			fuzzerB.Fuzz(&b)
			if !hasUniqueLabels(&a) || !hasUniqueLabels(&b) {
				continue
			}
			if reflect.DeepEqual(&a, &b) {
				// Two different fuzzers managed to create the exact same struct.
				continue
			}
			if a.Equal(&b) {
				t.Errorf("DescriptorOpts (seed %d, iteration %d): (%+v).Equal(%+v) unexpectedly holds", randSeed, i, a, b)
			}
			checked++
		}
	})
}

func TestCreateGaugePoint(t *testing.T) {
	typedValue := &cpb.TypedValue{
		Value: &cpb.TypedValue_Int64Value{
			Int64Value: 1,
		},
	}
	want := &mgrpb.Point{
		Interval: &cpb.TimeInterval{
			EndTime: tpb.New(testEndTime),
		},
		Value: typedValue,
	}
	got := CreateGaugePoint(typedValue, testEndTime)
	if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
		t.Errorf("createGaugePoint() returned diff (-want +got):\n%s", diff)
	}
}

func TestCreateCumulativePoint(t *testing.T) {
	typedValue := &cpb.TypedValue{
		Value: &cpb.TypedValue_Int64Value{
			Int64Value: 1,
		},
	}
	want := &mgrpb.Point{
		Interval: &cpb.TimeInterval{
			StartTime: tpb.New(testStartTime),
			EndTime:   tpb.New(testEndTime),
		},
		Value: typedValue,
	}
	got := CreateCumulativePoint(typedValue, testStartTime, testEndTime)
	if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
		t.Errorf("createCumulativePoint() returned diff (-want +got):\n%s", diff)
	}
}

func TestCreateDeltaPoint(t *testing.T) {
	typedValue := &cpb.TypedValue{
		Value: &cpb.TypedValue_Int64Value{
			Int64Value: 1,
		},
	}
	want := &mgrpb.Point{
		Interval: &cpb.TimeInterval{
			StartTime: tpb.New(testStartTime),
			EndTime:   tpb.New(testEndTime),
		},
		Value: typedValue,
	}
	got := CreateDeltaPoint(typedValue, testStartTime, testEndTime)
	if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
		t.Errorf("CreateDeltaPoint() returned diff (-want +got):\n%s", diff)
	}
}

func TestCreateMonitoredResource(t *testing.T) {
	resource := NewK8sNode(testProjectName, testLocation, testCluster, testNode)
	want := &mdrpb.MonitoredResource{
		Type: string(k8sNode),
		Labels: map[string]string{
			"project_id":   testProjectName,
			"location":     testLocation,
			"cluster_name": testCluster,
			"node_name":    testNode,
		},
	}
	got := CreateMonitoredResource(resource)
	if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
		t.Errorf("createMonitoredResource() returned diff (-want +got):\n%s", diff)
	}
}

func TestCreateDoubleTypedValue(t *testing.T) {
	value := float64(1)
	want := &cpb.TypedValue{
		Value: &cpb.TypedValue_DoubleValue{
			DoubleValue: value,
		},
	}
	got := CreateDoubleTypedValue(value)
	if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
		t.Errorf("createDoubleTypedValue() returned diff (-want +got):\n%s", diff)
	}
}

func TestCreateInt64TypedValue(t *testing.T) {
	value := int64(1)
	want := &cpb.TypedValue{
		Value: &cpb.TypedValue_Int64Value{
			Int64Value: value,
		},
	}
	got := CreateInt64TypedValue(value)
	if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
		t.Errorf("createInt64TypedValue() returned diff (-want +got):\n%s", diff)
	}
}

func BenchmarkHashLabels(b *testing.B) {
	const (
		numLabels    = 10000
		maxMapSize   = 16
		maxKeySize   = 24
		maxValueSize = 64
	)
	labels := make([]map[string]string, 0, numLabels+2)
	labels = append(labels, nil)
	labels = append(labels, map[string]string{})
	rng := rand.New(rand.NewSource(42))
	for i := 0; i < numLabels; i++ {
		mapSize := rng.Intn(maxMapSize)
		m := make(map[string]string, mapSize)
		for j := 0; j < mapSize; j++ {
			key := make([]byte, rng.Intn(maxKeySize))
			if _, err := rng.Read(key); err != nil {
				b.Fatalf("Cannot create random byte string: %v", err)
			}
			val := make([]byte, rng.Intn(maxValueSize))
			if _, err := rng.Read(val); err != nil {
				b.Fatalf("Cannot create random byte string: %v", err)
			}
			m[string(key)] = string(val)
		}
		labels = append(labels, m)
	}
	var x uint64
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		x ^= hashLabels(labels[i%len(labels)])
	}
	b.StopTimer()
	b.Logf("Ran %d iterations (cumulative hash: %v)", b.N, x)
}
