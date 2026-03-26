package podlabels

import (
	"github.com/prometheus/client_golang/prometheus"
)

var cacheOpsCount = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name:      "cache_ops_count",
		Help:      "Number of operations in the pod label cache",
		Subsystem: "podlabel",
	},
	[]string{"operation"},
)

func init() {
	prometheus.MustRegister(cacheOpsCount)
}

func recordEviction() {
	cacheOpsCount.WithLabelValues("evict").Add(1)
}
func recordAddition() {
	cacheOpsCount.WithLabelValues("add").Add(1)
}

func recordQueryHit() {
	cacheOpsCount.WithLabelValues("queryhit").Add(1)
}

func recordQueryMiss() {
	cacheOpsCount.WithLabelValues("querymiss").Add(1)
}
