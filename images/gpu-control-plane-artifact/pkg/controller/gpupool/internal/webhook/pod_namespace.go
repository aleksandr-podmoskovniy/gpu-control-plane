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

package webhook

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonobject "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/common/object"
)

func requireGPUEnabledNamespace(ctx context.Context, c client.Client, namespace string) error {
	if c == nil {
		return nil
	}
	if strings.TrimSpace(namespace) == "" {
		return fmt.Errorf("pod namespace is empty")
	}

	ns := &corev1.Namespace{}
	ns, err := commonobject.FetchObject(ctx, types.NamespacedName{Name: namespace}, c, ns)
	if err != nil {
		return fmt.Errorf("get namespace %q: %w", namespace, err)
	}
	if ns == nil {
		return fmt.Errorf("namespace %q not found", namespace)
	}
	if ns.Labels[gpuEnabledLabelKey] != "true" {
		return fmt.Errorf("namespace %q is not enabled for GPU (label %s=true is required)", namespace, gpuEnabledLabelKey)
	}
	return nil
}
