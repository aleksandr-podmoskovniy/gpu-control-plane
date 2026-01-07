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
	"context"
	"fmt"
	"net/http"
	"strings"

	admissionv1 "k8s.io/api/admission/v1"
	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/deckhouse/pkg/log"

	configapi "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/configapi"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/logger"
)

// ParametersHandler validates device configuration parameters for DRA claims.
type ParametersHandler struct {
	log        *log.Logger
	driverName string
}

// NewParametersHandler builds a validator for claim configuration parameters.
func NewParametersHandler(log *log.Logger, driverName string) *ParametersHandler {
	return &ParametersHandler{
		log:        log,
		driverName: driverName,
	}
}

// Handle validates ResourceClaim and ResourceClaimTemplate objects.
func (h *ParametersHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	if req.Operation != admissionv1.Create && req.Operation != admissionv1.Update {
		return admission.Allowed("operation ignored")
	}

	var configs []resourcev1.DeviceClaimConfiguration
	specPath := "spec"

	switch req.Resource {
	case resourceClaimResourceV1, resourceClaimResourceV1Beta1, resourceClaimResourceV1Beta2:
		claim, err := extractResourceClaim(req)
		if err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		configs = claim.Spec.Devices.Config
	case resourceClaimTemplateResourceV1, resourceClaimTemplateResourceV1Beta1, resourceClaimTemplateResourceV1Beta2:
		template, err := extractResourceClaimTemplate(req)
		if err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		configs = template.Spec.Spec.Devices.Config
		specPath = "spec.spec"
	default:
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("unsupported resource: %s", req.Resource))
	}

	if len(configs) == 0 {
		return admission.Allowed("no device configurations")
	}

	errs := validateDeviceConfigs(configs, specPath, h.driverName)
	if len(errs) == 0 {
		return admission.Allowed("device configurations are valid")
	}

	messages := make([]string, 0, len(errs))
	for _, err := range errs {
		messages = append(messages, err.Error())
	}
	msg := fmt.Sprintf("%d configs failed to validate: %s", len(messages), strings.Join(messages, "; "))

	if h.log != nil {
		attrs := []any{
			"errors", len(messages),
			"resource", req.Resource.String(),
			"namespace", req.Namespace,
			"name", req.Name,
		}
		h.log.Warn("invalid DRA device configuration", attrs...)
	}

	return admission.Denied(msg)
}

func validateDeviceConfigs(configs []resourcev1.DeviceClaimConfiguration, specPath, driverName string) []error {
	var errs []error
	for idx, cfg := range configs {
		opaque := cfg.Opaque
		if opaque == nil || opaque.Driver != driverName {
			continue
		}

		fieldPath := fmt.Sprintf("%s.devices.config[%d].opaque.parameters", specPath, idx)
		decoded, err := runtime.Decode(configapi.StrictDecoder, opaque.Parameters.Raw)
		if err != nil {
			errs = append(errs, fmt.Errorf("error decoding object at %s: %w", fieldPath, err))
			continue
		}

		config, ok := decoded.(configapi.Interface)
		if !ok {
			errs = append(errs, fmt.Errorf("expected a recognized configuration type at %s but got: %T", fieldPath, decoded))
			continue
		}

		if err := config.Normalize(); err != nil {
			errs = append(errs, fmt.Errorf("error normalizing config at %s: %w", fieldPath, err))
			continue
		}
		if err := config.Validate(); err != nil {
			errs = append(errs, fmt.Errorf("object at %s is invalid: %w", fieldPath, err))
		}
	}
	return errs
}

var _ admission.Handler = (*ParametersHandler)(nil)

func ensureLog(l *log.Logger) *log.Logger {
	if l == nil {
		l = log.NewNop()
	}
	return l.With(logger.SlogHandler("dra-claim-webhook"))
}

func newHandler(log *log.Logger, driverName string) *ParametersHandler {
	return NewParametersHandler(ensureLog(log), driverName)
}
