package stackdriver

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clock "k8s.io/utils/clock/testing"
)

const benchmarkSeriesCount = 100

func BenchmarkProcessCoreV1RepeatedEvents(b *testing.B) {
	factory := newSdLogEntryFactory(clock.NewFakeClock(time.Time{}), newMonitoredResourceFactory(factoryConfig(newTypes)), nil)
	event := &corev1.Event{
		Type:           "Normal",
		InvolvedObject: corev1.ObjectReference{Kind: pod, Name: "bench-pod", Namespace: "default"},
		LastTimestamp:  metav1.NewTime(time.Now()),
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		for j := 0; j < benchmarkSeriesCount; j++ {
			_ = factory.FromEvent(event)
		}
	}
}

func BenchmarkProcessSeriesAggregatedEvent(b *testing.B) {
	factory := newSdLogEntryFactory(clock.NewFakeClock(time.Time{}), newMonitoredResourceFactory(factoryConfig(newTypes)), nil)
	event := &corev1.Event{
		Type:           "Normal",
		InvolvedObject: corev1.ObjectReference{Kind: pod, Name: "bench-pod", Namespace: "default"},
		Series: &corev1.EventSeries{
			Count:            benchmarkSeriesCount,
			LastObservedTime: metav1.NewMicroTime(time.Now()),
		},
	}

	b.ReportAllocs()
	b.ReportMetric(float64(benchmarkSeriesCount), "series_count")
	for i := 0; i < b.N; i++ {
		_ = factory.FromEvent(event)
	}
}
