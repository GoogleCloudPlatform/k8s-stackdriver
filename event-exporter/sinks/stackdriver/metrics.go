package stackdriver

import "github.com/prometheus/client_golang/prometheus"

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
)

func init() {
	prometheus.MustRegister(
		receivedEntryCount,
		successfullySentEntryCount,
		requestCount,
	)
}
