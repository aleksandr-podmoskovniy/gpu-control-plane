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

package inventory

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/config"
)

func TestNewDeviceApprovalPolicyDefaults(t *testing.T) {
	policy, err := newDeviceApprovalPolicy(config.DeviceApprovalSettings{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if policy.mode != config.DeviceApprovalModeManual {
		t.Fatalf("expected manual mode fallback, got %s", policy.mode)
	}
	if policy.AutoAttach(true, labels.Set{}) {
		t.Fatal("manual mode should never auto attach")
	}
}

func TestNewDeviceApprovalPolicySelector(t *testing.T) {
	policy, err := newDeviceApprovalPolicy(config.DeviceApprovalSettings{
		Mode: config.DeviceApprovalModeSelector,
		Selector: &metav1.LabelSelector{
			MatchLabels: map[string]string{"gpu.deckhouse.io/device.vendor": "10de"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !policy.AutoAttach(true, labels.Set{"gpu.deckhouse.io/device.vendor": "10de"}) {
		t.Fatal("selector should match when label present")
	}
	if policy.AutoAttach(true, labels.Set{"gpu.deckhouse.io/device.vendor": "1234"}) {
		t.Fatal("selector should not match other vendor")
	}
}

func TestNewDeviceApprovalPolicySelectorCompileError(t *testing.T) {
	_, err := newDeviceApprovalPolicy(config.DeviceApprovalSettings{
		Mode: config.DeviceApprovalModeSelector,
		Selector: &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{Key: "gpu.deckhouse.io/device.vendor", Operator: "Invalid"},
			},
		},
	})
	if err == nil {
		t.Fatal("expected selector compile error")
	}
}

func TestNewDeviceApprovalPolicyUnknownMode(t *testing.T) {
	policy, err := newDeviceApprovalPolicy(config.DeviceApprovalSettings{
		Mode: "something-unsupported",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if policy.mode != config.DeviceApprovalModeManual {
		t.Fatalf("expected manual fallback, got %s", policy.mode)
	}
}

func TestDeviceApprovalAutoAttachManagedFlag(t *testing.T) {
	policy, err := newDeviceApprovalPolicy(config.DeviceApprovalSettings{
		Mode: config.DeviceApprovalModeAutomatic,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if policy.AutoAttach(false, labels.Set{}) {
		t.Fatal("auto attach must be false when node is unmanaged")
	}
}

func TestDeviceApprovalAutoAttachSelectorWithoutCompiledSelector(t *testing.T) {
	policy := DeviceApprovalPolicy{mode: config.DeviceApprovalModeSelector, selector: nil}
	if policy.AutoAttach(true, labels.Set{"gpu.deckhouse.io/device.vendor": "10de"}) {
		t.Fatal("expected selector without compiled matcher to return false")
	}
}

func TestLabelsForDeviceAggregatesAttributes(t *testing.T) {
	snapshot := deviceSnapshot{
		Index:     "0",
		Vendor:    "10DE",
		Device:    "2230",
		Class:     "0302",
		Product:   "NVIDIA RTX 6000 Ada",
		UUID:      "DEVICE-UUID",
		MemoryMiB: 49152,
		MIG: gpuv1alpha1.GPUMIGConfig{
			Capable:  true,
			Strategy: gpuv1alpha1.GPUMIGStrategyMixed,
		},
	}

	nodeLabels := map[string]string{
		"gpu.deckhouse.io/device.00.vendor": "10de",
		"gpu.deckhouse.io/device.00.device": "2230",
	}

	result := labelsForDevice(snapshot, nodeLabels)
	if result["gpu.deckhouse.io/device.index"] != "0" {
		t.Fatalf("unexpected index label: %s", result["gpu.deckhouse.io/device.index"])
	}
	if result["gpu.deckhouse.io/device.vendor"] != "10de" {
		t.Fatalf("vendor should be normalized to lowercase, got %s", result["gpu.deckhouse.io/device.vendor"])
	}
	if result["gpu.deckhouse.io/device.product"] != "NVIDIA RTX 6000 Ada" {
		t.Fatalf("unexpected product: %s", result["gpu.deckhouse.io/device.product"])
	}
	if result["gpu.deckhouse.io/device.uuid"] != "DEVICE-UUID" {
		t.Fatalf("unexpected uuid: %s", result["gpu.deckhouse.io/device.uuid"])
	}
	if result["gpu.deckhouse.io/device.mig.capable"] != "true" {
		t.Fatalf("expected mig capable true, got %s", result["gpu.deckhouse.io/device.mig.capable"])
	}
	if result["gpu.deckhouse.io/device.memoryMiB"] != "49152" {
		t.Fatalf("unexpected memory label: %s", result["gpu.deckhouse.io/device.memoryMiB"])
	}
}

func TestLabelsForDevicePreservesIndexedLabels(t *testing.T) {
	snapshot := deviceSnapshot{Index: "03"}
	nodeLabels := map[string]string{
		"gpu.deckhouse.io/device.03.vendor": "10de",
		"gpu.deckhouse.io/device.03.custom": "value",
	}
	labels := labelsForDevice(snapshot, nodeLabels)
	if labels["gpu.deckhouse.io/device.03.custom"] != "value" {
		t.Fatalf("expected indexed label to be preserved, got %s", labels["gpu.deckhouse.io/device.03.custom"])
	}
}
func TestLabelsForDeviceMarksMigCapabilityFalse(t *testing.T) {
	result := labelsForDevice(deviceSnapshot{Index: "0"}, map[string]string{})
	if result["gpu.deckhouse.io/device.mig.capable"] != "false" {
		t.Fatalf("expected mig capable label to be false, got %s", result["gpu.deckhouse.io/device.mig.capable"])
	}
}
