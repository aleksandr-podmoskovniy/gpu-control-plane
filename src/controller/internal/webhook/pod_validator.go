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
	"net/http"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	cradmission "sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// podValidator rejects pods requesting GPU resources when namespace/pool access is not allowed.
type podValidator struct {
	log    logr.Logger
	client client.Client
}

func newPodValidator(log logr.Logger, c client.Client) *podValidator {
	return &podValidator{
		log:    log.WithName("pod-validator"),
		client: c,
	}
}

func (v *podValidator) Handle(ctx context.Context, req cradmission.Request) cradmission.Response {
	pod, _, err := decodePodRequest(req)
	if err != nil {
		if err == errEmptyPodAdmissionRequest {
			return cradmission.Denied(err.Error())
		}
		return cradmission.Errored(http.StatusUnprocessableEntity, err)
	}

	poolRef, ok, err := selectSinglePool(pod)
	if err != nil {
		return cradmission.Denied(err.Error())
	}
	if !ok {
		return cradmission.Allowed("no gpu pool requested")
	}

	if err := requireGPUEnabledNamespace(ctx, v.client, pod.Namespace); err != nil {
		return cradmission.Denied(err.Error())
	}

	requested := requestedResources(pod, poolRef)
	if requested <= 0 {
		return cradmission.Allowed("no gpu pool requested")
	}

	pool, err := resolvePoolByRequest(ctx, v.client, poolRef, pod.Namespace)
	if err != nil {
		return cradmission.Denied(err.Error())
	}

	// Static sanity check only: requested <= capacity.total (no dynamic availability tracking).
	// Skip the check when the pool has not been configured yet or reports zero capacity.
	// Zero capacity is not treated as a hard admission error: the scheduler/device-plugin remain the source of truth.
	cond := apimeta.FindStatusCondition(pool.Status.Conditions, "Configured")
	if cond != nil && cond.Status == metav1.ConditionFalse {
		return cradmission.Denied(fmt.Sprintf("GPU pool %s is not configured: %s", poolRef.keyPrefix+poolRef.name, cond.Message))
	}
	if cond != nil && cond.Status == metav1.ConditionTrue {
		total := int64(pool.Status.Capacity.Total)
		if total > 0 && requested > total {
			return cradmission.Denied(fmt.Sprintf("requested %d units of %s but pool capacity is %d", requested, poolRef.keyPrefix+poolRef.name, total))
		}
	}

	return cradmission.Allowed("gpu pod validated")
}

func (v *podValidator) GVK() schema.GroupVersionKind {
	return corev1.SchemeGroupVersion.WithKind("Pod")
}

func requestedResources(pod *corev1.Pod, pool poolRequest) int64 {
	name := corev1.ResourceName(pool.keyPrefix + pool.name)
	value := func(req corev1.ResourceRequirements) int64 {
		if q, ok := req.Limits[name]; ok {
			return q.Value()
		}
		if q, ok := req.Requests[name]; ok {
			return q.Value()
		}
		return 0
	}

	var sumContainers int64
	for _, c := range pod.Spec.Containers {
		sumContainers += value(c.Resources)
	}

	var maxInit int64
	for _, c := range pod.Spec.InitContainers {
		if v := value(c.Resources); v > maxInit {
			maxInit = v
		}
	}

	if sumContainers > maxInit {
		return sumContainers
	}
	return maxInit
}
