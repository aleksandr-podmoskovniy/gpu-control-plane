// Copyright 2025 Flant JSC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package trigger

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicinformer "k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"

	"github.com/deckhouse/deckhouse/pkg/log"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/state"
)

// PhysicalGPUWatcher watches PhysicalGPU changes on the current node.
type PhysicalGPUWatcher struct {
	log           *log.Logger
	client        dynamic.Interface
	labelSelector string
}

// NewPhysicalGPUWatcher constructs a watcher filtered by node label.
func NewPhysicalGPUWatcher(client dynamic.Interface, nodeName string, log *log.Logger) *PhysicalGPUWatcher {
	selector := labels.Set{state.LabelNode: nodeName}.AsSelector().String()
	return &PhysicalGPUWatcher{
		log:           log,
		client:        client,
		labelSelector: selector,
	}
}

// Run starts the informer and triggers sync on any change.
func (w *PhysicalGPUWatcher) Run(ctx context.Context, notify NotifyFunc) error {
	gvr := schema.GroupVersionResource{
		Group:    "gpu.deckhouse.io",
		Version:  "v1alpha1",
		Resource: "physicalgpus",
	}

	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(
		w.client,
		0,
		metav1.NamespaceAll,
		func(options *metav1.ListOptions) {
			options.LabelSelector = w.labelSelector
		},
	)
	informer := factory.ForResource(gvr).Informer()
	if _, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(_ interface{}) {
			notify()
		},
		UpdateFunc: func(_, _ interface{}) {
			notify()
		},
		DeleteFunc: func(_ interface{}) {
			notify()
		},
	}); err != nil {
		return fmt.Errorf("register physicalgpu watcher: %w", err)
	}

	factory.Start(ctx.Done())
	if !cache.WaitForCacheSync(ctx.Done(), informer.HasSynced) {
		return fmt.Errorf("physicalgpu watcher sync failed")
	}

	w.log.Info("physicalgpu watcher started", "selector", w.labelSelector)
	<-ctx.Done()
	return nil
}
