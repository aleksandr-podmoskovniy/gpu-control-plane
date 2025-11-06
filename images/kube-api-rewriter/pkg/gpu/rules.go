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

package gpu

import (
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/images/kube-api-rewriter/pkg/rewriter"
)

const (
	apiGroup            = "gpu.deckhouse.io"
	internalAPIGroup    = "internal.gpu.deckhouse.io"
	internalKindPrefix  = "InternalGPU"
	internalResourceTag = "internalgpu"
	internalShortName   = "igpu"
)

// GPURewriteRules describes how public GPU API resources are mirrored to an
// internal group consumed by the control plane. The structure is intentionally
// compact: we only rename the API group and reuse existing plural/short names.
var GPURewriteRules = &rewriter.RewriteRules{
	KindPrefix:         internalKindPrefix,
	ResourceTypePrefix: internalResourceTag,
	ShortNamePrefix:    internalShortName,
	Categories:         []string{"gpu"},
	Rules: map[string]rewriter.APIGroupRule{
		apiGroup: {
			GroupRule: rewriter.GroupRule{
				Group:            apiGroup,
				Versions:         []string{"v1alpha1"},
				PreferredVersion: "v1alpha1",
				Renamed:          internalAPIGroup,
			},
			ResourceRules: map[string]rewriter.ResourceRule{
				"gpudevices": {
					Kind:             "GPUDevice",
					ListKind:         "GPUDeviceList",
					Plural:           "gpudevices",
					Singular:         "gpudevice",
					ShortNames:       []string{"gdevice", "gpudev"},
					Categories:       []string{"gpu"},
					Versions:         []string{"v1alpha1"},
					PreferredVersion: "v1alpha1",
				},
				"gpunodeinventories": {
					Kind:             "GPUNodeInventory",
					ListKind:         "GPUNodeInventoryList",
					Plural:           "gpunodeinventories",
					Singular:         "gpunodeinventory",
					ShortNames:       []string{"gpunode", "gpnode"},
					Categories:       []string{"gpu"},
					Versions:         []string{"v1alpha1"},
					PreferredVersion: "v1alpha1",
				},
				"gpupools": {
					Kind:             "GPUPool",
					ListKind:         "GPUPoolList",
					Plural:           "gpupools",
					Singular:         "gpupool",
					Categories:       []string{"gpu"},
					Versions:         []string{"v1alpha1"},
					PreferredVersion: "v1alpha1",
				},
			},
		},
	},
}
