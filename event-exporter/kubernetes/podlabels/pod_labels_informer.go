package podlabels

import (
	"runtime"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientfeatures "k8s.io/client-go/features"
	"k8s.io/client-go/metadata"

	// "k8s.io/client-go/informers"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/event-exporter/utils"
	"k8s.io/client-go/metadata/metadatainformer"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

// PodLabelCollector defines the interface for GetLabels for a namespace and pod name pair.
type PodLabelCollector interface {
	GetLabels(namespace, pod string) map[string]string
}

type PodLabelsSharedInformerFactory struct {
	factory metadatainformer.SharedInformerFactory
}

func (f *PodLabelsSharedInformerFactory) Run(stopCh <-chan struct{}) {
	var m runtime.MemStats
	f.factory.Start(stopCh)
	runtime.ReadMemStats(&m)
	klog.Infof("Start at %d events - Alloc=%vMiB Sys=%vMiB", m.Alloc/1024/1024, m.Sys/1024/1024)

	f.factory.WaitForCacheSync(stopCh)
	runtime.ReadMemStats(&m)
	klog.Infof("WaitForCacheSync at %d events - Alloc=%vMiB Sys=%vMiB", m.Alloc/1024/1024, m.Sys/1024/1024)
	klog.Info("Cache synced!")

	utils.TakeMemorySnapshot()
}

func (f *PodLabelsSharedInformerFactory) NewPodLabelsSharedInformer() *PodLabelsSharedInformer {
	podInformer := f.factory.ForResource(corev1.SchemeGroupVersion.WithResource("pods")).Informer()
	podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(_ any) {
			recordAddition()
		},
		DeleteFunc: func(_ any) {
			recordEviction()
		},
	})
	return &PodLabelsSharedInformer{
		informer: podInformer,
	}
}

type customFeatureGates struct {
	clientfeatures.Gates
}

func (c *customFeatureGates) Enabled(key clientfeatures.Feature) bool {
	if key == clientfeatures.WatchListClient {
		klog.Info("Enabled feature gate: ", key)
		return true
	}
	return c.Gates.Enabled(key)
}

func init() {
	clientfeatures.ReplaceFeatureGates(&customFeatureGates{clientfeatures.FeatureGates()})
}

func NewPodLabelsSharedInformerFactory(client metadata.Interface, ignoredNamespaces []string) *PodLabelsSharedInformerFactory {
	ignoredNamespacesMap := make(map[string]struct{})
	for _, ns := range ignoredNamespaces {
		ignoredNamespacesMap[ns] = struct{}{}
	}
	return &PodLabelsSharedInformerFactory{
		factory: metadatainformer.NewSharedInformerFactoryWithOptions(
			client,
			0,
			metadatainformer.WithTransform(func(obj interface{}) (interface{}, error) {
				meta := obj.(*metav1.PartialObjectMetadata)
				if _, ok := ignoredNamespacesMap[meta.Namespace]; ok {
					return nil, nil
				}
				labels := make(map[string]string)
				if v, ok := meta.Labels["pod-template-hash"]; ok {
					labels["pod-template-hash"] = v
				}
				if v, ok := meta.Labels[jobSetNameLabelKey]; ok {
					labels[jobSetNameLabelKey] = v
				}
				if v, ok := meta.Labels[jobSetRestartAttemptLabelKey]; ok {
					labels[jobSetRestartAttemptLabelKey] = v
				}
				if v, ok := meta.Labels[jobsetUIDLabelKey]; ok {
					labels[jobsetUIDLabelKey] = v
				}
				return &metav1.PartialObjectMetadata{
					ObjectMeta: metav1.ObjectMeta{
						Name:            meta.Name,
						Namespace:       meta.Namespace,
						OwnerReferences: meta.OwnerReferences,
						Labels:          labels,
					},
				}, nil
			}),
		),
	}
}

type PodLabelsSharedInformer struct {
	informer cache.SharedIndexInformer
}

func (informer *PodLabelsSharedInformer) GetLabels(namespace, podName string) map[string]string {
	podItem, exists, _ := informer.informer.GetStore().GetByKey(namespace + "/" + podName)
	if !exists {
		recordQueryMiss()
		return nil
	}
	meta := podItem.(*metav1.PartialObjectMetadata)
	recordQueryHit()
	return getLabelsFromMeta(meta)
}

// get owner labels for go/gke-log-owner-label for non-system logs.
// If there are multiple owners, the loop below picks the last valid one.
func getLabelsFromMeta(meta *metav1.PartialObjectMetadata) map[string]string {
	transformedLabels := map[string]string{}
	for _, owner := range meta.GetOwnerReferences() {
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
			if templateHashSuffix := "-" + meta.GetLabels()["pod-template-hash"]; len(templateHashSuffix) > 1 {
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
			} else if jobsetName := meta.GetLabels()[jobSetNameLabelKey]; jobsetName != "" {
				// Pod that is eventually owned by a JobSet has the jobset name label set.
				transformedLabels[ownerTypeKeyName] = "JobSet"
				transformedLabels[ownerNameKeyName] = jobsetName

				// Add restart_attempt and uid labels for JobSet events.
				if restartAttempt, ok := meta.GetLabels()[jobSetRestartAttemptLabelKey]; ok {
					transformedLabels[jobSetRestartAttemptLabelKey] = restartAttempt
				}
				if uid, ok := meta.GetLabels()[jobsetUIDLabelKey]; ok {
					transformedLabels[jobsetUIDLabelKey] = uid
				}
			} else {
				transformedLabels[ownerTypeKeyName] = "Job"
				transformedLabels[ownerNameKeyName] = owner.Name
			}
		}
	}
	if len(transformedLabels) == 0 {
		return nil
	}
	return transformedLabels
}
