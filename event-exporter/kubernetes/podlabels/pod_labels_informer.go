package podlabels

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

// PodLabelCollector defines the interface for GetLabels for a namespace and pod name pair.
type PodLabelCollector interface {
	GetLabels(namespace, pod string) map[string]string
}

type PodLabelsSharedInformerFactory struct {
	factory informers.SharedInformerFactory
}

func (f *PodLabelsSharedInformerFactory) Run(stopCh <-chan struct{}) {
	f.factory.Start(stopCh)
	f.factory.WaitForCacheSync(stopCh)
}

func (f *PodLabelsSharedInformerFactory) NewPodLabelsSharedInformer() *PodLabelsSharedInformer {
	podInformer := f.factory.Core().V1().Pods().Informer()
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

func NewPodLabelsSharedInformerFactory(client kubernetes.Interface, ignoredNamespaces []string) *PodLabelsSharedInformerFactory {
	ignoredNamespacesMap := make(map[string]struct{})
	for _, ns := range ignoredNamespaces {
		ignoredNamespacesMap[ns] = struct{}{}
	}
	return &PodLabelsSharedInformerFactory{
		factory: informers.NewSharedInformerFactoryWithOptions(
			client,
			0,
			informers.WithTransform(func(obj interface{}) (interface{}, error) {
				pod := obj.(*corev1.Pod)
				if _, ok := ignoredNamespacesMap[pod.Namespace]; ok {
					return nil, nil
				}
				labels := make(map[string]string)
				if v, ok := pod.Labels["pod-template-hash"]; ok {
					labels["pod-template-hash"] = v
				}
				if v, ok := pod.Labels[jobSetNameLabelKey]; ok {
					labels[jobSetNameLabelKey] = v
				}
				return &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:            pod.Name,
						Namespace:       pod.Namespace,
						OwnerReferences: pod.OwnerReferences,
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
	pod := podItem.(*corev1.Pod)
	recordQueryHit()
	return getLabelsFromPod(pod)
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
	if len(transformedLabels) == 0 {
		return nil
	}
	return transformedLabels
}
