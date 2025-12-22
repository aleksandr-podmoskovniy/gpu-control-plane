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

package bootstrap

import (
	"context"
	"testing"

	promdto "github.com/prometheus/client_model/go"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	crmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/validation"
)

func labelsMatch(metric *promdto.Metric, expected map[string]string) bool {
	for name, want := range expected {
		found := false
		for _, pair := range metric.Label {
			if pair.GetName() != name {
				continue
			}
			found = true
			if pair.GetValue() != want {
				return false
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func findMetric(t *testing.T, name string, labels map[string]string) (*promdto.Metric, bool) {
	t.Helper()

	families, err := crmetrics.Registry.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}

	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.Metric {
			if labelsMatch(metric, labels) {
				return metric, true
			}
		}
		return nil, false
	}

	return nil, false
}

func counterValue(t *testing.T, name string, labels map[string]string) (float64, bool) {
	t.Helper()

	metric, ok := findMetric(t, name, labels)
	if !ok || metric.Counter == nil {
		return 0, false
	}
	return metric.Counter.GetValue(), true
}

func gaugeValue(t *testing.T, name string, labels map[string]string) (float64, bool) {
	t.Helper()

	metric, ok := findMetric(t, name, labels)
	if !ok || metric.Gauge == nil {
		return 0, false
	}
	return metric.Gauge.GetValue(), true
}

type stubBootstrapHandler struct {
	name   string
	result reconcile.Result
	err    error
	calls  int
}

func (s *stubBootstrapHandler) Name() string { return s.name }

func (s *stubBootstrapHandler) HandleNode(context.Context, *v1alpha1.GPUNodeState) (reconcile.Result, error) {
	s.calls++
	return s.result, s.err
}

type stubValidator struct {
	statusCalls int
}

func (s *stubValidator) Status(context.Context, string) (validation.Result, error) {
	s.statusCalls++
	return validation.Result{}, nil
}

type capturingValidator struct {
	statusCalls int
	nodeName    string
	result      validation.Result
	err         error
}

func (v *capturingValidator) Status(_ context.Context, nodeName string) (validation.Result, error) {
	v.statusCalls++
	v.nodeName = nodeName
	return v.result, v.err
}

type statusChangingHandler struct{}

func (statusChangingHandler) Name() string { return "status-changer" }

func (statusChangingHandler) HandleNode(_ context.Context, inventory *v1alpha1.GPUNodeState) (reconcile.Result, error) {
	inventory.Status.Conditions = append(inventory.Status.Conditions, metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue})
	return reconcile.Result{}, nil
}

type statusReadingHandler struct {
	present bool
	status  validation.Result
}

func (h *statusReadingHandler) Name() string { return "status-reader" }

func (h *statusReadingHandler) HandleNode(ctx context.Context, _ *v1alpha1.GPUNodeState) (reconcile.Result, error) {
	h.status, h.present = validation.StatusFromContext(ctx)
	return reconcile.Result{}, nil
}

type failingClient struct {
	client.Client
	err error
}

func (f *failingClient) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	return f.err
}

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add apps scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	return scheme
}
