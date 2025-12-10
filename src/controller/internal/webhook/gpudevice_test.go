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
	"encoding/json"
	"net/http"
	"testing"

	"github.com/go-logr/logr/testr"
	admv1 "k8s.io/api/admission/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	cradmission "sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
)

func TestGPUDeviceAssignmentValidator(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	pool := &v1alpha1.GPUPool{
		TypeMeta:   metav1TypeMeta("GPUPool"),
		ObjectMeta: metav1ObjectMeta("pool-a"),
		Spec: v1alpha1.GPUPoolSpec{
			DeviceSelector: &v1alpha1.GPUPoolDeviceSelector{
				Include: v1alpha1.GPUPoolSelectorRules{
					InventoryIDs: []string{"inv-1"},
				},
			},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pool).Build()
	decoder := cradmission.NewDecoder(scheme)
	validator := newGPUDeviceAssignmentValidator(testr.New(t), decoder, cl)

	device := &v1alpha1.GPUDevice{
		TypeMeta:   metav1TypeMeta("GPUDevice"),
		ObjectMeta: metav1ObjectMeta("dev-a"),
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "inv-1",
			State:       v1alpha1.GPUDeviceStateReady,
		},
	}
	device.Annotations = map[string]string{assignmentAnnotation: "pool-a"}
	raw, _ := json.Marshal(device)
	req := cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{
		Operation: admv1.Update,
		Object:    runtime.RawExtension{Raw: raw},
	}}

	resp := validator.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Fatalf("expected allowed assignment, got %v", resp.Result)
	}

	// Mismatch selector should be denied.
	device.Status.InventoryID = "inv-2"
	raw, _ = json.Marshal(device)
	req.Object = runtime.RawExtension{Raw: raw}
	resp = validator.Handle(context.Background(), req)
	if resp.Allowed || resp.Result == nil || resp.Result.Code != http.StatusForbidden {
		t.Fatalf("expected denial for selector mismatch, got %+v", resp.Result)
	}

	// Missing pool should be denied.
	device.Annotations[assignmentAnnotation] = "absent"
	raw, _ = json.Marshal(device)
	req.Object = runtime.RawExtension{Raw: raw}
	resp = validator.Handle(context.Background(), req)
	if resp.Allowed || resp.Result == nil || resp.Result.Code != http.StatusForbidden {
		t.Fatalf("expected denial for missing pool, got %+v", resp.Result)
	}

	// No annotation -> allowed.
	delete(device.Annotations, assignmentAnnotation)
	raw, _ = json.Marshal(device)
	req.Object = runtime.RawExtension{Raw: raw}
	resp = validator.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Fatalf("expected allowed when no assignment annotation")
	}

	// Decode error returns 422
	badReq := cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{
		Operation: admv1.Create,
		Object:    runtime.RawExtension{Raw: []byte{}},
	}}
	resp = validator.Handle(context.Background(), badReq)
	if resp.Result == nil || resp.Result.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 on decode error, got %+v", resp.Result)
	}

	// Non-create/update operation is allowed
	device.Annotations[assignmentAnnotation] = "pool-a"
	raw, _ = json.Marshal(device)
	otherReq := cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{
		Operation: admv1.Delete,
		Object:    runtime.RawExtension{Raw: raw},
	}}
	if resp = validator.Handle(context.Background(), otherReq); !resp.Allowed {
		t.Fatalf("expected delete operation to be allowed")
	}

	// Non-ready device is denied.
	device.Status.State = v1alpha1.GPUDeviceStateFaulted
	raw, _ = json.Marshal(device)
	req.Object = runtime.RawExtension{Raw: raw}
	resp = validator.Handle(context.Background(), req)
	if resp.Allowed {
		t.Fatalf("expected denial for non-ready state")
	}

	// Ignored device is denied.
	device.Status.State = v1alpha1.GPUDeviceStateReady
	device.Labels = map[string]string{"gpu.deckhouse.io/ignore": "true"}
	raw, _ = json.Marshal(device)
	req.Object = runtime.RawExtension{Raw: raw}
	resp = validator.Handle(context.Background(), req)
	if resp.Allowed {
		t.Fatalf("expected denial for ignored device")
	}

	// Ignored via annotation is denied.
	delete(device.Labels, "gpu.deckhouse.io/ignore")
	device.Annotations = map[string]string{"gpu.deckhouse.io/ignore": "true"}
	raw, _ = json.Marshal(device)
	req.Object = runtime.RawExtension{Raw: raw}
	resp = validator.Handle(context.Background(), req)
	if resp.Allowed {
		t.Fatalf("expected denial for ignored device via annotation")
	}
}

func TestMigProfilesHelper(t *testing.T) {
	cfg := v1alpha1.GPUMIGConfig{ProfilesSupported: []string{"1g.10gb"}}
	if profiles := migProfiles(cfg); len(profiles) != 1 || profiles[0] != "1g.10gb" {
		t.Fatalf("expected profilesSupported passthrough")
	}

	cfg = v1alpha1.GPUMIGConfig{Types: []v1alpha1.GPUMIGTypeCapacity{{Name: "2g.20gb"}, {Name: ""}}}
	if profiles := migProfiles(cfg); len(profiles) != 1 || profiles[0] != "2g.20gb" {
		t.Fatalf("expected fallback to types, got %v", profiles)
	}
}

type failingGetClient struct {
	client.Client
}

func (f *failingGetClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	return apierrors.NewBadRequest("boom")
}

func TestGPUDeviceValidatorClientError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	device := &v1alpha1.GPUDevice{
		TypeMeta: metav1TypeMeta("GPUDevice"),
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev",
			Annotations: map[string]string{assignmentAnnotation: "pool"},
		},
		Status: v1alpha1.GPUDeviceStatus{State: v1alpha1.GPUDeviceStateReady},
	}
	raw, _ := json.Marshal(device)
	req := cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{
		Operation: admv1.Create,
		Object:    runtime.RawExtension{Raw: raw},
	}}
	validator := &gpuDeviceAssignmentValidator{
		log:     testr.New(t),
		decoder: cradmission.NewDecoder(scheme),
		client:  &failingGetClient{Client: fake.NewClientBuilder().WithScheme(scheme).Build()},
	}
	resp := validator.Handle(context.Background(), req)
	if resp.Result == nil || resp.Result.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 error, got %+v", resp.Result)
	}
}

func metav1TypeMeta(kind string) metav1.TypeMeta {
	return metav1.TypeMeta{Kind: kind, APIVersion: v1alpha1.GroupVersion.String()}
}

func metav1ObjectMeta(name string) metav1.ObjectMeta {
	return metav1.ObjectMeta{Name: name}
}
