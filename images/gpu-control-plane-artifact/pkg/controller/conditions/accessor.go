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

package conditions

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
)

type ConditionsAccessor interface {
	Conditions() *[]metav1.Condition
}

type conditionsAccessorImpl struct {
	conditions *[]metav1.Condition
}

func (c *conditionsAccessorImpl) Conditions() *[]metav1.Condition {
	return c.conditions
}

func NewConditionsAccessor(obj client.Object) ConditionsAccessor {
	var ptr *[]metav1.Condition
	switch v := obj.(type) {
	case *v1alpha1.GPUDevice:
		ptr = &v.Status.Conditions
	case *v1alpha1.GPUNodeState:
		ptr = &v.Status.Conditions
	case *v1alpha1.GPUPool:
		ptr = &v.Status.Conditions
	case *v1alpha1.ClusterGPUPool:
		ptr = &v.Status.Conditions
	}
	return &conditionsAccessorImpl{conditions: ptr}
}
