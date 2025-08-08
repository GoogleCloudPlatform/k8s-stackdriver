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

var noLabelPodCacheOpsCount = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name:      "nolabel_pod_cache_ops_count",
		Help:      "Number of operations in the cache for pods with empty labels",
		Subsystem: "podlabel",
	},
	[]string{"operation"},
)

var podGetCount = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name:      "get_count",
		Help:      "Number of get pod requests to apiserver",
		Subsystem: "podlabel",
	},
	[]string{"status"},
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
