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

package watcher

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	invconsts "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory/internal/consts"

	nfdv1alpha1 "sigs.k8s.io/node-feature-discovery/api/nfd/v1alpha1"
)

const (
	deviceLabelPrefix     = invconsts.DeviceLabelPrefix
	migProfileLabelPrefix = invconsts.MIGProfileLabelPrefix

	gfdProductLabel            = invconsts.GFDProductLabel
	gfdMemoryLabel             = invconsts.GFDMemoryLabel
	gfdComputeMajorLabel       = invconsts.GFDComputeMajorLabel
	gfdComputeMinorLabel       = invconsts.GFDComputeMinorLabel
	gfdDriverVersionLabel      = invconsts.GFDDriverVersionLabel
	gfdCudaRuntimeVersionLabel = invconsts.GFDCudaRuntimeVersionLabel
	gfdCudaDriverMajorLabel    = invconsts.GFDCudaDriverMajorLabel
	gfdCudaDriverMinorLabel    = invconsts.GFDCudaDriverMinorLabel
	gfdMigCapableLabel         = invconsts.GFDMigCapableLabel
	gfdMigStrategyLabel        = invconsts.GFDMigStrategyLabel
	gfdMigAltCapableLabel      = invconsts.GFDMigAltCapableLabel
	gfdMigAltStrategy          = invconsts.GFDMigAltStrategyLabel

	nodeFeatureNodeNameLabel = invconsts.NodeFeatureNodeNameLabel
)

type NodeFeatureWatcher struct{}

func NewNodeFeatureWatcher() *NodeFeatureWatcher {
	return &NodeFeatureWatcher{}
}

func (w *NodeFeatureWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	cache := mgr.GetCache()
	if cache == nil {
		return fmt.Errorf("manager cache is required")
	}

	obj := &nfdv1alpha1.NodeFeature{}
	obj.SetGroupVersionKind(nfdv1alpha1.SchemeGroupVersion.WithKind("NodeFeature"))

	return ctr.Watch(
		source.Kind(
			cache,
			obj,
			handler.TypedEnqueueRequestsFromMapFunc(mapNodeFeatureToNode),
			nodeFeaturePredicates(),
		),
	)
}

func mapNodeFeatureToNode(_ context.Context, feature *nfdv1alpha1.NodeFeature) []reconcile.Request {
	if feature == nil {
		return nil
	}
	if !hasGPUDeviceLabels(feature.Spec.Labels) {
		return nil
	}

	nodeName := feature.GetName()
	if labeled := feature.GetLabels()[nodeFeatureNodeNameLabel]; labeled != "" {
		nodeName = labeled
	}
	nodeName = strings.TrimPrefix(nodeName, "nvidia-features-for-")
	if nodeName == "" {
		return nil
	}
	return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: nodeName}}}
}

func nodeFeaturePredicates() predicate.TypedPredicate[*nfdv1alpha1.NodeFeature] {
	return predicate.TypedFuncs[*nfdv1alpha1.NodeFeature]{
		CreateFunc: func(e event.TypedCreateEvent[*nfdv1alpha1.NodeFeature]) bool {
			return hasGPUDeviceLabels(e.Object.Spec.Labels)
		},
		UpdateFunc: func(e event.TypedUpdateEvent[*nfdv1alpha1.NodeFeature]) bool {
			oldLabels := nodeFeatureLabels(e.ObjectOld)
			newLabels := nodeFeatureLabels(e.ObjectNew)
			oldHas := hasGPUDeviceLabels(oldLabels)
			newHas := hasGPUDeviceLabels(newLabels)
			if !oldHas && !newHas {
				return false
			}
			if oldHas != newHas {
				return true
			}
			return gpuLabelsDiffer(oldLabels, newLabels)
		},
		DeleteFunc: func(e event.TypedDeleteEvent[*nfdv1alpha1.NodeFeature]) bool {
			return hasGPUDeviceLabels(nodeFeatureLabels(e.Object))
		},
		GenericFunc: func(event.TypedGenericEvent[*nfdv1alpha1.NodeFeature]) bool { return false },
	}
}

func nodeFeatureLabels(feature *nfdv1alpha1.NodeFeature) map[string]string {
	if feature == nil {
		return nil
	}
	return feature.Spec.Labels
}

func hasGPUDeviceLabels(labels map[string]string) bool {
	if len(labels) == 0 {
		return false
	}
	for key := range labels {
		if strings.HasPrefix(key, deviceLabelPrefix) || strings.HasPrefix(key, migProfileLabelPrefix) {
			return true
		}
	}
	if labels[gfdProductLabel] != "" ||
		labels[gfdMemoryLabel] != "" ||
		labels[gfdMigCapableLabel] != "" ||
		labels[gfdMigAltCapableLabel] != "" ||
		labels[gfdMigStrategyLabel] != "" ||
		labels[gfdMigAltStrategy] != "" {
		return true
	}
	return false
}

func gpuLabelsDiffer(oldLabels, newLabels map[string]string) bool {
	relevantKey := func(key string) bool {
		if strings.HasPrefix(key, deviceLabelPrefix) || strings.HasPrefix(key, migProfileLabelPrefix) {
			return true
		}
		switch key {
		case gfdProductLabel,
			gfdMemoryLabel,
			gfdComputeMajorLabel,
			gfdComputeMinorLabel,
			gfdDriverVersionLabel,
			gfdCudaRuntimeVersionLabel,
			gfdCudaDriverMajorLabel,
			gfdCudaDriverMinorLabel,
			gfdMigCapableLabel,
			gfdMigStrategyLabel,
			gfdMigAltCapableLabel,
			gfdMigAltStrategy:
			return true
		default:
			return false
		}
	}

	get := func(labels map[string]string, key string) string {
		if labels == nil {
			return ""
		}
		return labels[key]
	}

	for key, val := range oldLabels {
		if !relevantKey(key) {
			continue
		}
		if val != get(newLabels, key) {
			return true
		}
	}
	for key, val := range newLabels {
		if !relevantKey(key) {
			continue
		}
		if val != get(oldLabels, key) {
			return true
		}
	}
	return false
}
