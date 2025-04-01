package podlabels

import (
	"context"
	"strings"
	"time"

	"github.com/golang/glog"
	lru "github.com/hashicorp/golang-lru/v2"
	"golang.org/x/time/rate"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var (
	getPodErrLogRateLimiter = rate.NewLimiter(rate.Every(10*time.Second), 1)
)

type cacheKey struct {
	Namespace string
	Name      string
}

// PodLabelCollector defines the interface for GetLabels for a namespace and pod name pair.
type PodLabelCollector interface {
	GetLabels(namespace, pod string) map[string]string
}

// podLabelCollectorWithCache implements PodLabelCollector interface with an internal pod label cache.
type podLabelCollectorWithCache struct {
	ignoredNamespaces     map[string]struct{}
	client                kubernetes.Interface
	getPodTimeout         time.Duration
	cache                 *lru.Cache[cacheKey, map[string]string]
	emptyLabelPodCache    *lru.Cache[cacheKey, time.Time] // cache the timestamp so we can check TTL.
	emptyLabelPodCacheTTL time.Duration
}

// NewCollector returns a new podLabelCollectorWithCache that maintains a set of (podName, namespace) -> (podLabels) mappings.
func NewCollector(client kubernetes.Interface, ignoredNamespaces []string, podCacheSize, emptyLabelPodCacheSize int, emptyLabelPodCacheTTL, getPodTimeout time.Duration) (*podLabelCollectorWithCache, error) {
	cache, err := lru.NewWithEvict[cacheKey, map[string]string](podCacheSize, recordEviction)
	if err != nil {
		return nil, err
	}
	emptyLabelPodCache, err := lru.NewWithEvict[cacheKey, time.Time](emptyLabelPodCacheSize, recordEmptyLabelPodCacheEvict)
	if err != nil {
		return nil, err
	}
	pc := &podLabelCollectorWithCache{
		client:                client,
		getPodTimeout:         getPodTimeout,
		ignoredNamespaces:     make(map[string]struct{}),
		cache:                 cache,
		emptyLabelPodCache:    emptyLabelPodCache,
		emptyLabelPodCacheTTL: emptyLabelPodCacheTTL,
	}
	for _, ns := range ignoredNamespaces {
		pc.ignoredNamespaces[ns] = struct{}{}
	}

	return pc, nil
}

// GetLabels returns the labels for a given Pod. If there's no local cache, we will attempt a
// get from apiserver.
func (pc *podLabelCollectorWithCache) GetLabels(namespaceName, podName string) map[string]string {
	if _, ok := pc.ignoredNamespaces[namespaceName]; ok {
		return nil
	}
	podID := cacheKey{Namespace: namespaceName, Name: podName}
	if labels, ok := pc.cache.Get(podID); ok {
		recordQueryHit()
		return labels
	}
	recordQueryMiss()
	// For pod with empty labels, return empty label directly.
	if timestamp, ok := pc.emptyLabelPodCache.Get(podID); ok {
		if timestamp.Add(pc.emptyLabelPodCacheTTL).After(time.Now()) {
			recordEmptyLabelPodCacheHit()
			return nil
		} else {
			recordEmptyLabelPodCacheExpire()
			pc.emptyLabelPodCache.Remove(podID)
		}
	} else {
		recordEmptyLabelPodCacheMiss()
	}
	ctx, cancel := context.WithTimeout(context.TODO(), pc.getPodTimeout)
	pod, err := pc.client.CoreV1().Pods(namespaceName).Get(ctx, podName, metav1.GetOptions{})
	cancel()
	if err != nil {
		if getPodErrLogRateLimiter.Allow() {
			glog.Errorf("Failed to get pod %s/%s %v", namespaceName, podName, err)
		}
		if statusError, isStatus := err.(*apierrors.StatusError); isStatus {
			recordPodGet(string(statusError.ErrStatus.Reason))
			if apierrors.IsNotFound(statusError) {
				recordEmptyLabelPodCacheAddition()
				pc.emptyLabelPodCache.Add(podID, time.Now())
			}
		} else {
			recordPodGet("UnknownFailure")
		}
		return nil
	}
	recordPodGet("OK")
	labels := getLabelsFromPod(pod)
	if len(labels) > 0 {
		pc.cache.Add(podID, labels)
		recordAddition()
		return labels
	}
	recordEmptyLabelPodCacheAddition()
	pc.emptyLabelPodCache.Add(podID, time.Now())
	return nil
}

// get owner labels for go/gke-log-owner-label for non-system logs.
// If there are multiple owners, the loop below picks the last valid one.
func getLabelsFromPod(pod *corev1.Pod) map[string]string {
	transformedLabels := map[string]string{}
	for _, owner := range pod.GetObjectMeta().GetOwnerReferences() {
		switch owner.Kind {
		case "DaemonSet":
			transformedLabels[ownerTypeKeyName] = "DaemonSet"
			transformedLabels[ownerNameKeyName] = owner.Name
		case "StatefulSet":
			transformedLabels[ownerTypeKeyName] = "StatefulSet"
			transformedLabels[ownerNameKeyName] = owner.Name
		case "ReplicaSet":
			// Pod that is eventually owned by Deployment has pod name:
			// <DeploymentName>-<PodTemplateHash>-<RandomString>
			// and owner replicaset name: <DeploymentName>-<PodTemplateHash>
			if templateHashSuffix := "-" + pod.GetObjectMeta().GetLabels()["pod-template-hash"]; len(templateHashSuffix) > 1 {
				if ownerName, ok := strings.CutSuffix(owner.Name, templateHashSuffix); ok {
					transformedLabels[ownerTypeKeyName] = "Deployment"
					transformedLabels[ownerNameKeyName] = ownerName
				}
			}
		case "Job":
			if ownerName, ok := stripUnixTimeSuffix(owner.Name); ok {
				// Pod that is eventually owned by a CronJob has pod name:
				// <CronJobName>-<UnixTimeInMin>-<RandomString>
				// and owner job name: <CronJobName>-<UnixTimeInMin>
				transformedLabels[ownerTypeKeyName] = "CronJob"
				transformedLabels[ownerNameKeyName] = ownerName
			} else if jobsetName := pod.GetObjectMeta().GetLabels()[jobSetNameLabelKey]; jobsetName != "" {
				// Pod that is eventually owned by a JobSet has the jobset name label set.
				transformedLabels[ownerTypeKeyName] = "JobSet"
				transformedLabels[ownerNameKeyName] = jobsetName
			} else {
				transformedLabels[ownerTypeKeyName] = "Job"
				transformedLabels[ownerNameKeyName] = owner.Name
			}
		}
	}
	return transformedLabels
}
