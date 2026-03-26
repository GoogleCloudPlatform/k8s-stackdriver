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
			Name:      "records_latency_seconds",
			Help:      "Log entry latency between log timestamp and delivery to StackDriver.",
			Subsystem: "stackdriver_sink",
			// Highest bucket start at 2 sec * 1.5^19 = 4433.68 sec
			Buckets: prometheus.ExponentialBuckets(2, 1.5, 20),
		},
	)

	droppedByHashCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name:      "events_dropped_by_hash_total",
			Help:      "Number of events skipped because another pod owns them (hash partitioning)",
			Subsystem: "event_exporter",
		},
	)

	ownedEntryCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name:      "events_owned_total",
			Help:      "Number of events this pod owns and processes (hash partitioning)",
			Subsystem: "event_exporter",
		},
	)

	queueDepth = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name:      "queue_depth",
			Help:      "Current number of entries in the log entry channel",
			Subsystem: "event_exporter",
		},
	)
)

func init() {
	prometheus.MustRegister(
		receivedEntryCount,
		successfullySentEntryCount,
		requestCount,
		recordLatency,
		droppedByHashCount,
		ownedEntryCount,
		queueDepth,
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
