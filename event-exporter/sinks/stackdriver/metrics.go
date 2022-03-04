package stackdriver

import (
	"time"

	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
	sd "google.golang.org/api/logging/v2"
)

var (
	receivedEntryCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name:      "received_entry_count",
			Help:      "Number of entries received by the Stackdriver sink",
			Subsystem: "stackdriver_sink",
		},
	)

	requestCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      "request_count",
			Help:      "Number of request, issued to Stackdriver API",
			Subsystem: "stackdriver_sink",
		},
		[]string{"code"},
	)

	successfullySentEntryCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name:      "successfully_sent_entry_count",
			Help:      "Number of entries successfully ingested by Stackdriver",
			Subsystem: "stackdriver_sink",
		},
	)

	recordLatency = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "records_latency_seconds",
			Help:    "Log entry latency between log timestamp and delivery to StackDriver.",
			Subsystem: "stackdriver_sink",
			Buckets: prometheus.ExponentialBuckets(2, 1.4, 13),
		},
	)
)

func init() {
	prometheus.MustRegister(
		receivedEntryCount,
		successfullySentEntryCount,
		requestCount,
		recordLatency,
	)
}

func measureLatencyOnSuccess(entries []*sd.LogEntry) {
	samples := make([]time.Time, 0)
	for _, e := range entries {
		// Do not measure latency if a log entry does not have a timestamp.
		if e.Timestamp == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339Nano, e.Timestamp)
		if err != nil {
			glog.Warningf("Failed to parse timestamp: %s", e.Timestamp)
			continue
		}
		samples = append(samples, t)
	}
	now := time.Now()
	for _, ts := range samples {
		recordLatency.Observe(now.Sub(ts).Seconds())
	}
}
