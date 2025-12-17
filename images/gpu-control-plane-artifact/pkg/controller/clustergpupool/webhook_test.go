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

package clustergpupool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/go-logr/logr/testr"
	admv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	cradmission "sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	pooladmission "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/pool/admission"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
)

func TestClusterGPUPoolDefaulterAddsDefaults(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	handlers := []contracts.AdmissionHandler{pooladmission.NewPoolValidationHandler(testr.New(t))}

	rawPool := v1alpha1.ClusterGPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool"},
		Spec: v1alpha1.GPUPoolSpec{
			Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"},
		},
	}
	rawBytes, _ := json.Marshal(rawPool)

	req := cradmission.Request{
		AdmissionRequest: admv1.AdmissionRequest{
			Operation: admv1.Create,
			Object:    runtime.RawExtension{Raw: rawBytes},
		},
	}

	defaulter := cradmission.WithCustomDefaulter(scheme, &v1alpha1.ClusterGPUPool{}, NewClusterGPUPoolDefaulter(testr.New(t), handlers))
	resp := defaulter.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Fatalf("expected allowed response, got denied: %v", resp.Result)
	}
	if len(resp.Patches) == 0 {
		t.Fatalf("expected patches with defaults")
	}
}

func TestClusterGPUPoolValidatorRejectsImmutableChanges(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	handlers := []contracts.AdmissionHandler{pooladmission.NewPoolValidationHandler(testr.New(t))}

	oldPool := v1alpha1.ClusterGPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool"},
		Spec: v1alpha1.GPUPoolSpec{
			Provider: "Nvidia",
			Backend:  "DevicePlugin",
			Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card", SlicesPerUnit: 1},
		},
	}
	newPool := oldPool.DeepCopy()
	newPool.Spec.Backend = "DRA"

	oldRaw, _ := json.Marshal(oldPool)
	newRaw, _ := json.Marshal(newPool)

	req := cradmission.Request{
		AdmissionRequest: admv1.AdmissionRequest{
			Operation: admv1.Update,
			Object:    runtime.RawExtension{Raw: newRaw},
			OldObject: runtime.RawExtension{Raw: oldRaw},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	validator := cradmission.WithCustomValidator(scheme, &v1alpha1.ClusterGPUPool{}, NewClusterGPUPoolValidator(testr.New(t), cl, handlers))
	resp := validator.Handle(context.Background(), req)
	if resp.Allowed {
		t.Fatalf("expected immutable change to be denied")
	}
}

func TestClusterGPUPoolValidatorRejectsNameConflictWithNamespacedPool(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	handlers := []contracts.AdmissionHandler{pooladmission.NewPoolValidationHandler(testr.New(t))}

	existing := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: "gpu-team"},
		Spec:       v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"}},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()

	pool := v1alpha1.ClusterGPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool"},
		Spec:       v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"}},
	}
	raw, _ := json.Marshal(pool)
	req := cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{
		Operation: admv1.Create,
		Object:    runtime.RawExtension{Raw: raw},
	}}

	validator := cradmission.WithCustomValidator(scheme, &v1alpha1.ClusterGPUPool{}, NewClusterGPUPoolValidator(testr.New(t), cl, handlers))
	if resp := validator.Handle(context.Background(), req); resp.Allowed {
		t.Fatalf("expected denial due to name conflict with GPUPool")
	}
}
