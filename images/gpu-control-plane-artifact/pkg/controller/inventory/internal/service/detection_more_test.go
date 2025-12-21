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

package service

import (
	"context"
	"errors"
	"testing"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	invstate "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory/internal/state"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestCollectNodeDetectionsListError(t *testing.T) {
	scheme := newTestScheme(t)
	base := newTestClient(t, scheme)

	boom := errors.New("list failed")
	cl := &cleanupDelegatingClient{
		Client: base,
		list: func(context.Context, client.ObjectList, ...client.ListOption) error {
			return boom
		},
	}

	collector := NewDetectionCollector(cl)
	if _, err := collector.Collect(context.Background(), "node"); !errors.Is(err, boom) {
		t.Fatalf("expected error %v, got %v", boom, err)
	}
}

func TestApplyDetectionHardwareMIGNormalization(t *testing.T) {
	device := &v1alpha1.GPUDevice{
		Status: v1alpha1.GPUDeviceStatus{
			Hardware: v1alpha1.GPUDeviceHardware{
				MIG: v1alpha1.GPUMIGConfig{
					Capable:  false,
					Strategy: v1alpha1.GPUMIGStrategyMixed,
				},
			},
		},
	}

	entry := detectGPUEntry{
		MIG: detectGPUMIG{
			Capable:           false,
			Mode:              " single ",
			ProfilesSupported: []string{"", "mig-2g.20gb", "MIG-1g.10gb", "mig-2g.20gb", " 1g.10gb "},
		},
		PCI: detectGPUPCI{
			Address: "00000000:65:00.0",
			Vendor:  "10DE",
			Device:  "2203",
			Class:   "0302",
		},
	}
	applyDetectionHardware(device, entry)

	if device.Status.Hardware.MIG.Strategy != v1alpha1.GPUMIGStrategySingle {
		t.Fatalf("expected strategy=%s, got %s", v1alpha1.GPUMIGStrategySingle, device.Status.Hardware.MIG.Strategy)
	}
	if !device.Status.Hardware.MIG.Capable {
		t.Fatalf("expected capable=true when profiles are present")
	}
	if got := device.Status.Hardware.MIG.ProfilesSupported; len(got) != 2 || got[0] != "1g.10gb" || got[1] != "2g.20gb" {
		t.Fatalf("unexpected profiles: %+v", got)
	}
	if device.Status.Hardware.PCI.Address != "0000:65:00.0" {
		t.Fatalf("expected canonical pci address, got %q", device.Status.Hardware.PCI.Address)
	}

	for _, tc := range []struct {
		mode string
		want v1alpha1.GPUMIGStrategy
	}{
		{mode: "mixed", want: v1alpha1.GPUMIGStrategyMixed},
		{mode: "none", want: v1alpha1.GPUMIGStrategyNone},
	} {
		device.Status.Hardware.MIG.Strategy = ""
		entry.MIG.Mode = tc.mode
		applyDetectionHardware(device, entry)
		if device.Status.Hardware.MIG.Strategy != tc.want {
			t.Fatalf("expected strategy=%s for mode=%s, got %s", tc.want, tc.mode, device.Status.Hardware.MIG.Strategy)
		}
	}

	device.Status.Hardware.MIG.Strategy = v1alpha1.GPUMIGStrategyMixed
	entry.MIG.Mode = "unknown"
	applyDetectionHardware(device, entry)
	if device.Status.Hardware.MIG.Strategy != v1alpha1.GPUMIGStrategyMixed {
		t.Fatalf("expected unknown mode to preserve existing strategy, got %s", device.Status.Hardware.MIG.Strategy)
	}
}

func TestApplyDetectionDoesNotOverridePCIFieldsWhenAlreadySet(t *testing.T) {
	device := &v1alpha1.GPUDevice{
		Status: v1alpha1.GPUDeviceStatus{
			Hardware: v1alpha1.GPUDeviceHardware{
				PCI: v1alpha1.PCIAddress{
					Vendor: "10de",
					Device: "2203",
					Class:  "0302",
				},
			},
		},
	}

	ApplyDetection(device, invstate.DeviceSnapshot{Index: "0"}, NodeDetection{
		byIndex: map[string]detectGPUEntry{
			"0": {
				Index: 0,
				PCI:   detectGPUPCI{Vendor: "ffff", Device: "ffff", Class: "ffff"},
			},
		},
	})

	if device.Status.Hardware.PCI.Vendor != "10de" || device.Status.Hardware.PCI.Device != "2203" || device.Status.Hardware.PCI.Class != "0302" {
		t.Fatalf("pci fields must not be overwritten: %+v", device.Status.Hardware.PCI)
	}
}
