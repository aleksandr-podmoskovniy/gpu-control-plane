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

package inventory

import (
	"context"
	"strconv"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonobject "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/common/object"
	invconsts "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory/internal/consts"

	nfdv1alpha1 "sigs.k8s.io/node-feature-discovery/api/nfd/v1alpha1"
)

func (r *Reconciler) findNodeFeature(ctx context.Context, nodeName string) (*nfdv1alpha1.NodeFeature, error) {
	feature := &nfdv1alpha1.NodeFeature{}
	feature, err := commonobject.FetchObject(ctx, types.NamespacedName{Name: nodeName}, r.client, feature)
	if err != nil {
		return nil, err
	}
	if feature != nil {
		return feature, nil
	}

	list := &nfdv1alpha1.NodeFeatureList{}
	if err := r.client.List(ctx, list, client.MatchingLabels{invconsts.NodeFeatureNodeNameLabel: nodeName}); err != nil {
		return nil, err
	}

	return chooseNodeFeature(list.Items, nodeName), nil
}

func chooseNodeFeature(items []nfdv1alpha1.NodeFeature, nodeName string) *nfdv1alpha1.NodeFeature {
	if len(items) == 0 {
		return nil
	}

	selected := items[0].DeepCopy()
	for i := 1; i < len(items); i++ {
		item := items[i]
		if item.GetName() == nodeName && selected.GetName() != nodeName {
			selected = item.DeepCopy()
			continue
		}
		if resourceVersionNewer(item.GetResourceVersion(), selected.GetResourceVersion()) {
			selected = item.DeepCopy()
		}
	}
	return selected
}

func resourceVersionNewer(candidate, current string) bool {
	if candidate == "" {
		return false
	}
	if current == "" {
		return true
	}

	candidateInt, errCandidate := strconv.ParseUint(candidate, 10, 64)
	currentInt, errCurrent := strconv.ParseUint(current, 10, 64)

	switch {
	case errCandidate == nil && errCurrent == nil:
		return candidateInt > currentInt
	case errCandidate == nil:
		return true
	case errCurrent == nil:
		return false
	default:
		return candidate > current
	}
}
