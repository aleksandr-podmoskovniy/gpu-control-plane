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

package webhook

import (
	"fmt"
	"strings"

	"github.com/deckhouse/deckhouse/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	draallocator "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/services/allocator"
)

const ValidateResourceClaimParametersPath = "/validate-resource-claim-parameters"

// Config defines webhook configuration.
type Config struct {
	DriverName string
}

// SetupWithManager registers the webhook handler in the controller-runtime server.
func SetupWithManager(mgr manager.Manager, log *log.Logger, cfg Config) error {
	if mgr == nil {
		return fmt.Errorf("manager is nil")
	}

	driverName := strings.TrimSpace(cfg.DriverName)
	if driverName == "" {
		driverName = draallocator.DefaultDriverName
	}

	handler := newHandler(log, driverName)
	mgr.GetWebhookServer().Register(ValidateResourceClaimParametersPath, &admission.Webhook{Handler: handler})

	if log != nil {
		log.Info("DRA webhook registered", "path", ValidateResourceClaimParametersPath, "driver", driverName)
	}
	return nil
}
