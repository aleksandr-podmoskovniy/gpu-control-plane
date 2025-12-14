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
	"context"
	"errors"
	"testing"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
)

func TestDetectionCollectorCollectListError(t *testing.T) {
	listErr := errors.New("list failed")
	collector := &detectionCollector{client: &failingListClient{err: listErr}}
	if _, err := collector.Collect(context.Background(), "node"); !errors.Is(err, listErr) {
		t.Fatalf("expected list error, got %v", err)
	}
}

func TestApplyDetectionHardwareMIGModeAndProfilesBranches(t *testing.T) {
	device := &v1alpha1.GPUDevice{}

	applyDetectionHardware(device, detectGPUEntry{
		MIG: detectGPUMIG{
			Capable:           false,
			Mode:              " SINGLE ",
			ProfilesSupported: []string{" MIG-1g.10gb ", "", "mig-1g.10gb", "2g.20gb"},
		},
	})

	if device.Status.Hardware.MIG.Strategy != v1alpha1.GPUMIGStrategySingle {
		t.Fatalf("expected MIG strategy Single, got %v", device.Status.Hardware.MIG.Strategy)
	}
	if !device.Status.Hardware.MIG.Capable {
		t.Fatalf("expected MIG to become capable when profiles are present")
	}
	if len(device.Status.Hardware.MIG.ProfilesSupported) != 2 ||
		device.Status.Hardware.MIG.ProfilesSupported[0] != "1g.10gb" ||
		device.Status.Hardware.MIG.ProfilesSupported[1] != "2g.20gb" {
		t.Fatalf("unexpected normalized profiles: %+v", device.Status.Hardware.MIG.ProfilesSupported)
	}

	device2 := &v1alpha1.GPUDevice{}
	applyDetectionHardware(device2, detectGPUEntry{MIG: detectGPUMIG{Mode: "mixed"}})
	if device2.Status.Hardware.MIG.Strategy != v1alpha1.GPUMIGStrategyMixed {
		t.Fatalf("expected MIG strategy Mixed, got %v", device2.Status.Hardware.MIG.Strategy)
	}

	device3 := &v1alpha1.GPUDevice{}
	applyDetectionHardware(device3, detectGPUEntry{MIG: detectGPUMIG{Mode: "unknown"}})
	if device3.Status.Hardware.MIG.Strategy != v1alpha1.GPUMIGStrategyNone {
		t.Fatalf("expected MIG strategy None, got %v", device3.Status.Hardware.MIG.Strategy)
	}
}

