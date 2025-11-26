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

package admission

import (
	"errors"
	"testing"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
)

func TestRunValidationsSuccessAndError(t *testing.T) {
	spec := &v1alpha1.GPUPoolSpec{}
	called := 0
	validators := []func(*v1alpha1.GPUPoolSpec) error{
		func(*v1alpha1.GPUPoolSpec) error { called++; return nil },
		func(*v1alpha1.GPUPoolSpec) error { called++; return nil },
	}
	if err := runValidations(validators, spec); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if called != 2 {
		t.Fatalf("expected both validators to run, got %d", called)
	}

	errValidators := []func(*v1alpha1.GPUPoolSpec) error{
		func(*v1alpha1.GPUPoolSpec) error { return errors.New("fail") },
		func(*v1alpha1.GPUPoolSpec) error { t.Fatal("should not be called"); return nil },
	}
	if err := runValidations(errValidators, spec); err == nil {
		t.Fatalf("expected error from validators")
	}
}
