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

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlwebhook "sigs.k8s.io/controller-runtime/pkg/webhook"
	cradmission "sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/config"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
)

// Register initialises webhook handlers and attaches them to the controller-runtime webhook server.
func Register(_ context.Context, mgr webhookManager, deps Dependencies) error {
	server := mgr.GetWebhookServer()
	if server == nil {
		return nil
	}

	decoder := cradmission.NewDecoder(mgr.GetScheme())

	poolValidator := newGPUPoolValidator(deps.Logger, decoder, deps.AdmissionHandlers.List())
	server.Register("/validate-gpu-deckhouse-io-v1alpha1-gpupool", &ctrlwebhook.Admission{Handler: poolValidator})

	deviceValidator := newGPUDeviceAssignmentValidator(deps.Logger, decoder, deps.Client)
	server.Register("/validate-gpu-deckhouse-io-v1alpha1-gpudevice", &ctrlwebhook.Admission{Handler: deviceValidator})

	poolDefaulter := newGPUPoolDefaulter(deps.Logger, decoder, deps.AdmissionHandlers.List())
	server.Register("/mutate-gpu-deckhouse-io-v1alpha1-gpupool", &ctrlwebhook.Admission{Handler: poolDefaulter})

	podMutator := newPodMutator(deps.Logger, decoder, deps.ModuleConfigStore, deps.Client)
	server.Register("/mutate-v1-pod-gpupool", &ctrlwebhook.Admission{Handler: podMutator})
	podValidator := newPodValidator(deps.Logger, decoder, deps.ModuleConfigStore, deps.Client)
	server.Register("/validate-v1-pod-gpupool", &ctrlwebhook.Admission{Handler: podValidator})

	return nil
}

// Dependencies is a narrow projection of controller dependencies needed here.
type Dependencies struct {
	Logger            logr.Logger
	AdmissionHandlers *contracts.AdmissionRegistry
	Client            client.Client
	ModuleConfigStore *config.ModuleConfigStore
}

type webhookManager interface {
	GetWebhookServer() ctrlwebhook.Server
	GetScheme() *runtime.Scheme
}
