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
	"fmt"
	"net/http"
	"strings"

	"github.com/go-logr/logr"
	admv1 "k8s.io/api/admission/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	cradmission "sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/handlers/gpupool"
)

const assignmentAnnotation = "gpu.deckhouse.io/assignment"

type gpuDeviceAssignmentValidator struct {
	log     logr.Logger
	decoder cradmission.Decoder
	client  client.Client
}

func newGPUDeviceAssignmentValidator(log logr.Logger, decoder cradmission.Decoder, c client.Client) cradmission.Handler {
	return &gpuDeviceAssignmentValidator{
		log:     log.WithName("gpu-device-assignment"),
		decoder: decoder,
		client:  c,
	}
}

func (h *gpuDeviceAssignmentValidator) Handle(ctx context.Context, req cradmission.Request) cradmission.Response {
	if req.Operation != admv1.Create && req.Operation != admv1.Update {
		return cradmission.Allowed("")
	}

	device := &v1alpha1.GPUDevice{}
	if err := h.decoder.Decode(req, device); err != nil {
		return cradmission.Errored(http.StatusUnprocessableEntity, err)
	}

	if device.Status.State != v1alpha1.GPUDeviceStateReady {
		return cradmission.Denied(fmt.Sprintf("device state must be Ready, got %s", device.Status.State))
	}

	if strings.EqualFold(device.Labels["gpu.deckhouse.io/ignore"], "true") ||
		strings.EqualFold(device.Annotations["gpu.deckhouse.io/ignore"], "true") {
		return cradmission.Denied("device is marked as ignored")
	}

	poolName := strings.TrimSpace(device.Annotations[assignmentAnnotation])
	if poolName == "" {
		return cradmission.Allowed("")
	}

	pool := &v1alpha1.GPUPool{}
	if err := h.client.Get(ctx, types.NamespacedName{Name: poolName}, pool); err != nil {
		if apierrors.IsNotFound(err) {
			return cradmission.Denied(fmt.Sprintf("assigned pool %q not found", poolName))
		}
		return cradmission.Errored(http.StatusInternalServerError, err)
	}

	if matchesPool(device, pool) {
		return cradmission.Allowed("")
	}

	return cradmission.Denied(fmt.Sprintf("device does not match selector of pool %q", pool.Name))
}

func matchesPool(dev *v1alpha1.GPUDevice, pool *v1alpha1.GPUPool) bool {
	nodeDev := v1alpha1.GPUNodeDevice{
		InventoryID: dev.Status.InventoryID,
		Product:     dev.Status.Hardware.Product,
		PCI: v1alpha1.PCIAddress{
			Vendor: dev.Status.Hardware.PCI.Vendor,
			Device: dev.Status.Hardware.PCI.Device,
			Class:  dev.Status.Hardware.PCI.Class,
		},
		MIG: v1alpha1.GPUMIGConfig{
			Capable:           dev.Status.Hardware.MIG.Capable,
			ProfilesSupported: migProfiles(dev.Status.Hardware.MIG),
		},
	}

	candidates := gpupool.FilterDevices([]v1alpha1.GPUNodeDevice{nodeDev}, pool.Spec.DeviceSelector)
	return len(candidates) == 1
}

func migProfiles(m v1alpha1.GPUMIGConfig) []string {
	if len(m.ProfilesSupported) > 0 {
		return m.ProfilesSupported
	}
	profiles := make([]string, 0, len(m.Types))
	for _, t := range m.Types {
		if t.Name != "" {
			profiles = append(profiles, t.Name)
		}
	}
	return profiles
}
