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

package service

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	moduleLabelKey     = "module"
	moduleLabelValue   = "gpu-control-plane"
	componentLabelKey  = "component"
	componentValidator = "validator"
	appLabelKey        = "app"
	validatorAppName   = "nvidia-operator-validator"
)

// ValidationResult describes validator readiness for a node.
type ValidationResult struct {
	Present bool
	Ready   bool
	Message string
}

// Validator checks readiness of bootstrap validator pods.
type Validator struct {
	client    client.Client
	namespace string
}

// NewValidator creates a validator checker.
func NewValidator(cl client.Client, namespace string) *Validator {
	return &Validator{client: cl, namespace: namespace}
}

// Status reports whether validator pod is ready on the given node.
func (v *Validator) Status(ctx context.Context, nodeName string) (ValidationResult, error) {
	result := ValidationResult{}

	pods := &corev1.PodList{}
	if err := v.client.List(ctx, pods,
		client.InNamespace(v.namespace),
		client.MatchingLabels{
			moduleLabelKey:    moduleLabelValue,
			componentLabelKey: componentValidator,
			appLabelKey:       validatorAppName,
		},
	); err != nil {
		return result, err
	}

	if len(pods.Items) == 0 {
		result.Message = "validator pods not found"
		return result, nil
	}

	result.Present = true
	nodePods := filterPodsByNode(pods.Items, nodeName)
	if len(nodePods) == 0 {
		result.Message = fmt.Sprintf("validator pod not found on node %q", nodeName)
		return result, nil
	}

	for i := range nodePods {
		if podReady(&nodePods[i]) {
			result.Ready = true
			return result, nil
		}
	}

	result.Message = "validator pod is not ready"
	return result, nil
}

func filterPodsByNode(pods []corev1.Pod, nodeName string) []corev1.Pod {
	if nodeName == "" {
		return pods
	}
	out := make([]corev1.Pod, 0, len(pods))
	for i := range pods {
		if pods[i].Spec.NodeName == nodeName {
			out = append(out, pods[i])
		}
	}
	return out
}

func podReady(pod *corev1.Pod) bool {
	for i := range pod.Status.Conditions {
		cond := pod.Status.Conditions[i]
		if cond.Type == corev1.PodReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}
