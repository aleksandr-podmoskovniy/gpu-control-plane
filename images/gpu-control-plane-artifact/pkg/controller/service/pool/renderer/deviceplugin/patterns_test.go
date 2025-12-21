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

package deviceplugin

import (
	"context"
	"testing"

	"github.com/go-logr/logr/testr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/indexer"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/renderer/deps"
)

func TestAssignedDevicePatternsNilInputs(t *testing.T) {
	if got := AssignedDevicePatterns(context.Background(), deps.Deps{}, nil); got != nil {
		t.Fatalf("expected nil patterns for nil client/pool, got %+v", got)
	}
}

func TestAssignedDevicePatternsFiltersAndSorts(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "alpha", Namespace: "ns"}}

	devices := []client.Object{
		&v1alpha1.GPUDevice{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "ignored",
				Labels: map[string]string{"gpu.deckhouse.io/ignore": "true"},
			},
			Status: v1alpha1.GPUDeviceStatus{
				State:       v1alpha1.GPUDeviceStateAssigned,
				PoolRef:     &v1alpha1.GPUPoolReference{Name: "alpha", Namespace: "ns"},
				Hardware:    v1alpha1.GPUDeviceHardware{UUID: "GPU-IGN"},
				NodeName:    "node1",
				Managed:     true,
				InventoryID: "inv",
			},
		},
		&v1alpha1.GPUDevice{
			ObjectMeta: metav1.ObjectMeta{Name: "mismatch"},
			Status: v1alpha1.GPUDeviceStatus{
				State:    v1alpha1.GPUDeviceStateAssigned,
				PoolRef:  &v1alpha1.GPUPoolReference{Name: "alpha", Namespace: "other"},
				Hardware: v1alpha1.GPUDeviceHardware{UUID: "GPU-MISM"},
			},
		},
		&v1alpha1.GPUDevice{
			ObjectMeta: metav1.ObjectMeta{Name: "state-not-allowed"},
			Status: v1alpha1.GPUDeviceStatus{
				State:    v1alpha1.GPUDeviceStateReady,
				PoolRef:  &v1alpha1.GPUPoolReference{Name: "alpha", Namespace: "ns"},
				Hardware: v1alpha1.GPUDeviceHardware{UUID: "GPU-READY"},
			},
		},
		&v1alpha1.GPUDevice{
			ObjectMeta: metav1.ObjectMeta{Name: "no-uuid"},
			Status: v1alpha1.GPUDeviceStatus{
				State:    v1alpha1.GPUDeviceStateAssigned,
				PoolRef:  &v1alpha1.GPUPoolReference{Name: "alpha", Namespace: "ns"},
				Hardware: v1alpha1.GPUDeviceHardware{UUID: " "},
			},
		},
		&v1alpha1.GPUDevice{
			ObjectMeta: metav1.ObjectMeta{Name: "dup"},
			Status: v1alpha1.GPUDeviceStatus{
				State:    v1alpha1.GPUDeviceStateReserved,
				PoolRef:  &v1alpha1.GPUPoolReference{Name: "alpha"},
				Hardware: v1alpha1.GPUDeviceHardware{UUID: "GPU-BBB"},
			},
		},
		&v1alpha1.GPUDevice{
			ObjectMeta: metav1.ObjectMeta{Name: "dup-2"},
			Status: v1alpha1.GPUDeviceStatus{
				State:    v1alpha1.GPUDeviceStateAssigned,
				PoolRef:  &v1alpha1.GPUPoolReference{Name: "alpha", Namespace: "ns"},
				Hardware: v1alpha1.GPUDeviceHardware{UUID: "GPU-BBB"},
			},
		},
		&v1alpha1.GPUDevice{
			ObjectMeta: metav1.ObjectMeta{Name: "ok"},
			Status: v1alpha1.GPUDeviceStatus{
				State:    v1alpha1.GPUDeviceStateAssigned,
				PoolRef:  &v1alpha1.GPUPoolReference{Name: "alpha", Namespace: "ns"},
				Hardware: v1alpha1.GPUDeviceHardware{UUID: "GPU-AAA"},
			},
		},
	}

	cl := withPoolDeviceIndexes(fake.NewClientBuilder().
		WithScheme(scheme)).
		WithObjects(devices...).
		Build()

	got := AssignedDevicePatterns(context.Background(), deps.Deps{Client: cl, Log: testr.New(t)}, pool)
	if len(got) != 2 || got[0] != "GPU-AAA" || got[1] != "GPU-BBB" {
		t.Fatalf("unexpected patterns: %+v", got)
	}
}

func TestPoolHasAssignedDevicesBranches(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "alpha", Namespace: "ns"}}

	has, err := PoolHasAssignedDevices(context.Background(), deps.Deps{}, pool)
	if err != nil || has {
		t.Fatalf("expected (false,nil) for nil client, got (%v,%v)", has, err)
	}

	noIndex := fake.NewClientBuilder().WithScheme(scheme).Build()
	if _, err := PoolHasAssignedDevices(context.Background(), deps.Deps{Client: noIndex}, pool); err == nil {
		t.Fatalf("expected list error without index")
	}

	cl := withPoolDeviceIndexes(fake.NewClientBuilder().
		WithScheme(scheme)).
		WithObjects(
			&v1alpha1.GPUDevice{ObjectMeta: metav1.ObjectMeta{Name: "ignored", Labels: map[string]string{"gpu.deckhouse.io/ignore": "true"}}, Status: v1alpha1.GPUDeviceStatus{PoolRef: &v1alpha1.GPUPoolReference{Name: "alpha", Namespace: "ns"}}},
			&v1alpha1.GPUDevice{ObjectMeta: metav1.ObjectMeta{Name: "mismatch"}, Status: v1alpha1.GPUDeviceStatus{PoolRef: &v1alpha1.GPUPoolReference{Name: "alpha", Namespace: "other"}}},
		).
		Build()

	has, err = PoolHasAssignedDevices(context.Background(), deps.Deps{Client: cl}, pool)
	if err != nil || has {
		t.Fatalf("expected (false,nil) for only ignored/mismatch, got (%v,%v)", has, err)
	}

	if err := cl.Create(context.Background(), &v1alpha1.GPUDevice{ObjectMeta: metav1.ObjectMeta{Name: "ok"}, Status: v1alpha1.GPUDeviceStatus{PoolRef: &v1alpha1.GPUPoolReference{Name: "alpha", Namespace: "ns"}}}); err != nil {
		t.Fatalf("create: %v", err)
	}
	has, err = PoolHasAssignedDevices(context.Background(), deps.Deps{Client: cl}, pool)
	if err != nil || !has {
		t.Fatalf("expected (true,nil) when device exists, got (%v,%v)", has, err)
	}
}

func withPoolDeviceIndexes(builder *fake.ClientBuilder) *fake.ClientBuilder {
	if builder == nil {
		return nil
	}

	return builder.WithIndex(&v1alpha1.GPUDevice{}, indexer.GPUDevicePoolRefNameField, func(obj client.Object) []string {
		dev, ok := obj.(*v1alpha1.GPUDevice)
		if !ok || dev.Status.PoolRef == nil || dev.Status.PoolRef.Name == "" {
			return nil
		}
		return []string{dev.Status.PoolRef.Name}
	})
}
