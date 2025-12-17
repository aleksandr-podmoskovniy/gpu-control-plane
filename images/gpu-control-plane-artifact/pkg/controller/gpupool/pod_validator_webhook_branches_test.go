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

package gpupool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/go-logr/logr/testr"
	admv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	cradmission "sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func TestPodValidatorAllowsWhenRequestedIsZero(t *testing.T) {
	validator := newPodValidator(testr.New(t), nil)
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{corev1.ResourceName("gpu.deckhouse.io/pool-a"): resource.MustParse("0")},
				},
			}},
		},
	}
	raw, _ := json.Marshal(pod)
	req := cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{Object: runtime.RawExtension{Raw: raw}}}
	if resp := validator.Handle(context.Background(), req); !resp.Allowed {
		t.Fatalf("expected allowed when requested is zero, got %v", resp.Result)
	}
}

func TestRequestedResourcesRequestsAndZeroBranch(t *testing.T) {
	pool := poolRequest{name: "pool-a", keyPrefix: localPoolResourcePrefix}

	pod := &corev1.Pod{Spec: corev1.PodSpec{
		Containers: []corev1.Container{{
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceName("gpu.deckhouse.io/pool-a"): resource.MustParse("2")},
			},
		}},
	}}
	if got := requestedResources(pod, pool); got != 2 {
		t.Fatalf("expected Requests value to be used, got %d", got)
	}

	// Ensure "return 0" branch is executed (container exists but resource not present).
	if got := requestedResources(&corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{}}}}, pool); got != 0 {
		t.Fatalf("expected zero when resource is absent, got %d", got)
	}
}
