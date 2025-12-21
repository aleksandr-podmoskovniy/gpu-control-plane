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
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/config"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
	invconsts "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory/internal/consts"
	invstate "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory/internal/state"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/moduleconfig"
	promdto "github.com/prometheus/client_model/go"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	crmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"

	nfdv1alpha1 "sigs.k8s.io/node-feature-discovery/api/nfd/v1alpha1"
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

func gaugeValue(t *testing.T, name string, labels map[string]string) (float64, bool) {
	t.Helper()

	metric, ok := findMetric(t, name, labels)
	if !ok || metric.Gauge == nil {
		return 0, false
	}
	return metric.Gauge.GetValue(), true
}

type trackingHandler struct {
	name    string
	state   v1alpha1.GPUDeviceState
	result  contracts.Result
	handled []string
}

type failingListClient struct {
	client.Client
	err error
}

func (f *failingListClient) List(context.Context, client.ObjectList, ...client.ListOption) error {
	return f.err
}

type listErrorClient struct {
	client.Client
	err          error
	failOnSecond bool
	calls        int
}

func (c *listErrorClient) List(ctx context.Context, obj client.ObjectList, opts ...client.ListOption) error {
	c.calls++
	if c.failOnSecond && c.calls == 2 {
		return c.err
	}
	if !c.failOnSecond {
		return c.err
	}
	return c.Client.List(ctx, obj, opts...)
}

type delegatingClient struct {
	client.Client
	get          func(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error
	list         func(context.Context, client.ObjectList, ...client.ListOption) error
	delete       func(context.Context, client.Object, ...client.DeleteOption) error
	create       func(context.Context, client.Object, ...client.CreateOption) error
	patch        func(context.Context, client.Object, client.Patch, ...client.PatchOption) error
	statusWriter client.StatusWriter
}

func (d *delegatingClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if d.get != nil {
		return d.get(ctx, key, obj, opts...)
	}
	return d.Client.Get(ctx, key, obj, opts...)
}

func (d *delegatingClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if d.list != nil {
		return d.list(ctx, list, opts...)
	}
	return d.Client.List(ctx, list, opts...)
}

func (d *delegatingClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	if d.delete != nil {
		return d.delete(ctx, obj, opts...)
	}
	return d.Client.Delete(ctx, obj, opts...)
}

func (d *delegatingClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if d.create != nil {
		return d.create(ctx, obj, opts...)
	}
	return d.Client.Create(ctx, obj, opts...)
}

func (d *delegatingClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	if d.patch != nil {
		return d.patch(ctx, obj, patch, opts...)
	}
	return d.Client.Patch(ctx, obj, patch, opts...)
}

func (d *delegatingClient) Status() client.StatusWriter {
	if d.statusWriter != nil {
		return d.statusWriter
	}
	return d.Client.Status()
}

type errorStatusWriter struct {
	client.StatusWriter
	err error
}

func (w *errorStatusWriter) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	return w.err
}

type trackingStatusWriter struct {
	client.StatusWriter
	patches int
}

func (w *trackingStatusWriter) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	w.patches++
	return w.StatusWriter.Patch(ctx, obj, patch, opts...)
}

type conflictStatusUpdater struct {
	client.StatusWriter
	triggered bool
}

func (w *conflictStatusUpdater) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	if !w.triggered {
		w.triggered = true
		return apierrors.NewConflict(schema.GroupResource{Group: "gpu.deckhouse.io", Resource: "gpudevices"}, obj.GetName(), errors.New("status conflict"))
	}
	return w.StatusWriter.Update(ctx, obj, opts...)
}

func (h *trackingHandler) Name() string {
	if h.name != "" {
		return h.name
	}
	return "tracking"
}

func (h *trackingHandler) HandleDevice(_ context.Context, device *v1alpha1.GPUDevice) (contracts.Result, error) {
	h.handled = append(h.handled, device.Name)
	if h.state != "" {
		device.Status.State = h.state
	}
	return h.result, nil
}

func defaultModuleSettings() config.ModuleSettings {
	return config.DefaultSystem().Module
}

func moduleStoreFrom(settings config.ModuleSettings) *moduleconfig.ModuleConfigStore {
	state, err := config.ModuleSettingsToState(settings)
	if err != nil {
		panic(err)
	}
	return moduleconfig.NewModuleConfigStore(state)
}

func managedPolicyFrom(module config.ModuleSettings) invstate.ManagedNodesPolicy {
	return invstate.ManagedNodesPolicy{
		LabelKey:         module.ManagedNodes.LabelKey,
		EnabledByDefault: module.ManagedNodes.EnabledByDefault,
	}
}

func getCondition(conditions []metav1.Condition, condType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == condType {
			return &conditions[i]
		}
	}
	return nil
}

func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add gpu scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	if err := nfdv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add nfd scheme: %v", err)
	}
	scheme.AddKnownTypes(
		nfdv1alpha1.SchemeGroupVersion,
		&nfdv1alpha1.NodeFeatureList{},
		&nfdv1alpha1.NodeFeatureRuleList{},
		&nfdv1alpha1.NodeFeatureGroupList{},
	)
	return scheme
}

func newTestClient(scheme *runtime.Scheme, objs ...client.Object) client.Client {
	builder := clientfake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUDevice{}, &v1alpha1.GPUNodeState{}).
		WithObjects(objs...)

	builder = builder.WithIndex(&v1alpha1.GPUDevice{}, invconsts.DeviceNodeIndexKey, func(obj client.Object) []string {
		device, ok := obj.(*v1alpha1.GPUDevice)
		if !ok || device.Status.NodeName == "" {
			return nil
		}
		return []string{device.Status.NodeName}
	})

	return builder.Build()
}
