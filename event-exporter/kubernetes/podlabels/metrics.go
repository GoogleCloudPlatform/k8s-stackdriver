package podlabels

import (
	"time"

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
	prometheus.MustRegister(podGetCount)
	prometheus.MustRegister(noLabelPodCacheOpsCount)
}

func recordEviction(pod cacheKey, labels map[string]string) {
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

func recordPodGet(status string) {
	podGetCount.WithLabelValues(status).Add(1)
}

func recordEmptyLabelPodCacheEvict(pod cacheKey, timestamp time.Time) {
	noLabelPodCacheOpsCount.WithLabelValues("evict").Add(1)
}

func recordEmptyLabelPodCacheAddition() {
	noLabelPodCacheOpsCount.WithLabelValues("add").Add(1)
}

func recordEmptyLabelPodCacheHit() {
	noLabelPodCacheOpsCount.WithLabelValues("queryhit").Add(1)
}

func recordEmptyLabelPodCacheExpire() {
	noLabelPodCacheOpsCount.WithLabelValues("expire").Add(1)
}

func recordEmptyLabelPodCacheMiss() {
	noLabelPodCacheOpsCount.WithLabelValues("querymiss").Add(1)
}
