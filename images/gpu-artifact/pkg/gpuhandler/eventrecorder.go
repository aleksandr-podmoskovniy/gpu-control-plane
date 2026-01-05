/*
Copyright 2025 Flant JSC

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

package gpuhandler

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/eventrecord"
)

type eventRecorderProducer struct {
	broadcaster record.EventBroadcaster
	scheme      *runtime.Scheme
}

func (p *eventRecorderProducer) GetEventRecorderFor(name string) record.EventRecorder {
	return p.broadcaster.NewRecorder(p.scheme, corev1.EventSource{Component: name})
}

func newEventRecorder(kubeClient kubernetes.Interface, scheme *runtime.Scheme, component string) (eventrecord.EventRecorderLogger, func()) {
	if kubeClient == nil || scheme == nil {
		return nil, func() {}
	}

	broadcaster := record.NewBroadcaster()
	broadcaster.StartRecordingToSink(&corev1client.EventSinkImpl{
		Interface: kubeClient.CoreV1().Events(""),
	})

	recorder := eventrecord.NewEventRecorderLogger(&eventRecorderProducer{
		broadcaster: broadcaster,
		scheme:      scheme,
	}, component)

	return recorder, broadcaster.Shutdown
}
