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
)

func TestPodValidatorInterfaceMethods(t *testing.T) {
	v := NewPodValidator(testr.New(t), nil)

	t.Run("validate-create-no-pool", func(t *testing.T) {
		warnings, err := v.ValidateCreate(context.Background(), &corev1.Pod{})
		if err != nil || len(warnings) != 0 {
			t.Fatalf("expected (nil,nil), got (%v,%v)", warnings, err)
		}
	})

	t.Run("validate-update-no-pool", func(t *testing.T) {
		warnings, err := v.ValidateUpdate(context.Background(), &corev1.Pod{}, &corev1.Pod{})
		if err != nil || len(warnings) != 0 {
			t.Fatalf("expected (nil,nil), got (%v,%v)", warnings, err)
		}
	})

	t.Run("validate-create-wrong-type", func(t *testing.T) {
		if _, err := v.ValidateCreate(context.Background(), &corev1.Namespace{}); err == nil {
			t.Fatalf("expected type assertion error")
		}
	})

	t.Run("validate-delete-noop", func(t *testing.T) {
		warnings, err := v.ValidateDelete(context.Background(), &corev1.Pod{})
		if err != nil || len(warnings) != 0 {
			t.Fatalf("expected (nil,nil), got (%v,%v)", warnings, err)
		}
	})
}
