package metrics

import (
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestGaugeInt64IsThreadSafe(t *testing.T) {
	metricOptions := DescriptorOpts{
		Name:        "metric",
		Description: "description",
		Unit:        "unit",
		Labels: []LabelDescriptor{{
			Name:        "label",
			Description: "description",
		}},
	}
	goroutines := 10
	got := NewGaugeInt64(metricOptions)

	var wg sync.WaitGroup
	wg.Add(goroutines)
	waitDone := make(chan struct{}, goroutines)
	for i := 0; i < goroutines; i++ {
		i := i // go/nogo-check#loopclosure
		go func(value int64) {
			defer wg.Done()
			// Block this goroutine execution until this channel is closed.
			// We do this in an attempt to unblock all goroutines at once to try
			// to maximize the probability they'll run in parallel.
			<-waitDone
			got.WithLabels(map[string]string{
				fmt.Sprintf("label-%d", i): fmt.Sprintf("%d", i),
			}).Set(value)
		}(int64(i))
	}
	close(waitDone)
	wg.Wait()

	want := NewGaugeInt64(metricOptions)
	for i := 0; i < goroutines; i++ {
		want.WithLabels(map[string]string{
			fmt.Sprintf("label-%d", i): fmt.Sprintf("%d", i),
		}).Set(int64(i))
	}
	options := cmp.Options{
		cmp.AllowUnexported(GaugeInt64{}, gaugeInt64{}, atomic.Int64{}, sync.RWMutex{}),
		cmpopts.IgnoreFields(GaugeInt64{}, "mu"),
	}
	if diff := cmp.Diff(want, got, options); diff != "" {
		t.Errorf("GaugeInt64 returned diff (-want +got):\n%s", diff)
	}
}

func TestGaugeDoubleIsThreadSafe(t *testing.T) {
	metricOptions := DescriptorOpts{
		Name:        "metric",
		Description: "description",
		Unit:        "unit",
		Labels: []LabelDescriptor{{
			Name:        "label",
			Description: "description",
		}},
	}
	goroutines := 10
	got := NewGaugeDouble(metricOptions)

	var wg sync.WaitGroup
	wg.Add(goroutines)
	waitDone := make(chan struct{}, goroutines)
	for i := 0; i < goroutines; i++ {
		i := i // go/nogo-check#loopclosure
		go func(value int64) {
			defer wg.Done()
			// Block this goroutine execution until this channel is closed.
			// We do this in an attempt to unblock all goroutines at once to try
			// to maximize the probability they'll run in parallel.
			<-waitDone
			got.WithLabels(map[string]string{
				fmt.Sprintf("label-%d", i): fmt.Sprintf("%d", i),
			}).Set(float64(value))
		}(int64(i))
	}
	close(waitDone)
	wg.Wait()

	want := NewGaugeDouble(metricOptions)
	for i := 0; i < goroutines; i++ {
		want.WithLabels(map[string]string{
			fmt.Sprintf("label-%d", i): fmt.Sprintf("%d", i),
		}).Set(float64(i))
	}
	options := cmp.Options{
		cmp.AllowUnexported(GaugeDouble{}, gaugeDouble{}, sync.RWMutex{}),
		cmpopts.IgnoreFields(GaugeDouble{}, "mu"),
		cmp.Comparer(func(x, y float64) bool {
			return math.Abs(x-y) < 0.001
		}),
	}
	if diff := cmp.Diff(want, got, options); diff != "" {
		t.Errorf("GaugeDouble returned diff (-want +got):\n%s", diff)
	}
}
