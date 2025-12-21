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

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	cradmission "sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type PodValidator struct {
	log    logr.Logger
	client client.Client
}

func NewPodValidator(log logr.Logger, c client.Client) *PodValidator {
	return &PodValidator{
		log:    log.WithName("pod-webhook"),
		client: c,
	}
}

func (v *PodValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (cradmission.Warnings, error) {
	return v.validate(ctx, obj)
}

func (v *PodValidator) ValidateUpdate(ctx context.Context, _ runtime.Object, newObj runtime.Object) (cradmission.Warnings, error) {
	return v.validate(ctx, newObj)
}

func (v *PodValidator) ValidateDelete(_ context.Context, _ runtime.Object) (cradmission.Warnings, error) {
	err := fmt.Errorf("misconfigured webhook rules: delete operation not implemented")
	v.log.Error(err, "Ensure the correctness of ValidatingWebhookConfiguration")
	return nil, nil
}

func (v *PodValidator) validate(ctx context.Context, obj runtime.Object) (cradmission.Warnings, error) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return nil, fmt.Errorf("expected a Pod but got a %T", obj)
	}

	namespace := effectiveNamespace(ctx, pod.Namespace)

	poolRef, ok, err := selectSinglePool(pod)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}

	if err := requireGPUEnabledNamespace(ctx, v.client, namespace); err != nil {
		return nil, err
	}

	requested := requestedResources(pod, poolRef)
	if requested <= 0 {
		return nil, nil
	}

	poolObj, err := resolvePoolByRequest(ctx, v.client, poolRef, namespace)
	if err != nil {
		return nil, err
	}

	cond := apimeta.FindStatusCondition(poolObj.Status.Conditions, "Configured")
	if cond != nil && cond.Status == metav1.ConditionFalse {
		return nil, fmt.Errorf("GPU pool %s is not configured: %s", poolRef.keyPrefix+poolRef.name, cond.Message)
	}
	if cond != nil && cond.Status == metav1.ConditionTrue {
		total := int64(poolObj.Status.Capacity.Total)
		if total > 0 && requested > total {
			return nil, fmt.Errorf("requested %d units of %s but pool capacity is %d", requested, poolRef.keyPrefix+poolRef.name, total)
		}
	}

	return nil, nil
}
