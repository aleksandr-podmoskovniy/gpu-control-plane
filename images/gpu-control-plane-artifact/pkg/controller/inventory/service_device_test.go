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

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
	cpmetrics "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/monitoring/metrics"
)

type namedErrorHandler struct {
	name string
	err  error
}

func (h namedErrorHandler) Name() string { return h.name }

func (h namedErrorHandler) HandleDevice(context.Context, *v1alpha1.GPUDevice) (contracts.Result, error) {
	return contracts.Result{}, h.err
}

func TestDeviceServiceInvokeHandlersIncrementsErrorMetric(t *testing.T) {
	handler := namedErrorHandler{name: "error-" + t.Name(), err: errors.New("boom")}

	svc := &deviceService{handlers: []contracts.InventoryHandler{handler}}
	if _, err := svc.invokeHandlers(context.Background(), &v1alpha1.GPUDevice{}); err == nil {
		t.Fatalf("expected handler error")
	}

	metric, ok := findMetric(t, cpmetrics.InventoryHandlerErrorsTotal, map[string]string{"handler": handler.name})
	if !ok || metric.Counter == nil || metric.Counter.GetValue() != 1 {
		value := 0.0
		if ok && metric.Counter != nil {
			value = metric.Counter.GetValue()
		}
		t.Fatalf("expected handler errors counter=1, got %f (present=%t)", value, ok)
	}
}
