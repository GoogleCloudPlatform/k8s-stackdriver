/*
Copyright 2017 Google Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package events

import (
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func eventV1ToCoreEvent(event *eventsv1.Event) *corev1.Event {
	if event == nil {
		return nil
	}

	coreEvent := &corev1.Event{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "Event",
		},
		ObjectMeta:          event.ObjectMeta,
		InvolvedObject:      event.Regarding,
		Reason:              event.Reason,
		Message:             event.Note,
		Source:              event.DeprecatedSource,
		FirstTimestamp:      event.DeprecatedFirstTimestamp,
		LastTimestamp:       event.DeprecatedLastTimestamp,
		Count:               event.DeprecatedCount,
		Type:                event.Type,
		EventTime:           event.EventTime,
		Series:              convertEventSeries(event.Series),
		Action:              event.Action,
		Related:             event.Related,
		ReportingController: event.ReportingController,
		ReportingInstance:   event.ReportingInstance,
	}

	if coreEvent.Count == 0 && coreEvent.Series != nil {
		coreEvent.Count = coreEvent.Series.Count
	}

	return coreEvent
}

func convertEventSeries(series *eventsv1.EventSeries) *corev1.EventSeries {
	if series == nil {
		return nil
	}

	return &corev1.EventSeries{
		Count:            series.Count,
		LastObservedTime: series.LastObservedTime,
	}
}
