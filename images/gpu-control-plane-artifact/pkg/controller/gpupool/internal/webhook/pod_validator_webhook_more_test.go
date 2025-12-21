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
	"testing"

	"github.com/go-logr/logr/testr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestPodValidatorRequestedZeroIsAllowed(t *testing.T) {
	v := NewPodValidator(testr.New(t), nil)
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name: "c",
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceName(localPoolResourcePrefix + "pool-a"): *resource.NewQuantity(0, resource.DecimalSI),
					},
				},
			}},
		},
	}

	warnings, err := v.validate(context.Background(), pod)
	if err != nil || len(warnings) != 0 {
		t.Fatalf("expected allowed for 0 request, got warnings=%v err=%v", warnings, err)
	}
}

