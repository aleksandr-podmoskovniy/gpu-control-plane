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

package handler

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/nodeagent/internal/service"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/nodeagent/internal/state"
)

func TestApplyHandlerCreatesPhysicalGPU(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := gpuv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}

	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gpuv1alpha1.PhysicalGPU{}).
		Build()

	handler := NewApplyHandler(service.NewClientStore(cl))
	st := state.New("node-1")
	devices := []state.Device{
		{
			Address:    "0000:01:00.0",
			ClassCode:  "0300",
			Index:      "0",
			VendorID:   "10de",
			DeviceID:   "1eb8",
			DeviceName: "GA100GL [A30 PCIe]",
		},
	}
	st.SetDevices(devices)

	if err := handler.Handle(context.Background(), st); err != nil {
		t.Fatalf("handle: %v", err)
	}

	name := state.PhysicalGPUName("node-1", devices[0])
	obj := &gpuv1alpha1.PhysicalGPU{}
	if err := cl.Get(context.Background(), client.ObjectKey{Name: name}, obj); err != nil {
		t.Fatalf("get PhysicalGPU: %v", err)
	}

	if obj.Labels[state.LabelNode] != "node-1" {
		t.Fatalf("expected node label, got %q", obj.Labels[state.LabelNode])
	}
	if obj.Labels[state.LabelVendor] != "nvidia" {
		t.Fatalf("expected vendor label, got %q", obj.Labels[state.LabelVendor])
	}
	if obj.Labels[state.LabelDevice] != "a30-pcie" {
		t.Fatalf("expected device label, got %q", obj.Labels[state.LabelDevice])
	}

	if obj.Status.NodeInfo == nil || obj.Status.NodeInfo.NodeName != "node-1" {
		t.Fatalf("expected status nodeInfo.nodeName node-1, got %#v", obj.Status.NodeInfo)
	}
	if obj.Status.PCIInfo == nil || obj.Status.PCIInfo.Address != "0000:01:00.0" {
		t.Fatalf("unexpected PCI address %#v", obj.Status.PCIInfo)
	}

}
