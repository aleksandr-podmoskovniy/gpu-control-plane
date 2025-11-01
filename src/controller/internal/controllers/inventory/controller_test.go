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
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr/testr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/config"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"

	nfdv1alpha1 "sigs.k8s.io/node-feature-discovery/pkg/apis/nfd/v1alpha1"
)

type trackingHandler struct {
	name    string
	state   gpuv1alpha1.GPUDeviceState
	result  contracts.Result
	handled []string
}

type errorHandler struct {
	err error
}

func (errorHandler) Name() string {
	return "error"
}

func (h errorHandler) HandleDevice(context.Context, *gpuv1alpha1.GPUDevice) (contracts.Result, error) {
	return contracts.Result{}, h.err
}

type fakeFieldIndexer struct {
	lastObject client.Object
	lastField  string
}

func (f *fakeFieldIndexer) IndexField(_ context.Context, obj client.Object, field string, extract client.IndexerFunc) error {
	f.lastObject = obj
	f.lastField = field
	if extract != nil {
		extract(obj)
	}
	return nil
}

type capturingFieldIndexer struct {
	values [][]string
}

func (c *capturingFieldIndexer) IndexField(_ context.Context, obj client.Object, field string, extract client.IndexerFunc) error {
	_ = field
	if device, ok := obj.(*gpuv1alpha1.GPUDevice); ok {
		device.Status.NodeName = "worker-indexed"
	}
	if extract != nil {
		vals := extract(obj)
		c.values = append(c.values, vals)
	}
	return nil
}

type fakeControllerBuilder struct {
	name          string
	forObjects    []client.Object
	ownedObjects  []client.Object
	watchedSource source.Source
	options       controller.Options
	completed     bool
	completeErr   error
}

func (b *fakeControllerBuilder) Named(name string) controllerBuilder {
	b.name = name
	return b
}

func (b *fakeControllerBuilder) For(obj client.Object, _ ...builder.ForOption) controllerBuilder {
	b.forObjects = append(b.forObjects, obj)
	return b
}

func (b *fakeControllerBuilder) Owns(obj client.Object, _ ...builder.OwnsOption) controllerBuilder {
	b.ownedObjects = append(b.ownedObjects, obj)
	return b
}

func (b *fakeControllerBuilder) WatchesRawSource(src source.Source) controllerBuilder {
	b.watchedSource = src
	return b
}

func (b *fakeControllerBuilder) WithOptions(opts controller.Options) controllerBuilder {
	b.options = opts
	return b
}

func (b *fakeControllerBuilder) Complete(reconcile.Reconciler) error {
	b.completed = true
	if b.completeErr != nil {
		return b.completeErr
	}
	return nil
}

type fakeSyncingSource struct {
	source.SyncingSource
}

type adapterFromControllerBuilder struct {
	builder *fakeControllerBuilder
}

func (a *adapterFromControllerBuilder) Named(name string) controllerRuntimeAdapter {
	a.builder.Named(name)
	return a
}

func (a *adapterFromControllerBuilder) For(obj client.Object, opts ...builder.ForOption) controllerRuntimeAdapter {
	a.builder.For(obj, opts...)
	return a
}

func (a *adapterFromControllerBuilder) Owns(obj client.Object, opts ...builder.OwnsOption) controllerRuntimeAdapter {
	a.builder.Owns(obj, opts...)
	return a
}

func (a *adapterFromControllerBuilder) WatchesRawSource(src source.Source) controllerRuntimeAdapter {
	a.builder.WatchesRawSource(src)
	return a
}

func (a *adapterFromControllerBuilder) WithOptions(opts controller.Options) controllerRuntimeAdapter {
	a.builder.WithOptions(opts)
	return a
}

func (a *adapterFromControllerBuilder) Complete(r reconcile.Reconciler) error {
	return a.builder.Complete(r)
}

type errorFieldIndexer struct {
	err error
}

func (e *errorFieldIndexer) IndexField(context.Context, client.Object, string, client.IndexerFunc) error {
	return e.err
}

type multiFieldIndexer struct {
	invocations int
}

func (m *multiFieldIndexer) IndexField(_ context.Context, obj client.Object, field string, extract client.IndexerFunc) error {
	_ = field
	if extract != nil {
		// first invoke with non-GPU object to hit type mismatch branch
		extract(&corev1.Node{})
		// then with GPU device lacking node name to hit empty check
		extract(&gpuv1alpha1.GPUDevice{})
		m.invocations++
	}
	return nil
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

type conflictStatusWriter struct {
	client.StatusWriter
	triggered bool
}

func (w *conflictStatusWriter) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	if !w.triggered {
		w.triggered = true
		return apierrors.NewConflict(schema.GroupResource{Group: "gpu.deckhouse.io", Resource: "gpudevices"}, obj.GetName(), errors.New("conflict"))
	}
	return w.StatusWriter.Patch(ctx, obj, patch, opts...)
}

type errorStatusWriter struct {
	client.StatusWriter
	err error
}

func (w *errorStatusWriter) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	return w.err
}

type errorStatusUpdater struct {
	client.StatusWriter
	err error
}

func (w *errorStatusUpdater) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
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

type stubManager struct {
	ctrl.Manager
	client   client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
	indexer  client.FieldIndexer
	cache    cache.Cache
}

func (m stubManager) GetClient() client.Client                        { return m.client }
func (m stubManager) GetScheme() *runtime.Scheme                      { return m.scheme }
func (m stubManager) GetEventRecorderFor(string) record.EventRecorder { return m.recorder }
func (m stubManager) GetFieldIndexer() client.FieldIndexer            { return m.indexer }
func (m stubManager) GetCache() cache.Cache                           { return m.cache }

func (h *trackingHandler) Name() string {
	if h.name != "" {
		return h.name
	}
	return "tracking"
}

func (h *trackingHandler) HandleDevice(_ context.Context, device *gpuv1alpha1.GPUDevice) (contracts.Result, error) {
	h.handled = append(h.handled, device.Name)
	if h.state != "" {
		device.Status.State = h.state
	}
	return h.result, nil
}

type resultHandler struct {
	name   string
	result contracts.Result
}

func (h resultHandler) Name() string {
	if h.name != "" {
		return h.name
	}
	return "result-handler"
}

func (h resultHandler) HandleDevice(context.Context, *gpuv1alpha1.GPUDevice) (contracts.Result, error) {
	return h.result, nil
}

func defaultModuleSettings() config.ModuleSettings {
	return config.DefaultSystem().Module
}

func managedPolicyFrom(module config.ModuleSettings) ManagedNodesPolicy {
	return ManagedNodesPolicy{
		LabelKey:         module.ManagedNodes.LabelKey,
		EnabledByDefault: module.ManagedNodes.EnabledByDefault,
	}
}

func TestReconcileCreatesDeviceAndInventory(t *testing.T) {
	module := defaultModuleSettings()
	scheme := newTestScheme(t)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-a",
			UID:  types.UID("node-worker-a"),
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "1db5",
				"gpu.deckhouse.io/device.00.class":  "0302",
				"nvidia.com/gpu.product":            "NVIDIA A100-PCIE-40GB",
				"nvidia.com/gpu.memory":             "40536 MiB",
				"nvidia.com/gpu.compute.major":      "8",
				"nvidia.com/gpu.compute.minor":      "0",
				"nvidia.com/mig.capable":            "true",
				"nvidia.com/mig.strategy":           "single",
				"nvidia.com/mig-1g.10gb.count":      "2",
			},
		},
	}

	feature := &nfdv1alpha1.NodeFeature{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-a"},
		Spec: nfdv1alpha1.NodeFeatureSpec{
			Labels: map[string]string{
				"nvidia.com/gpu.driver":          "535.86.05",
				"nvidia.com/cuda.driver.major":   "12",
				"nvidia.com/cuda.driver.minor":   "2",
				"gpu.deckhouse.io/toolkit.ready": "true",
			},
			Features: nfdv1alpha1.Features{
				Instances: map[string]nfdv1alpha1.InstanceFeatureSet{
					"nvidia.com/gpu": {
						Elements: []nfdv1alpha1.InstanceFeature{
							{Attributes: map[string]string{
								"index":          "0",
								"uuid":           "GPU-TEST-UUID-0001",
								"precision":      "fp32,fp16",
								"memory.total":   "40536 MiB",
								"compute.major":  "8",
								"compute.minor":  "0",
								"product":        "NVIDIA A100-PCIE-40GB",
								"precision.bf16": "false",
							}},
						},
					},
				},
			},
		},
	}

	handler := &trackingHandler{
		name:   "state-default",
		state:  gpuv1alpha1.GPUDeviceStateReserved,
		result: contracts.Result{RequeueAfter: 10 * time.Second},
	}

	client := newTestClient(scheme, node, feature)

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, module, []contracts.InventoryHandler{handler})
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	ctx := context.Background()
	res, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}})
	if err != nil {
		t.Fatalf("unexpected reconcile error: %v", err)
	}
	if res.Requeue || res.RequeueAfter != 10*time.Second {
		t.Fatalf("unexpected reconcile result: %+v", res)
	}
	if len(handler.handled) != 1 {
		t.Fatalf("expected handler to be invoked once, got %d", len(handler.handled))
	}

	nodeSnapshot := buildNodeSnapshot(node, feature, managedPolicyFrom(module))
	if len(nodeSnapshot.Devices) != 1 {
		t.Fatalf("expected single snapshot, got %d", len(nodeSnapshot.Devices))
	}
	snapshot := nodeSnapshot.Devices[0]

	deviceName := buildDeviceName(node.Name, snapshot)
	device := &gpuv1alpha1.GPUDevice{}
	if err := client.Get(ctx, types.NamespacedName{Name: deviceName}, device); err != nil {
		t.Fatalf("device not found: %v", err)
	}
	if device.Status.NodeName != node.Name {
		t.Fatalf("expected node name %q, got %q", node.Name, device.Status.NodeName)
	}
	if device.Status.InventoryID == "" {
		t.Fatal("inventoryID must be populated")
	}
	if device.Status.Managed != true {
		t.Fatalf("expected managed true, got %v", device.Status.Managed)
	}
	if device.Status.AutoAttach {
		t.Fatalf("expected autoAttach=false for manual approval, got true")
	}
	if device.Status.State != gpuv1alpha1.GPUDeviceStateReserved {
		t.Fatalf("expected state Reserved, got %s", device.Status.State)
	}
	if device.Status.Hardware.PCI.Vendor != "10de" {
		t.Fatalf("unexpected vendor: %s", device.Status.Hardware.PCI.Vendor)
	}
	if device.Status.Hardware.Product != "NVIDIA A100-PCIE-40GB" {
		t.Fatalf("unexpected product: %s", device.Status.Hardware.Product)
	}
	if device.Status.Hardware.MemoryMiB != 40536 {
		t.Fatalf("unexpected memory: %d", device.Status.Hardware.MemoryMiB)
	}
	if device.Status.Hardware.ComputeCapability == nil {
		t.Fatal("compute capability must be set")
	}
	if device.Status.Hardware.ComputeCapability.Major != 8 || device.Status.Hardware.ComputeCapability.Minor != 0 {
		t.Fatalf("unexpected compute capability: %+v", device.Status.Hardware.ComputeCapability)
	}
	mig := device.Status.Hardware.MIG
	if !mig.Capable {
		t.Fatal("expected MIG capable true")
	}
	if mig.Strategy != gpuv1alpha1.GPUMIGStrategySingle {
		t.Fatalf("unexpected MIG strategy: %s", mig.Strategy)
	}
	if len(mig.ProfilesSupported) != 1 || mig.ProfilesSupported[0] != "mig-1g.10gb" {
		t.Fatalf("unexpected MIG profiles: %+v", mig.ProfilesSupported)
	}
	if len(mig.Types) != 1 || mig.Types[0].Name != "mig-1g.10gb" || mig.Types[0].Count != 2 {
		t.Fatalf("unexpected MIG types: %+v", mig.Types)
	}
	if !stringSlicesEqual(device.Status.Hardware.Precision.Supported, []string{"fp16", "fp32"}) {
		t.Fatalf("unexpected precision list: %+v", device.Status.Hardware.Precision.Supported)
	}

	inventory := &gpuv1alpha1.GPUNodeInventory{}
	if err := client.Get(ctx, types.NamespacedName{Name: node.Name}, inventory); err != nil {
		t.Fatalf("inventory not found: %v", err)
	}
	if inventory.Spec.NodeName != node.Name {
		t.Fatalf("inventory spec node mismatch: %q", inventory.Spec.NodeName)
	}
	if !inventory.Status.Hardware.Present {
		t.Fatal("inventory should mark hardware present")
	}
	if len(inventory.Status.Hardware.Devices) != 1 {
		t.Fatalf("expected 1 device in inventory, got %d", len(inventory.Status.Hardware.Devices))
	}
	if inventory.Status.Hardware.Devices[0].InventoryID != device.Status.InventoryID {
		t.Fatalf("inventory device id mismatch")
	}
	inventoryDevice := inventory.Status.Hardware.Devices[0]
	if inventoryDevice.Product != "NVIDIA A100-PCIE-40GB" {
		t.Fatalf("unexpected inventory product: %s", inventoryDevice.Product)
	}
	if inventoryDevice.MIG.Strategy != gpuv1alpha1.GPUMIGStrategySingle {
		t.Fatalf("unexpected inventory MIG strategy: %s", inventoryDevice.MIG.Strategy)
	}
	if len(inventoryDevice.MIG.Types) != 1 || inventoryDevice.MIG.Types[0].Name != "mig-1g.10gb" || inventoryDevice.MIG.Types[0].Count != 2 {
		t.Fatalf("unexpected inventory MIG types: %+v", inventoryDevice.MIG.Types)
	}
	if inventoryDevice.UUID != "GPU-TEST-UUID-0001" {
		t.Fatalf("unexpected inventory UUID: %s", inventoryDevice.UUID)
	}
	if !stringSlicesEqual(inventoryDevice.Precision.Supported, []string{"fp16", "fp32"}) {
		t.Fatalf("unexpected inventory precision: %+v", inventoryDevice.Precision.Supported)
	}
	if inventory.Status.Driver.Version != "535.86.05" {
		t.Fatalf("unexpected driver version: %s", inventory.Status.Driver.Version)
	}
	if inventory.Status.Driver.CUDAVersion != "12.2" {
		t.Fatalf("unexpected cuda version: %s", inventory.Status.Driver.CUDAVersion)
	}
	if !inventory.Status.Driver.ToolkitReady {
		t.Fatal("expected driver toolkit ready true")
	}
	if cond := getCondition(inventory.Status.Conditions, conditionInventoryIncomplete); cond == nil || cond.Status != metav1.ConditionFalse {
		t.Fatalf("expected InventoryIncomplete=false, got %+v", cond)
	}
	if cond := getCondition(inventory.Status.Conditions, conditionManagedDisabled); cond == nil || cond.Status != metav1.ConditionFalse {
		t.Fatalf("expected ManagedDisabled=false, got %+v", cond)
	}
}

func TestReconcileSchedulesDefaultResync(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-resync"}}

	client := newTestClient(scheme, node)

	module := defaultModuleSettings()
	reconciler, err := New(testr.New(t), config.ControllerConfig{}, module, nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	res, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}})
	if err != nil {
		t.Fatalf("unexpected reconcile error: %v", err)
	}
	if res.Requeue {
		t.Fatalf("expected no immediate requeue, got %+v", res)
	}
	if res.RequeueAfter != defaultResyncPeriod {
		t.Fatalf("expected default resync period %s, got %s", defaultResyncPeriod, res.RequeueAfter)
	}
}

func TestNewAppliesDefaultsAndPolicies(t *testing.T) {
	module := defaultModuleSettings()
	cfg := config.ControllerConfig{Workers: 0, ResyncPeriod: 0}

	rec, err := New(testr.New(t), cfg, module, nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	if rec.cfg.Workers != 1 {
		t.Fatalf("expected workers default to 1, got %d", rec.cfg.Workers)
	}
	if rec.resyncPeriod != defaultResyncPeriod {
		t.Fatalf("expected default resync period %s, got %s", defaultResyncPeriod, rec.resyncPeriod)
	}
	if rec.managed.LabelKey != module.ManagedNodes.LabelKey {
		t.Fatalf("unexpected managed label key %s", rec.managed.LabelKey)
	}
	if rec.approval.mode != module.DeviceApproval.Mode {
		t.Fatalf("unexpected approval mode %s", rec.approval.mode)
	}
}

func TestNewReturnsErrorOnInvalidSelector(t *testing.T) {
	module := defaultModuleSettings()
	module.DeviceApproval.Mode = config.DeviceApprovalModeSelector
	module.DeviceApproval.Selector = &metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{Key: "gpu.deckhouse.io/device.vendor", Operator: metav1.LabelSelectorOperator("Invalid")},
		},
	}

	_, err := New(testr.New(t), config.ControllerConfig{}, module, nil)
	if err == nil {
		t.Fatalf("expected error due to invalid selector")
	}
}

func TestReconcileDeletesOrphansAndUpdatesManagedFlag(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-b",
			UID:  types.UID("node-worker-b"),
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "2230",
				"gpu.deckhouse.io/device.00.class":  "0302",
				"gpu.deckhouse.io/enabled":          "false",
			},
		},
	}

	primary := &gpuv1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "worker-b-0-10de-2230",
			Labels: map[string]string{deviceNodeLabelKey: "worker-b", deviceIndexLabelKey: "0"},
		},
		Status: gpuv1alpha1.GPUDeviceStatus{
			NodeName: "worker-b",
			Managed:  true,
		},
	}
	orphan := &gpuv1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "obsolete-device",
			Labels: map[string]string{deviceNodeLabelKey: "worker-b", deviceIndexLabelKey: "99"},
		},
		Status: gpuv1alpha1.GPUDeviceStatus{
			NodeName: "worker-b",
		},
	}

	client := newTestClient(scheme, node, primary, orphan)

	handler := &trackingHandler{name: "noop"}
	module := defaultModuleSettings()
	reconciler, err := New(testr.New(t), config.ControllerConfig{}, module, []contracts.InventoryHandler{handler})
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	ctx := context.Background()
	if _, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}}); err != nil {
		t.Fatalf("unexpected reconcile error: %v", err)
	}

	if err := client.Get(ctx, types.NamespacedName{Name: orphan.Name}, &gpuv1alpha1.GPUDevice{}); err == nil || !apierrors.IsNotFound(err) {
		t.Fatalf("orphan device should be deleted, got err=%v", err)
	}

	updated := &gpuv1alpha1.GPUDevice{}
	if err := client.Get(ctx, types.NamespacedName{Name: primary.Name}, updated); err != nil {
		t.Fatalf("failed to get primary device: %v", err)
	}
	if updated.Status.Managed {
		t.Fatal("expected managed flag to be false after reconcile")
	}
	if updated.Labels[deviceIndexLabelKey] != "0" {
		t.Fatalf("expected index label to remain 0, got %s", updated.Labels[deviceIndexLabelKey])
	}

	inventory := &gpuv1alpha1.GPUNodeInventory{}
	if err := client.Get(ctx, types.NamespacedName{Name: node.Name}, inventory); err != nil {
		t.Fatalf("inventory missing: %v", err)
	}
	if len(inventory.Status.Hardware.Devices) != 1 {
		t.Fatalf("inventory devices mismatch: %#v", inventory.Status.Hardware)
	}
	if cond := getCondition(inventory.Status.Conditions, conditionManagedDisabled); cond == nil || cond.Status != metav1.ConditionTrue {
		t.Fatalf("expected ManagedDisabled=true, got %+v", cond)
	}
	if cond := getCondition(inventory.Status.Conditions, conditionInventoryIncomplete); cond == nil || cond.Status != metav1.ConditionTrue {
		t.Fatalf("expected InventoryIncomplete=true, got %+v", cond)
	}
}

func TestReconcileDeviceUpdatesMetadata(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-meta",
			UID:  types.UID("node-worker-meta"),
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "1db5",
				"gpu.deckhouse.io/device.00.class":  "0302",
			},
		},
	}
	device := &gpuv1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "worker-meta-0-10de-1db5",
			Labels: map[string]string{},
		},
		Status: gpuv1alpha1.GPUDeviceStatus{},
	}

	baseClient := newTestClient(scheme, node, device)
	var deviceGets int
	client := &delegatingClient{
		Client: baseClient,
		get: func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if _, ok := obj.(*gpuv1alpha1.GPUDevice); ok {
				deviceGets++
			}
			return baseClient.Get(ctx, key, obj, opts...)
		},
	}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, defaultModuleSettings(), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	if _, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}}); err != nil {
		t.Fatalf("unexpected reconcile error: %v", err)
	}
	if deviceGets < 2 {
		t.Fatalf("expected device to be re-fetched after metadata update, got %d", deviceGets)
	}
}

func TestReconcileDeviceOwnerReferenceError(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-owner-error",
			UID:  types.UID("node-owner-error"),
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "1db5",
				"gpu.deckhouse.io/device.00.class":  "0302",
			},
		},
	}
	client := newTestClient(scheme, node)

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, defaultModuleSettings(), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = runtime.NewScheme() // missing core types to force owner reference failure
	reconciler.recorder = record.NewFakeRecorder(32)

	snapshot := deviceSnapshot{Index: "0", Vendor: "10de", Device: "1db5", Class: "0302"}
	_, _, err = reconciler.reconcileDevice(context.Background(), node, snapshot, map[string]string{}, true)
	if err == nil {
		t.Fatalf("expected error due to missing scheme registration")
	}
}

func TestReconcileDeviceGetError(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-get-error",
			UID:  types.UID("node-get-error"),
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "1db5",
				"gpu.deckhouse.io/device.00.class":  "0302",
			},
		},
	}
	baseClient := newTestClient(scheme, node)
	getErr := errors.New("device get failed")
	client := &delegatingClient{
		Client: baseClient,
		get: func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if _, ok := obj.(*gpuv1alpha1.GPUDevice); ok {
				return getErr
			}
			return baseClient.Get(ctx, key, obj, opts...)
		},
	}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, defaultModuleSettings(), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	snapshot := deviceSnapshot{Index: "0", Vendor: "10de", Device: "1db5", Class: "0302"}
	_, _, err = reconciler.reconcileDevice(context.Background(), node, snapshot, map[string]string{}, true)
	if !errors.Is(err, getErr) {
		t.Fatalf("expected device get error, got %v", err)
	}
}

func TestReconcileDeviceMetadataPatchError(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-metadata-error",
			UID:  types.UID("node-metadata-error"),
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "1db5",
				"gpu.deckhouse.io/device.00.class":  "0302",
			},
		},
	}
	device := &gpuv1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "worker-metadata-error-0-10de-1db5",
			Labels: map[string]string{},
		},
		Status: gpuv1alpha1.GPUDeviceStatus{},
	}

	baseClient := newTestClient(scheme, node, device)
	patchErr := errors.New("patch failed")
	client := &delegatingClient{Client: baseClient, patch: func(context.Context, client.Object, client.Patch, ...client.PatchOption) error {
		return patchErr
	}}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, defaultModuleSettings(), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	snapshot := deviceSnapshot{Index: "0", Vendor: "10de", Device: "1db5", Class: "0302"}
	_, _, err = reconciler.reconcileDevice(context.Background(), node, snapshot, map[string]string{}, true)
	if !errors.Is(err, patchErr) {
		t.Fatalf("expected patch error, got %v", err)
	}
}

func TestReconcileDeviceCreateError(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-create-error",
			UID:  types.UID("node-create-error"),
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "1db5",
				"gpu.deckhouse.io/device.00.class":  "0302",
			},
		},
	}

	baseClient := newTestClient(scheme, node)
	createErr := errors.New("create failed")
	client := &delegatingClient{Client: baseClient, create: func(context.Context, client.Object, ...client.CreateOption) error { return createErr }}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, defaultModuleSettings(), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	snapshot := deviceSnapshot{Index: "0", Vendor: "10de", Device: "1db5", Class: "0302"}
	_, _, err = reconciler.reconcileDevice(context.Background(), node, snapshot, map[string]string{}, true)
	if !errors.Is(err, createErr) {
		t.Fatalf("expected create error, got %v", err)
	}
}

func TestCreateDeviceStatusConflictTriggersRequeue(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-conflict-create",
			UID:  types.UID("node-conflict-create"),
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "1db5",
				"gpu.deckhouse.io/device.00.class":  "0302",
			},
		},
	}
	baseClient := newTestClient(scheme, node)
	statusWriter := &conflictStatusUpdater{StatusWriter: baseClient.Status()}
	client := &delegatingClient{Client: baseClient, statusWriter: statusWriter}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, defaultModuleSettings(), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	snapshot := deviceSnapshot{Index: "0", Vendor: "10de", Device: "1db5", Class: "0302"}
	device, result, err := reconciler.reconcileDevice(context.Background(), node, snapshot, map[string]string{}, true)
	if err != nil {
		t.Fatalf("unexpected reconcileDevice error: %v", err)
	}
	if device == nil {
		t.Fatalf("expected device to be returned")
	}
	if !result.Requeue {
		t.Fatalf("expected requeue result when status update conflicts")
	}
}

func TestReconcileDeviceNoStatusPatchWhenUnchanged(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-nochange",
			UID:  types.UID("nochange"),
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "1db5",
				"gpu.deckhouse.io/device.00.class":  "0302",
			},
		},
	}
	device := &gpuv1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "worker-nochange-0-10de-1db5",
			Labels: map[string]string{deviceNodeLabelKey: "worker-nochange", deviceIndexLabelKey: "0"},
		},
		Status: gpuv1alpha1.GPUDeviceStatus{
			NodeName:    "worker-nochange",
			InventoryID: buildInventoryID("worker-nochange", deviceSnapshot{Index: "0", Vendor: "10de", Device: "1db5"}),
			Managed:     true,
			Hardware: gpuv1alpha1.GPUDeviceHardware{
				PCI:               gpuv1alpha1.PCIAddress{Vendor: "10de", Device: "1db5", Class: "0302"},
				Product:           "Existing",
				MemoryMiB:         1024,
				MIG:               gpuv1alpha1.GPUMIGConfig{},
				Precision:         gpuv1alpha1.GPUPrecision{Supported: []string{"fp32"}},
				ComputeCapability: &gpuv1alpha1.GPUComputeCapability{Major: 8, Minor: 0},
			},
		},
	}

	baseClient := newTestClient(scheme, node, device)
	tracker := &trackingStatusWriter{StatusWriter: baseClient.Status()}
	client := &delegatingClient{Client: baseClient, statusWriter: tracker}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, defaultModuleSettings(), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	snapshot := deviceSnapshot{
		Index:        "0",
		Vendor:       "10de",
		Device:       "1db5",
		Class:        "0302",
		Product:      "Existing",
		MemoryMiB:    1024,
		Precision:    []string{"fp32"},
		ComputeMajor: 8,
		ComputeMinor: 0,
	}
	_, _, err = reconciler.reconcileDevice(context.Background(), node, snapshot, map[string]string{}, true)
	if err != nil {
		t.Fatalf("unexpected reconcileDevice error: %v", err)
	}
	if tracker.patches != 0 {
		t.Fatalf("expected no status patch when nothing changes")
	}
}

func TestReconcileReturnsDeleteError(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-delete-error",
			UID:  types.UID("node-delete-error"),
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "2230",
				"gpu.deckhouse.io/device.00.class":  "0302",
			},
		},
	}
	primary := &gpuv1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "worker-delete-error-0-10de-2230",
			Labels: map[string]string{deviceNodeLabelKey: "worker-delete-error", deviceIndexLabelKey: "0"},
		},
		Status: gpuv1alpha1.GPUDeviceStatus{NodeName: "worker-delete-error"},
	}
	orphan := &gpuv1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "worker-delete-error-orphan",
			Labels: map[string]string{deviceNodeLabelKey: "worker-delete-error", deviceIndexLabelKey: "99"},
		},
		Status: gpuv1alpha1.GPUDeviceStatus{NodeName: "worker-delete-error"},
	}

	baseClient := newTestClient(scheme, node, primary, orphan)
	delErr := errors.New("delete failure")
	client := &delegatingClient{
		Client: baseClient,
		delete: func(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
			if dev, ok := obj.(*gpuv1alpha1.GPUDevice); ok && dev.Name == orphan.Name {
				return delErr
			}
			return baseClient.Delete(ctx, obj, opts...)
		},
	}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, defaultModuleSettings(), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	_, err = reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}})
	if !errors.Is(err, delErr) {
		t.Fatalf("expected delete error to propagate, got %v", err)
	}
}

func TestReconcileCleanupOnMissingNode(t *testing.T) {
	scheme := newTestScheme(t)
	device := &gpuv1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "worker-c-0-10de-1db5",
			Labels: map[string]string{deviceNodeLabelKey: "worker-c", deviceIndexLabelKey: "0"},
		},
		Status: gpuv1alpha1.GPUDeviceStatus{
			NodeName: "worker-c",
		},
	}
	inventory := &gpuv1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-c"},
		Spec:       gpuv1alpha1.GPUNodeInventorySpec{NodeName: "worker-c"},
	}

	client := newTestClient(scheme, device, inventory)

	module := defaultModuleSettings()
	reconciler, err := New(testr.New(t), config.ControllerConfig{}, module, nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	ctx := context.Background()
	if _, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "worker-c"}}); err != nil {
		t.Fatalf("unexpected reconcile error: %v", err)
	}

	if err := client.Get(ctx, types.NamespacedName{Name: device.Name}, &gpuv1alpha1.GPUDevice{}); err == nil || !apierrors.IsNotFound(err) {
		t.Fatalf("expected device to be removed, err=%v", err)
	}
	if err := client.Get(ctx, types.NamespacedName{Name: inventory.Name}, &gpuv1alpha1.GPUNodeInventory{}); err == nil || !apierrors.IsNotFound(err) {
		t.Fatalf("expected inventory to be removed, err=%v", err)
	}
}

func TestReconcileHandlesNodeFeatureMissing(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-missing-feature",
			UID:  types.UID("node-worker-missing-feature"),
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "2230",
				"gpu.deckhouse.io/device.00.class":  "0302",
			},
		},
	}

	client := newTestClient(scheme, node)
	module := defaultModuleSettings()
	reconciler, err := New(testr.New(t), config.ControllerConfig{}, module, nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	ctx := context.Background()
	if _, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}}); err != nil {
		t.Fatalf("unexpected reconcile error: %v", err)
	}

	inventory := &gpuv1alpha1.GPUNodeInventory{}
	if err := client.Get(ctx, types.NamespacedName{Name: node.Name}, inventory); err != nil {
		t.Fatalf("inventory missing: %v", err)
	}
	if cond := getCondition(inventory.Status.Conditions, conditionInventoryIncomplete); cond == nil || cond.Reason != reasonNodeFeatureMissing || cond.Status != metav1.ConditionTrue {
		t.Fatalf("expected InventoryIncomplete=true reason=%s, got %+v", reasonNodeFeatureMissing, cond)
	}
}

func TestReconcileHandlesNoDevicesDiscovered(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-no-devices",
			UID:  types.UID("worker-no-devices"),
		},
	}
	feature := &nfdv1alpha1.NodeFeature{
		ObjectMeta: metav1.ObjectMeta{Name: node.Name},
		Spec: nfdv1alpha1.NodeFeatureSpec{
			Labels: map[string]string{
				"nvidia.com/gpu.driver":        "535.86.05",
				"nvidia.com/cuda.driver.major": "12",
				"nvidia.com/cuda.driver.minor": "2",
			},
		},
	}

	client := newTestClient(scheme, node, feature)
	reconciler, err := New(testr.New(t), config.ControllerConfig{}, defaultModuleSettings(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	ctx := context.Background()
	if _, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}}); err != nil {
		t.Fatalf("unexpected reconcile error: %v", err)
	}

	inventory := &gpuv1alpha1.GPUNodeInventory{}
	if err := client.Get(ctx, types.NamespacedName{Name: node.Name}, inventory); err != nil {
		t.Fatalf("inventory missing: %v", err)
	}
	if cond := getCondition(inventory.Status.Conditions, conditionInventoryIncomplete); cond == nil || cond.Reason != reasonNoDevicesDiscovered {
		t.Fatalf("expected no devices discovered condition, got %+v", cond)
	}
}

func TestReconcileReturnsErrorOnNodeGetFailure(t *testing.T) {
	scheme := newTestScheme(t)
	baseClient := newTestClient(scheme)
	boom := errors.New("node get failed")

	client := &delegatingClient{
		Client: baseClient,
		get: func(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
			return boom
		},
	}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, defaultModuleSettings(), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	_, err = reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "worker-error"}})
	if !errors.Is(err, boom) {
		t.Fatalf("expected node get error, got %v", err)
	}
}

func TestReconcileReturnsErrorOnNodeFeatureGetFailure(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-feature-error",
			UID:  types.UID("node-feature-error"),
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "1db5",
				"gpu.deckhouse.io/device.00.class":  "0302",
			},
		},
	}

	baseClient := newTestClient(scheme, node)
	featureErr := errors.New("feature get failed")
	client := &delegatingClient{
		Client: baseClient,
		get: func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			switch obj.(type) {
			case *corev1.Node:
				return baseClient.Get(ctx, key, obj, opts...)
			case *nfdv1alpha1.NodeFeature:
				return featureErr
			default:
				return baseClient.Get(ctx, key, obj, opts...)
			}
		},
	}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, defaultModuleSettings(), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	_, err = reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}})
	if !errors.Is(err, featureErr) {
		t.Fatalf("expected feature get error, got %v", err)
	}
}

func TestReconcileReturnsErrorOnDeviceListFailure(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-list-error",
			UID:  types.UID("node-list-error"),
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "1db5",
				"gpu.deckhouse.io/device.00.class":  "0302",
			},
		},
	}
	listErr := errors.New("device list failed")

	client := &delegatingClient{
		Client: newTestClient(scheme, node),
		list: func(context.Context, client.ObjectList, ...client.ListOption) error {
			return listErr
		},
	}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, defaultModuleSettings(), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	_, err = reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}})
	if !errors.Is(err, listErr) {
		t.Fatalf("expected list error, got %v", err)
	}
}

func TestReconcilePropagatesHandlerError(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-handler-error",
			UID:  types.UID("node-handler-error"),
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "1db5",
				"gpu.deckhouse.io/device.00.class":  "0302",
			},
		},
	}

	client := newTestClient(scheme, node)
	handlerErr := errors.New("handler failed")
	reconciler, err := New(testr.New(t), config.ControllerConfig{}, defaultModuleSettings(), []contracts.InventoryHandler{errorHandler{err: handlerErr}})
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	_, err = reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}})
	if !errors.Is(err, handlerErr) {
		t.Fatalf("expected handler error, got %v", err)
	}
}

func TestReconcileAggregatesHandlerResults(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-aggregate",
			UID:  types.UID("node-aggregate"),
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "1db5",
				"gpu.deckhouse.io/device.00.class":  "0302",
			},
		},
	}

	client := newTestClient(scheme, node)
	reconciler, err := New(testr.New(t), config.ControllerConfig{}, defaultModuleSettings(), []contracts.InventoryHandler{
		resultHandler{name: "requeue", result: contracts.Result{Requeue: true, RequeueAfter: 45 * time.Second}},
		resultHandler{name: "after", result: contracts.Result{RequeueAfter: 10 * time.Second}},
	})
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)
	reconciler.resyncPeriod = 0

	res, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}})
	if err != nil {
		t.Fatalf("unexpected reconcile error: %v", err)
	}
	if !res.Requeue {
		t.Fatal("expected requeue=true from handler result")
	}
	if res.RequeueAfter != 10*time.Second {
		t.Fatalf("expected min requeueAfter=10s, got %s", res.RequeueAfter)
	}
}

func TestReconcileAccountsResyncPeriodWhenNoHandlers(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-resync-only", UID: types.UID("resync-only")}}

	client := newTestClient(scheme, node)
	reconciler, err := New(testr.New(t), config.ControllerConfig{ResyncPeriod: 5 * time.Second}, defaultModuleSettings(), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	res, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}})
	if err != nil {
		t.Fatalf("unexpected reconcile error: %v", err)
	}
	if res.RequeueAfter != 5*time.Second {
		t.Fatalf("expected requeue after resync period, got %s", res.RequeueAfter)
	}
}

func TestReconcileNoStatusChangeSkipsPatch(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-no-status-change",
			UID:  types.UID("node-no-status-change"),
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "1db5",
				"gpu.deckhouse.io/device.00.class":  "0302",
			},
		},
	}

	client := newTestClient(scheme, node)
	reconciler, err := New(testr.New(t), config.ControllerConfig{}, defaultModuleSettings(), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}}
	if _, err := reconciler.Reconcile(context.Background(), req); err != nil {
		t.Fatalf("initial reconcile failed: %v", err)
	}

	tracker := &trackingStatusWriter{StatusWriter: client.Status()}
	reconciler.client = &delegatingClient{Client: client, statusWriter: tracker}

	if _, err := reconciler.Reconcile(context.Background(), req); err != nil {
		t.Fatalf("second reconcile failed: %v", err)
	}
	if tracker.patches != 0 {
		t.Fatalf("expected no status patch, got %d patches", tracker.patches)
	}
}

func TestReconcileStatusConflictTriggersRetry(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-conflict",
			UID:  types.UID("node-conflict"),
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "1db5",
				"gpu.deckhouse.io/device.00.class":  "0302",
				"nvidia.com/gpu.memory":             "40960 MiB",
			},
		},
	}

	deviceName := buildDeviceName("worker-conflict", deviceSnapshot{Index: "0", Vendor: "10de", Device: "1db5", Class: "0302"})
	device := &gpuv1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   deviceName,
			Labels: map[string]string{deviceNodeLabelKey: "worker-conflict", deviceIndexLabelKey: "0"},
		},
		Status: gpuv1alpha1.GPUDeviceStatus{
			NodeName: "worker-conflict",
		},
	}

	baseClient := newTestClient(scheme, node, device)
	conflictWriter := &conflictStatusWriter{StatusWriter: baseClient.Status()}
	client := &delegatingClient{Client: baseClient, statusWriter: conflictWriter}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, defaultModuleSettings(), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)
	reconciler.resyncPeriod = 0

	res, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}})
	if err != nil {
		t.Fatalf("unexpected reconcile error: %v", err)
	}
	if !res.Requeue {
		t.Fatal("expected reconcile to request requeue on conflict")
	}
	if !conflictWriter.triggered {
		t.Fatal("expected conflict writer to be invoked")
	}
}

func TestReconcileNodeInventoryUpdatesSpec(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-inventory-update",
			UID:  types.UID("node-inventory-update"),
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "1db5",
				"gpu.deckhouse.io/device.00.class":  "0302",
			},
		},
	}
	inventory := &gpuv1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{Name: node.Name},
		Spec:       gpuv1alpha1.GPUNodeInventorySpec{NodeName: "stale"},
	}

	baseClient := newTestClient(scheme, node, inventory)
	reconciler, err := New(testr.New(t), config.ControllerConfig{}, defaultModuleSettings(), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = baseClient
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	if _, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}}); err != nil {
		t.Fatalf("unexpected reconcile error: %v", err)
	}

	updated := &gpuv1alpha1.GPUNodeInventory{}
	if err := baseClient.Get(context.Background(), types.NamespacedName{Name: node.Name}, updated); err != nil {
		t.Fatalf("failed to get updated inventory: %v", err)
	}
	if updated.Spec.NodeName != node.Name {
		t.Fatalf("expected inventory spec node updated, got %s", updated.Spec.NodeName)
	}
	if len(updated.OwnerReferences) == 0 || updated.OwnerReferences[0].UID != node.UID {
		t.Fatalf("expected owner reference to be set")
	}
}

func TestReconcileNodeInventoryStatusPatchError(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-status-error",
			UID:  types.UID("status-error"),
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "1db5",
				"gpu.deckhouse.io/device.00.class":  "0302",
				"nvidia.com/gpu.memory":             "40960 MiB",
			},
		},
	}

	baseClient := newTestClient(scheme, node)
	statusErr := errors.New("status patch failed")
	client := &delegatingClient{Client: baseClient, statusWriter: &errorStatusWriter{StatusWriter: baseClient.Status(), err: statusErr}}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, defaultModuleSettings(), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	_, err = reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}})
	if !errors.Is(err, statusErr) {
		t.Fatalf("expected status patch error, got %v", err)
	}
}

func TestReconcileNodeInventoryOwnerReferenceError(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-owner-ref",
			UID:  types.UID("owner-ref"),
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "1db5",
				"gpu.deckhouse.io/device.00.class":  "0302",
			},
		},
	}
	client := newTestClient(scheme, node)
	reconciler := &Reconciler{client: client, scheme: runtime.NewScheme(), recorder: record.NewFakeRecorder(32), managed: ManagedNodesPolicy{LabelKey: "gpu.deckhouse.io/enabled", EnabledByDefault: true}}

	snapshot := buildNodeSnapshot(node, nil, reconciler.managed)

	err := reconciler.reconcileNodeInventory(context.Background(), node, snapshot, nil)
	if err == nil {
		t.Fatalf("expected owner reference error due to missing scheme registration")
	}
}

func TestReconcileNodeInventorySkipsUnknownDevices(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-unknown-device",
			UID:  types.UID("worker-unknown-device"),
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "1db5",
				"gpu.deckhouse.io/device.00.class":  "0302",
			},
		},
	}
	device := &gpuv1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "worker-unknown-device-orphan",
			Labels: map[string]string{deviceNodeLabelKey: "worker-unknown-device", deviceIndexLabelKey: "99"},
		},
		Status: gpuv1alpha1.GPUDeviceStatus{
			NodeName:    "worker-unknown-device",
			InventoryID: "worker-unknown-device-99",
			Hardware:    gpuv1alpha1.GPUDeviceHardware{Product: "Existing", MemoryMiB: 256},
		},
	}

	client := newTestClient(scheme, node, device)
	reconciler := &Reconciler{client: client, scheme: scheme, recorder: record.NewFakeRecorder(32), managed: managedPolicyFrom(defaultModuleSettings())}

	snapshot := nodeSnapshot{Managed: true, Devices: []deviceSnapshot{}}
	if err := reconciler.reconcileNodeInventory(context.Background(), node, snapshot, []*gpuv1alpha1.GPUDevice{device}); err != nil {
		t.Fatalf("unexpected reconcileNodeInventory error: %v", err)
	}
	updated := &gpuv1alpha1.GPUNodeInventory{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: node.Name}, updated); err != nil {
		t.Fatalf("failed to fetch inventory: %v", err)
	}
	if len(updated.Status.Hardware.Devices) != 1 || updated.Status.Hardware.Devices[0].InventoryID != device.Status.InventoryID {
		t.Fatalf("expected inventory to retain device data, got %+v", updated.Status.Hardware.Devices)
	}
}

func TestEnsureDeviceMetadataUpdatesLabelsAndOwner(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "metadata-node",
			UID:  types.UID("metadata-node"),
		},
	}
	device := &gpuv1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name: "metadata-device",
		},
	}

	client := newTestClient(scheme, node, device)
	reconciler := &Reconciler{
		client: client,
		scheme: scheme,
	}

	snapshot := deviceSnapshot{Index: "1"}
	changed, err := reconciler.ensureDeviceMetadata(context.Background(), node, device, snapshot)
	if err != nil {
		t.Fatalf("ensureDeviceMetadata returned error: %v", err)
	}
	if !changed {
		t.Fatal("expected metadata to change")
	}
	if device.Labels[deviceNodeLabelKey] != node.Name {
		t.Fatalf("device node label not set: %v", device.Labels)
	}
	if device.Labels[deviceIndexLabelKey] != "1" {
		t.Fatalf("device index label not set: %v", device.Labels)
	}
	if len(device.OwnerReferences) != 1 || device.OwnerReferences[0].UID != node.UID {
		t.Fatalf("expected owner reference to node, got %+v", device.OwnerReferences)
	}
}

func TestEnsureDeviceMetadataNoChanges(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "unchanged-node", UID: types.UID("unchanged")}}
	device := &gpuv1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name: "unchanged-device",
			Labels: map[string]string{
				deviceNodeLabelKey:  node.Name,
				deviceIndexLabelKey: "0",
			},
		},
	}
	if err := controllerutil.SetOwnerReference(node, device, scheme); err != nil {
		t.Fatalf("preparing owner reference: %v", err)
	}

	reconciler := &Reconciler{client: newTestClient(scheme), scheme: scheme}
	changed, err := reconciler.ensureDeviceMetadata(context.Background(), node, device, deviceSnapshot{Index: "0"})
	if err != nil {
		t.Fatalf("ensureDeviceMetadata returned error: %v", err)
	}
	if changed {
		t.Fatalf("expected metadata to remain unchanged")
	}
}

func TestEnsureDeviceMetadataPatchError(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "metadata-error", UID: types.UID("metadata-error")}}
	device := &gpuv1alpha1.GPUDevice{ObjectMeta: metav1.ObjectMeta{Name: "metadata-device"}}
	reconciler := &Reconciler{
		scheme: scheme,
		client: &delegatingClient{patch: func(context.Context, client.Object, client.Patch, ...client.PatchOption) error {
			return errors.New("metadata patch failed")
		}},
	}

	_, err := reconciler.ensureDeviceMetadata(context.Background(), node, device, deviceSnapshot{Index: "0"})
	if err == nil {
		t.Fatalf("expected patch error from ensureDeviceMetadata")
	}
}

func TestReconcileInvokesHandlersErrorPropagation(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "handler-error",
			UID:  types.UID("handler-error"),
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "1db5",
				"gpu.deckhouse.io/device.00.class":  "0302",
			},
		},
	}

	client := newTestClient(scheme, node)
	errHandler := errorHandler{err: fmt.Errorf("boom")}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, defaultModuleSettings(), []contracts.InventoryHandler{errHandler})
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	_, err = reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}})
	if err == nil || err.Error() != "boom" {
		t.Fatalf("expected handler error to propagate, got %v", err)
	}
}

func TestCleanupNodeDeletesMetrics(t *testing.T) {
	const nodeName = "cleanup-metrics"
	inventoryDevicesGauge.WithLabelValues(nodeName).Set(2)
	inventoryConditionGauge.WithLabelValues(nodeName, conditionManagedDisabled).Set(1)
	inventoryConditionGauge.WithLabelValues(nodeName, conditionInventoryIncomplete).Set(1)

	scheme := newTestScheme(t)
	client := newTestClient(scheme)
	reconciler := &Reconciler{client: client}

	if err := reconciler.cleanupNode(context.Background(), nodeName); err != nil {
		t.Fatalf("cleanupNode returned error: %v", err)
	}
}

func TestSetupWithDependencies(t *testing.T) {
	scheme := newTestScheme(t)
	indexer := &fakeFieldIndexer{}
	builder := &fakeControllerBuilder{}
	fakeSource := &fakeSyncingSource{}
	rec := &Reconciler{cfg: config.ControllerConfig{Workers: 3}}

	deps := setupDependencies{
		client:            newTestClient(scheme),
		scheme:            scheme,
		recorder:          record.NewFakeRecorder(1),
		indexer:           indexer,
		nodeFeatureSource: fakeSource,
		builder:           builder,
	}

	if err := rec.setupWithDependencies(context.Background(), deps); err != nil {
		t.Fatalf("setupWithDependencies returned error: %v", err)
	}
	if rec.client == nil || rec.scheme != scheme || rec.recorder == nil {
		t.Fatal("setupWithDependencies did not assign manager dependencies")
	}
	if indexer.lastField != deviceNodeIndexKey {
		t.Fatalf("expected index field %q, got %q", deviceNodeIndexKey, indexer.lastField)
	}
	if len(builder.forObjects) != 1 || len(builder.ownedObjects) != 2 {
		t.Fatalf("unexpected builder registrations: for=%d owns=%d", len(builder.forObjects), len(builder.ownedObjects))
	}
	if builder.watchedSource != fakeSource {
		t.Fatal("expected node feature source to be passed to builder")
	}
	if builder.options.MaxConcurrentReconciles != 3 {
		t.Fatalf("expected max workers 3, got %d", builder.options.MaxConcurrentReconciles)
	}
	if builder.options.RecoverPanic == nil || !*builder.options.RecoverPanic {
		t.Fatalf("expected RecoverPanic enabled")
	}
	if builder.options.LogConstructor == nil {
		t.Fatalf("expected LogConstructor configured")
	}
	if builder.options.CacheSyncTimeout != cacheSyncTimeoutDuration {
		t.Fatalf("expected CacheSyncTimeout=%s, got %s", cacheSyncTimeoutDuration, builder.options.CacheSyncTimeout)
	}
	if !builder.completed {
		t.Fatal("expected builder complete to be invoked")
	}
}

func TestSetupWithManagerUsesFactories(t *testing.T) {
	scheme := newTestScheme(t)
	indexer := &fakeFieldIndexer{}
	builder := &fakeControllerBuilder{}
	fakeSource := &fakeSyncingSource{}

	rec := &Reconciler{
		cfg: config.ControllerConfig{Workers: 1},
	}
	var factoryCalled, sourceCalled bool
	rec.builderFactory = func(ctrl.Manager) controllerBuilder {
		factoryCalled = true
		return builder
	}
	rec.nodeFeatureSourceFactory = func(cache.Cache) source.SyncingSource {
		sourceCalled = true
		return fakeSource
	}

	mgr := stubManager{
		client:   newTestClient(scheme),
		scheme:   scheme,
		recorder: record.NewFakeRecorder(1),
		indexer:  indexer,
	}

	if err := rec.SetupWithManager(context.Background(), mgr); err != nil {
		t.Fatalf("SetupWithManager returned error: %v", err)
	}
	if !factoryCalled {
		t.Fatal("expected builder factory to be invoked")
	}
	if !sourceCalled {
		t.Fatal("expected node feature source factory to be invoked")
	}
	if !builder.completed {
		t.Fatal("expected builder Complete to be called")
	}
}

func TestSetupWithManagerUsesDefaultFactories(t *testing.T) {
	scheme := newTestScheme(t)
	indexer := &fakeFieldIndexer{}
	builder := &fakeControllerBuilder{}

	origControllerFactory := newControllerManagedBy
	origSourceBuilder := nodeFeatureSourceBuilder
	defer func() {
		newControllerManagedBy = origControllerFactory
		nodeFeatureSourceBuilder = origSourceBuilder
	}()

	newControllerManagedBy = func(ctrl.Manager) controllerRuntimeAdapter {
		return &adapterFromControllerBuilder{builder: builder}
	}

	rec, err := New(testr.New(t), config.ControllerConfig{Workers: 2}, defaultModuleSettings(), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	rec.builderFactory = nil
	rec.nodeFeatureSourceFactory = nil

	var sourceCalled bool
	fakeSource := &fakeSyncingSource{}
	nodeFeatureSourceBuilder = func(cache.Cache) source.SyncingSource {
		sourceCalled = true
		return fakeSource
	}

	mgr := stubManager{
		client:   newTestClient(scheme),
		scheme:   scheme,
		recorder: record.NewFakeRecorder(1),
		indexer:  indexer,
	}

	if err := rec.SetupWithManager(context.Background(), mgr); err != nil {
		t.Fatalf("SetupWithManager returned error: %v", err)
	}
	if !sourceCalled {
		t.Fatal("expected default node feature source to be invoked")
	}
	if builder.name != "gpu-inventory-controller" {
		t.Fatalf("expected controller name to be set, got %q", builder.name)
	}
	if builder.watchedSource != fakeSource {
		t.Fatal("expected node feature source to be configured")
	}
	if builder.options.MaxConcurrentReconciles != 2 {
		t.Fatalf("expected workers=2, got %d", builder.options.MaxConcurrentReconciles)
	}
	if builder.options.RecoverPanic == nil || !*builder.options.RecoverPanic {
		t.Fatalf("expected RecoverPanic enabled")
	}
	if builder.options.LogConstructor == nil {
		t.Fatalf("expected LogConstructor configured")
	}
	if builder.options.CacheSyncTimeout != cacheSyncTimeoutDuration {
		t.Fatalf("expected CacheSyncTimeout=%s, got %s", cacheSyncTimeoutDuration, builder.options.CacheSyncTimeout)
	}
	if !builder.completed {
		t.Fatal("expected builder complete to be called")
	}
}

func TestSetupWithDependenciesHandlesIndexerError(t *testing.T) {
	scheme := newTestScheme(t)
	indexErr := errors.New("index failure")
	rec := &Reconciler{cfg: config.ControllerConfig{Workers: 1}}

	deps := setupDependencies{
		client:            newTestClient(scheme),
		scheme:            scheme,
		recorder:          record.NewFakeRecorder(1),
		indexer:           &errorFieldIndexer{err: indexErr},
		nodeFeatureSource: &fakeSyncingSource{},
		builder:           &fakeControllerBuilder{},
	}

	if err := rec.setupWithDependencies(context.Background(), deps); !errors.Is(err, indexErr) {
		t.Fatalf("expected indexer error, got %v", err)
	}
}

func TestSetupWithDependenciesHandlesBuilderError(t *testing.T) {
	scheme := newTestScheme(t)
	buildErr := errors.New("builder failure")
	rec := &Reconciler{cfg: config.ControllerConfig{Workers: 1}}

	builder := &fakeControllerBuilder{completeErr: buildErr}
	deps := setupDependencies{
		client:            newTestClient(scheme),
		scheme:            scheme,
		recorder:          record.NewFakeRecorder(1),
		indexer:           &fakeFieldIndexer{},
		nodeFeatureSource: &fakeSyncingSource{},
		builder:           builder,
	}

	if err := rec.setupWithDependencies(context.Background(), deps); !errors.Is(err, buildErr) {
		t.Fatalf("expected builder error, got %v", err)
	}
}

func TestSetupWithDependenciesIndexesNodeName(t *testing.T) {
	scheme := newTestScheme(t)
	indexer := &capturingFieldIndexer{}
	builder := &fakeControllerBuilder{}
	fakeSource := &fakeSyncingSource{}

	rec := &Reconciler{cfg: config.ControllerConfig{Workers: 1}}
	deps := setupDependencies{
		client:            newTestClient(scheme),
		scheme:            scheme,
		recorder:          record.NewFakeRecorder(32),
		indexer:           indexer,
		nodeFeatureSource: fakeSource,
		builder:           builder,
	}

	if err := rec.setupWithDependencies(context.Background(), deps); err != nil {
		t.Fatalf("setupWithDependencies returned error: %v", err)
	}
	if len(indexer.values) == 0 || len(indexer.values[0]) != 1 || indexer.values[0][0] != "worker-indexed" {
		t.Fatalf("expected indexer to extract node name, got %+v", indexer.values)
	}
}

func TestSetupWithDependenciesIndexerSkipsNonGPU(t *testing.T) {
	scheme := newTestScheme(t)
	indexer := &multiFieldIndexer{}
	builder := &fakeControllerBuilder{}
	rec := &Reconciler{cfg: config.ControllerConfig{Workers: 1}}
	deps := setupDependencies{
		client:            newTestClient(scheme),
		scheme:            scheme,
		recorder:          record.NewFakeRecorder(32),
		indexer:           indexer,
		nodeFeatureSource: &fakeSyncingSource{},
		builder:           builder,
	}

	if err := rec.setupWithDependencies(context.Background(), deps); err != nil {
		t.Fatalf("setupWithDependencies returned error: %v", err)
	}
	if indexer.invocations == 0 {
		t.Fatal("expected indexer to be invoked")
	}
	if !builder.completed {
		t.Fatal("expected builder to complete")
	}
}

func TestDeviceApprovalAutoAttachPolicies(t *testing.T) {
	baseModule := defaultModuleSettings()

	tests := []struct {
		name           string
		configure      func(*config.ModuleSettings)
		wantAutoAttach bool
	}{
		{
			name: "manual",
			configure: func(m *config.ModuleSettings) {
				m.DeviceApproval.Mode = config.DeviceApprovalModeManual
				m.DeviceApproval.Selector = nil
			},
			wantAutoAttach: false,
		},
		{
			name: "automatic",
			configure: func(m *config.ModuleSettings) {
				m.DeviceApproval.Mode = config.DeviceApprovalModeAutomatic
				m.DeviceApproval.Selector = nil
			},
			wantAutoAttach: true,
		},
		{
			name: "selector-match",
			configure: func(m *config.ModuleSettings) {
				m.DeviceApproval.Mode = config.DeviceApprovalModeSelector
				m.DeviceApproval.Selector = &metav1.LabelSelector{
					MatchLabels: map[string]string{"gpu.deckhouse.io/device.vendor": "10de"},
				}
			},
			wantAutoAttach: true,
		},
		{
			name: "selector-miss",
			configure: func(m *config.ModuleSettings) {
				m.DeviceApproval.Mode = config.DeviceApprovalModeSelector
				m.DeviceApproval.Selector = &metav1.LabelSelector{
					MatchLabels: map[string]string{"gpu.deckhouse.io/device.vendor": "1234"},
				}
			},
			wantAutoAttach: false,
		},
		{
			name: "selector-empty",
			configure: func(m *config.ModuleSettings) {
				m.DeviceApproval.Mode = config.DeviceApprovalModeSelector
				m.DeviceApproval.Selector = nil
			},
			wantAutoAttach: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			module := baseModule
			tt.configure(&module)

			scheme := newTestScheme(t)
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "auto-node-" + tt.name,
					UID:  types.UID("node-" + tt.name),
					Labels: map[string]string{
						"gpu.deckhouse.io/device.00.vendor": "10de",
						"gpu.deckhouse.io/device.00.device": "1db5",
						"gpu.deckhouse.io/device.00.class":  "0302",
					},
				},
			}

			client := newTestClient(scheme, node)
			reconciler, err := New(testr.New(t), config.ControllerConfig{}, module, nil)
			if err != nil {
				t.Fatalf("unexpected error constructing reconciler: %v", err)
			}
			reconciler.client = client
			reconciler.scheme = scheme
			reconciler.recorder = record.NewFakeRecorder(32)

			ctx := context.Background()
			if _, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}}); err != nil {
				t.Fatalf("unexpected reconcile error: %v", err)
			}

			deviceName := buildDeviceName(node.Name, deviceSnapshot{Index: "0", Vendor: "10de", Device: "1db5", Class: "0302"})
			device := &gpuv1alpha1.GPUDevice{}
			if err := client.Get(ctx, types.NamespacedName{Name: deviceName}, device); err != nil {
				t.Fatalf("expected device, got error: %v", err)
			}
			if device.Status.AutoAttach != tt.wantAutoAttach {
				t.Fatalf("autoAttach mismatch: want %v got %v", tt.wantAutoAttach, device.Status.AutoAttach)
			}
		})
	}
}

func TestDeviceApprovalAutoAttachRespectsManagedFlag(t *testing.T) {
	policy := DeviceApprovalPolicy{mode: config.DeviceApprovalModeAutomatic}
	if policy.AutoAttach(false, labels.Set{}) {
		t.Fatalf("auto attach should be false when node not managed")
	}
}

func TestReconcileDeviceAutoAttachAutomatic(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-auto",
			UID:  types.UID("worker-auto"),
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "1db5",
				"gpu.deckhouse.io/device.00.class":  "0302",
			},
		},
	}
	device := &gpuv1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "worker-auto-0-10de-1db5",
			Labels: map[string]string{deviceNodeLabelKey: "worker-auto", deviceIndexLabelKey: "0"},
		},
		Status: gpuv1alpha1.GPUDeviceStatus{},
	}

	module := defaultModuleSettings()
	module.DeviceApproval.Mode = config.DeviceApprovalModeAutomatic

	baseClient := newTestClient(scheme, node, device)
	tracker := &trackingStatusWriter{StatusWriter: baseClient.Status()}
	client := &delegatingClient{Client: baseClient, statusWriter: tracker}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, module, nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	snapshot := deviceSnapshot{Index: "0", Vendor: "10de", Device: "1db5", Class: "0302"}
	_, _, err = reconciler.reconcileDevice(context.Background(), node, snapshot, map[string]string{}, true)
	if err != nil {
		t.Fatalf("unexpected reconcileDevice error: %v", err)
	}
	if tracker.patches == 0 {
		t.Fatalf("expected status patch to be emitted")
	}
	updated := &gpuv1alpha1.GPUDevice{}
	if err := baseClient.Get(context.Background(), types.NamespacedName{Name: device.Name}, updated); err != nil {
		t.Fatalf("failed to fetch updated device: %v", err)
	}
	if !updated.Status.AutoAttach {
		t.Fatalf("expected autoAttach=true")
	}
}

func TestReconcileDeviceSkipsStatusPatchWhenUnchanged(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-unchanged",
			UID:  types.UID("worker-unchanged"),
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "1db5",
				"gpu.deckhouse.io/device.00.class":  "0302",
			},
		},
	}

	deviceName := buildDeviceName(node.Name, deviceSnapshot{Index: "0", Vendor: "10de", Device: "1db5", Class: "0302"})
	device := &gpuv1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   deviceName,
			Labels: map[string]string{deviceNodeLabelKey: node.Name, deviceIndexLabelKey: "0"},
		},
		Status: gpuv1alpha1.GPUDeviceStatus{
			NodeName:    node.Name,
			InventoryID: buildInventoryID(node.Name, deviceSnapshot{Index: "0", Vendor: "10de", Device: "1db5", Class: "0302"}),
			Managed:     true,
			Hardware: gpuv1alpha1.GPUDeviceHardware{
				Product:   "NVIDIA TEST",
				PCI:       gpuv1alpha1.PCIAddress{Vendor: "10de", Device: "1db5", Class: "0302"},
				MemoryMiB: 16384,
				ComputeCapability: &gpuv1alpha1.GPUComputeCapability{
					Major: 8,
					Minor: 6,
				},
				Precision: gpuv1alpha1.GPUPrecision{
					Supported: []string{"fp32"},
				},
			},
			AutoAttach: false,
		},
	}
	if err := controllerutil.SetOwnerReference(node, device, scheme); err != nil {
		t.Fatalf("set owner reference: %v", err)
	}

	baseClient := newTestClient(scheme, node, device)
	tracker := &trackingStatusWriter{StatusWriter: baseClient.Status()}
	client := &delegatingClient{Client: baseClient, statusWriter: tracker}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, defaultModuleSettings(), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	snapshot := deviceSnapshot{
		Index:        "0",
		Vendor:       "10de",
		Device:       "1db5",
		Class:        "0302",
		Product:      "NVIDIA TEST",
		MemoryMiB:    16384,
		ComputeMajor: 8,
		ComputeMinor: 6,
		Precision:    []string{"fp32"},
	}

	_, result, err := reconciler.reconcileDevice(context.Background(), node, snapshot, map[string]string{}, true)
	if err != nil {
		t.Fatalf("unexpected reconcileDevice error: %v", err)
	}
	if result.Requeue || result.RequeueAfter != 0 {
		t.Fatalf("expected no requeue, got %+v", result)
	}
	if tracker.patches != 0 {
		t.Fatalf("expected no status patch, got %d patches", tracker.patches)
	}
}

func TestReconcileCleanupPropagatesError(t *testing.T) {
	scheme := newTestScheme(t)
	base := newTestClient(scheme)
	cleanupErr := errors.New("cleanup failure")

	client := &delegatingClient{
		Client: base,
		get: func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if _, ok := obj.(*corev1.Node); ok {
				return apierrors.NewNotFound(schema.GroupResource{Group: "", Resource: "nodes"}, key.Name)
			}
			return base.Get(ctx, key, obj, opts...)
		},
		list: func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
			if _, ok := list.(*gpuv1alpha1.GPUDeviceList); ok {
				return cleanupErr
			}
			return base.List(ctx, list, opts...)
		},
	}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, defaultModuleSettings(), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	if _, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "worker-cleanup"}}); !errors.Is(err, cleanupErr) {
		t.Fatalf("expected cleanup error, got %v", err)
	}
}

func TestReconcileNoFollowupWhenIdle(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-idle"}}
	client := newTestClient(scheme, node)

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, defaultModuleSettings(), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)
	reconciler.resyncPeriod = 0

	res, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}})
	if err != nil {
		t.Fatalf("unexpected reconcile error: %v", err)
	}
	if res.Requeue || res.RequeueAfter != 0 {
		t.Fatalf("expected no requeue and no resync, got %+v", res)
	}
}

func TestReconcileDeviceRefetchFailure(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-refetch",
			UID:  types.UID("node-worker-refetch"),
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "1db5",
				"gpu.deckhouse.io/device.00.class":  "0302",
			},
		},
	}
	device := &gpuv1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-refetch-0-10de-1db5",
		},
	}

	base := newTestClient(scheme, node, device)
	var getCalls int
	client := &delegatingClient{
		Client: base,
		get: func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if _, ok := obj.(*gpuv1alpha1.GPUDevice); ok {
				getCalls++
				if getCalls > 1 {
					return errors.New("device refetch failure")
				}
			}
			return base.Get(ctx, key, obj, opts...)
		},
		statusWriter: base.Status(),
	}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, defaultModuleSettings(), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	_, _, err = reconciler.reconcileDevice(context.Background(), node, deviceSnapshot{
		Index:     "0",
		Vendor:    "10de",
		Device:    "1db5",
		Class:     "0302",
		Product:   "NVIDIA TEST",
		MemoryMiB: 1024,
	}, map[string]string{}, true)
	if err == nil || !strings.Contains(err.Error(), "device refetch failure") {
		t.Fatalf("expected refetch failure, got %v", err)
	}
}

func TestReconcileDeviceStatusPatchError(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-patch-error",
			UID:  types.UID("node-patch-error"),
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "1db5",
				"gpu.deckhouse.io/device.00.class":  "0302",
			},
		},
	}
	device := &gpuv1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "worker-patch-error-0-10de-1db5",
			Labels: map[string]string{deviceNodeLabelKey: "old-node"},
		},
		Status: gpuv1alpha1.GPUDeviceStatus{NodeName: "old-node"},
	}

	base := newTestClient(scheme, node, device)
	client := &delegatingClient{
		Client:       base,
		statusWriter: &errorStatusWriter{StatusWriter: base.Status(), err: errors.New("status patch failure")},
	}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, defaultModuleSettings(), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	_, _, err = reconciler.reconcileDevice(context.Background(), node, deviceSnapshot{
		Index:     "0",
		Vendor:    "10de",
		Device:    "1db5",
		Class:     "0302",
		Product:   "NVIDIA TEST",
		MemoryMiB: 1024,
	}, map[string]string{}, true)
	if err == nil || !strings.Contains(err.Error(), "status patch failure") {
		t.Fatalf("expected status patch failure, got %v", err)
	}
}

func TestCreateDeviceStatusUpdateError(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-create-status",
			UID:  types.UID("node-create-status"),
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "1db5",
				"gpu.deckhouse.io/device.00.class":  "0302",
			},
		},
	}

	base := newTestClient(scheme, node)
	client := &delegatingClient{
		Client: base,
	}
	updateErr := errors.New("status update failure")
	client.statusWriter = &errorStatusUpdater{StatusWriter: base.Status(), err: updateErr}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, defaultModuleSettings(), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	_, _, err = reconciler.reconcileDevice(context.Background(), node, deviceSnapshot{
		Index:     "0",
		Vendor:    "10de",
		Device:    "1db5",
		Class:     "0302",
		Product:   "NVIDIA TEST",
		MemoryMiB: 1024,
	}, map[string]string{}, true)
	if err == nil || !strings.Contains(err.Error(), "status update failure") {
		t.Fatalf("expected status update failure, got %v", err)
	}
}

func TestReconcileDeviceUpdatesProductPrecisionAndAutoAttach(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-update-fields",
			UID:  types.UID("node-update-fields"),
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "2230",
				"gpu.deckhouse.io/device.00.class":  "0302",
			},
		},
	}
	device := &gpuv1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-update-fields-0-10de-2230",
		},
		Status: gpuv1alpha1.GPUDeviceStatus{
			NodeName:   "worker-update-fields",
			Managed:    true,
			AutoAttach: false,
			Hardware: gpuv1alpha1.GPUDeviceHardware{
				Product: "Old Product",
				Precision: gpuv1alpha1.GPUPrecision{
					Supported: []string{"fp16"},
				},
			},
		},
	}

	base := newTestClient(scheme, node, device)
	client := &delegatingClient{
		Client:       base,
		statusWriter: base.Status(),
	}

	module := defaultModuleSettings()
	module.DeviceApproval.Mode = config.DeviceApprovalModeAutomatic

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, module, nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	snapshot := deviceSnapshot{
		Index:        "0",
		Vendor:       "10de",
		Device:       "2230",
		Class:        "0302",
		Product:      "New Product",
		MemoryMiB:    16384,
		Precision:    []string{"fp16", "fp32"},
		MIG:          gpuv1alpha1.GPUMIGConfig{Capable: true, Strategy: gpuv1alpha1.GPUMIGStrategyMixed},
		UUID:         "GPU-NEW-UUID",
		ComputeMajor: 8,
		ComputeMinor: 0,
	}

	updated, _, err := reconciler.reconcileDevice(context.Background(), node, snapshot, map[string]string{}, true)
	if err != nil {
		t.Fatalf("unexpected reconcileDevice error: %v", err)
	}
	if updated.Status.Hardware.Product != "New Product" {
		t.Fatalf("expected product to be updated, got %s", updated.Status.Hardware.Product)
	}
	if !stringSlicesEqual(updated.Status.Hardware.Precision.Supported, []string{"fp16", "fp32"}) {
		t.Fatalf("expected precision to be updated, got %+v", updated.Status.Hardware.Precision.Supported)
	}
	if !updated.Status.AutoAttach {
		t.Fatal("expected auto attach to be enabled in automatic mode")
	}
}

func TestReconcileDeviceHandlerError(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-handler-error",
			UID:  types.UID("node-handler-error"),
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "1db5",
				"gpu.deckhouse.io/device.00.class":  "0302",
			},
		},
	}

	device := &gpuv1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-handler-error-0-10de-1db5"},
		Status:     gpuv1alpha1.GPUDeviceStatus{NodeName: "worker-handler-error"},
	}
	if err := controllerutil.SetOwnerReference(node, device, scheme); err != nil {
		t.Fatalf("set owner reference: %v", err)
	}
	base := newTestClient(scheme, node, device)
	errHandler := errorHandler{err: errors.New("handler failure")}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, defaultModuleSettings(), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = base
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)
	reconciler.handlers = []contracts.InventoryHandler{errHandler}

	_, _, err = reconciler.reconcileDevice(context.Background(), node, deviceSnapshot{
		Index:     "0",
		Vendor:    "10de",
		Device:    "1db5",
		Class:     "0302",
		Product:   "GPU",
		MemoryMiB: 1024,
	}, map[string]string{}, true)
	if err == nil || !strings.Contains(err.Error(), "handler failure") {
		t.Fatalf("expected handler failure, got %v", err)
	}
}

func TestEnsureDeviceMetadataOwnerReferenceError(t *testing.T) {
	reconciler := &Reconciler{
		scheme: runtime.NewScheme(),
	}
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-meta-error"}}
	device := &gpuv1alpha1.GPUDevice{}

	changed, err := reconciler.ensureDeviceMetadata(context.Background(), node, device, deviceSnapshot{Index: "0"})
	if err == nil {
		t.Fatal("expected owner reference error")
	}
	if changed {
		t.Fatal("expected changed=false on error")
	}
}

func TestReconcileNodeInventoryOwnerReferenceCreateError(t *testing.T) {
	scheme := runtime.NewScheme()
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-inv-create"}}
	client := newTestClient(newTestScheme(t), node)

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, defaultModuleSettings(), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	err = reconciler.reconcileNodeInventory(context.Background(), node, nodeSnapshot{}, nil)
	if err == nil {
		t.Fatal("expected owner reference error on create")
	}
}

func TestReconcileNodeInventoryReturnsGetError(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-inv-get"}}
	base := newTestClient(scheme, node)
	getErr := errors.New("inventory get failure")

	client := &delegatingClient{
		Client: base,
		get: func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if _, ok := obj.(*gpuv1alpha1.GPUNodeInventory); ok {
				return getErr
			}
			return base.Get(ctx, key, obj, opts...)
		},
	}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, defaultModuleSettings(), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	if err := reconciler.reconcileNodeInventory(context.Background(), node, nodeSnapshot{}, nil); !errors.Is(err, getErr) {
		t.Fatalf("expected get error to propagate, got %v", err)
	}
}

func TestReconcileNodeInventoryCreateError(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-inv-create-fail",
			UID:  types.UID("node-inv-create-fail"),
		},
	}

	base := newTestClient(scheme, node)
	createErr := errors.New("inventory create failure")
	client := &delegatingClient{
		Client: base,
		create: func(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
			if _, ok := obj.(*gpuv1alpha1.GPUNodeInventory); ok {
				return createErr
			}
			return base.Create(ctx, obj, opts...)
		},
	}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, defaultModuleSettings(), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	err = reconciler.reconcileNodeInventory(context.Background(), node, nodeSnapshot{}, nil)
	if !errors.Is(err, createErr) {
		t.Fatalf("expected create error, got %v", err)
	}
}

func TestReconcileNodeInventoryOwnerReferenceUpdateError(t *testing.T) {
	scheme := runtime.NewScheme()
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-inv-update"}}
	inventory := &gpuv1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{Name: node.Name},
		Spec:       gpuv1alpha1.GPUNodeInventorySpec{NodeName: node.Name},
	}
	client := newTestClient(newTestScheme(t), node, inventory)

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, defaultModuleSettings(), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	err = reconciler.reconcileNodeInventory(context.Background(), node, nodeSnapshot{}, nil)
	if err == nil {
		t.Fatal("expected owner reference error on update")
	}
}

func TestReconcileNodeInventoryPatchError(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-inv-patch",
			UID:  types.UID("node-inv-patch"),
		},
	}
	inventory := &gpuv1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{Name: node.Name},
		Spec:       gpuv1alpha1.GPUNodeInventorySpec{NodeName: "old-node"},
	}

	base := newTestClient(scheme, node, inventory)
	patchErr := errors.New("inventory patch failure")
	client := &delegatingClient{
		Client: base,
		patch: func(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
			if _, ok := obj.(*gpuv1alpha1.GPUNodeInventory); ok {
				return patchErr
			}
			return base.Patch(ctx, obj, patch, opts...)
		},
	}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, defaultModuleSettings(), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	err = reconciler.reconcileNodeInventory(context.Background(), node, nodeSnapshot{}, nil)
	if !errors.Is(err, patchErr) {
		t.Fatalf("expected patch error, got %v", err)
	}
}

func TestReconcileNodeInventoryRefetchError(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-inv-refetch",
			UID:  types.UID("node-inv-refetch"),
		},
	}
	inventory := &gpuv1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{Name: node.Name},
		Spec:       gpuv1alpha1.GPUNodeInventorySpec{NodeName: "old"},
	}

	base := newTestClient(scheme, node, inventory)
	var getCalls int
	client := &delegatingClient{
		Client: base,
		patch: func(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
			return base.Patch(ctx, obj, patch, opts...)
		},
		get: func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if _, ok := obj.(*gpuv1alpha1.GPUNodeInventory); ok {
				getCalls++
				if getCalls > 1 {
					return errors.New("inventory refetch failure")
				}
			}
			return base.Get(ctx, key, obj, opts...)
		},
	}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, defaultModuleSettings(), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	err = reconciler.reconcileNodeInventory(context.Background(), node, nodeSnapshot{
		Devices: []deviceSnapshot{{
			Index:     "0",
			Vendor:    "10de",
			Device:    "1db5",
			Class:     "0302",
			Product:   "GPU",
			MemoryMiB: 2048,
		}},
	}, []*gpuv1alpha1.GPUDevice{{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "worker-inv-refetch-device",
			Labels: map[string]string{deviceIndexLabelKey: "0"},
		},
		Status: gpuv1alpha1.GPUDeviceStatus{
			InventoryID: "worker-inv-refetch-0-10de-1db5",
			Hardware: gpuv1alpha1.GPUDeviceHardware{
				Product:   "GPU",
				MemoryMiB: 2048,
				MIG:       gpuv1alpha1.GPUMIGConfig{},
			},
		},
	}})
	if err == nil || !strings.Contains(err.Error(), "inventory refetch failure") {
		t.Fatalf("expected refetch failure, got %v", err)
	}
}

func TestReconcileNodeInventoryAppliesSnapshotPrecision(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-inv-precision",
			UID:  types.UID("node-inv-precision"),
		},
	}
	device := &gpuv1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "worker-inv-precision-device",
			Labels: map[string]string{deviceIndexLabelKey: "0"},
		},
		Status: gpuv1alpha1.GPUDeviceStatus{
			InventoryID: "worker-inv-precision-0-10de-1db5",
		},
	}

	client := newTestClient(scheme, node, device)

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, defaultModuleSettings(), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	snapshot := nodeSnapshot{
		Devices: []deviceSnapshot{{
			Index:     "0",
			Vendor:    "10de",
			Device:    "1db5",
			Class:     "0302",
			Product:   "NVIDIA GPU",
			Precision: []string{"fp16", "fp32"},
		}},
	}

	if err := reconciler.reconcileNodeInventory(context.Background(), node, snapshot, []*gpuv1alpha1.GPUDevice{device}); err != nil {
		t.Fatalf("unexpected reconcile error: %v", err)
	}

	inventory := &gpuv1alpha1.GPUNodeInventory{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: node.Name}, inventory); err != nil {
		t.Fatalf("get inventory: %v", err)
	}
	if len(inventory.Status.Hardware.Devices) != 1 {
		t.Fatalf("expected 1 device, got %+v", inventory.Status.Hardware.Devices)
	}
	if !stringSlicesEqual(inventory.Status.Hardware.Devices[0].Precision.Supported, []string{"fp16", "fp32"}) {
		t.Fatalf("expected precision from snapshot, got %+v", inventory.Status.Hardware.Devices[0].Precision.Supported)
	}
}

func TestReconcileNodeInventorySortsDevices(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-inv-sort",
			UID:  types.UID("node-inv-sort"),
		},
	}
	devices := []*gpuv1alpha1.GPUDevice{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "worker-inv-sort-dev1",
				Labels: map[string]string{deviceIndexLabelKey: "1"},
			},
			Status: gpuv1alpha1.GPUDeviceStatus{
				InventoryID: "worker-inv-sort-1",
				Hardware:    gpuv1alpha1.GPUDeviceHardware{},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "worker-inv-sort-dev0",
				Labels: map[string]string{deviceIndexLabelKey: "0"},
			},
			Status: gpuv1alpha1.GPUDeviceStatus{
				InventoryID: "worker-inv-sort-0",
				Hardware:    gpuv1alpha1.GPUDeviceHardware{},
			},
		},
	}

	client := newTestClient(scheme, node, devices[0], devices[1])

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, defaultModuleSettings(), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	if err := reconciler.reconcileNodeInventory(context.Background(), node, nodeSnapshot{
		Devices: []deviceSnapshot{
			{Index: "1", Vendor: "10de", Device: "1db5", Class: "0302"},
			{Index: "0", Vendor: "10de", Device: "2230", Class: "0302"},
		},
	}, devices); err != nil {
		t.Fatalf("unexpected reconcile error: %v", err)
	}

	inventory := &gpuv1alpha1.GPUNodeInventory{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: node.Name}, inventory); err != nil {
		t.Fatalf("get inventory: %v", err)
	}
	if len(inventory.Status.Hardware.Devices) != 2 {
		t.Fatalf("expected 2 devices, got %+v", inventory.Status.Hardware.Devices)
	}
	if inventory.Status.Hardware.Devices[0].InventoryID > inventory.Status.Hardware.Devices[1].InventoryID {
		t.Fatalf("expected devices sorted by inventory id, got %+v", inventory.Status.Hardware.Devices)
	}
}

func TestReconcileNodeInventoryDefaultManagedLabel(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-inv-managed-default",
			UID:  types.UID("node-inv-managed-default"),
		},
	}
	client := newTestClient(scheme, node)

	module := defaultModuleSettings()
	module.ManagedNodes.LabelKey = ""

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, module, nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	if err := reconciler.reconcileNodeInventory(context.Background(), node, nodeSnapshot{Managed: false}, nil); err != nil {
		t.Fatalf("unexpected reconcile error: %v", err)
	}
	inventory := &gpuv1alpha1.GPUNodeInventory{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: node.Name}, inventory); err != nil {
		t.Fatalf("get inventory: %v", err)
	}
	cond := getCondition(inventory.Status.Conditions, conditionManagedDisabled)
	if cond == nil || cond.Message != fmt.Sprintf("node is marked with %s=false", defaultManagedNodeLabelKey) {
		t.Fatalf("expected default managed label usage, got %+v", cond)
	}
}

func TestMapNodeFeatureToNode(t *testing.T) {
	feature := &nfdv1alpha1.NodeFeature{ObjectMeta: metav1.ObjectMeta{Name: "worker-d"}}
	reqs := mapNodeFeatureToNode(context.Background(), feature)
	if len(reqs) != 1 || reqs[0].Name != "worker-d" {
		t.Fatalf("unexpected requests: %+v", reqs)
	}

	noName := &nfdv1alpha1.NodeFeature{}
	if reqs := mapNodeFeatureToNode(context.Background(), noName); len(reqs) != 0 {
		t.Fatalf("expected empty requests, got %+v", reqs)
	}
}

func TestCleanupNodeReturnsListError(t *testing.T) {
	scheme := newTestScheme(t)
	listErr := errors.New("list failure")

	client := &delegatingClient{
		Client: newTestClient(scheme),
		list: func(context.Context, client.ObjectList, ...client.ListOption) error {
			return listErr
		},
	}

	reconciler := &Reconciler{client: client}
	if err := reconciler.cleanupNode(context.Background(), "worker-list-error"); !errors.Is(err, listErr) {
		t.Fatalf("expected list error, got %v", err)
	}
}

func TestCleanupNodeReturnsDeviceDeleteError(t *testing.T) {
	scheme := newTestScheme(t)
	device := &gpuv1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "worker-delete-0",
			Labels: map[string]string{deviceNodeLabelKey: "worker-delete", deviceIndexLabelKey: "0"},
		},
		Status: gpuv1alpha1.GPUDeviceStatus{NodeName: "worker-delete"},
	}
	base := newTestClient(scheme, device)
	deleteErr := errors.New("device delete failure")

	client := &delegatingClient{
		Client: base,
		delete: func(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
			switch obj.(type) {
			case *gpuv1alpha1.GPUDevice:
				return deleteErr
			default:
				return base.Delete(ctx, obj, opts...)
			}
		},
	}

	reconciler := &Reconciler{client: client}
	if err := reconciler.cleanupNode(context.Background(), "worker-delete"); !errors.Is(err, deleteErr) {
		t.Fatalf("expected device delete error, got %v", err)
	}
}

func TestCleanupNodeReturnsInventoryDeleteError(t *testing.T) {
	scheme := newTestScheme(t)
	inventory := &gpuv1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-inventory"},
		Spec:       gpuv1alpha1.GPUNodeInventorySpec{NodeName: "worker-inventory"},
	}
	base := newTestClient(scheme, inventory)
	deleteErr := errors.New("inventory delete failure")

	client := &delegatingClient{
		Client: base,
		delete: func(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
			switch obj.(type) {
			case *gpuv1alpha1.GPUNodeInventory:
				return deleteErr
			default:
				return base.Delete(ctx, obj, opts...)
			}
		},
	}

	reconciler := &Reconciler{client: client}
	if err := reconciler.cleanupNode(context.Background(), "worker-inventory"); !errors.Is(err, deleteErr) {
		t.Fatalf("expected inventory delete error, got %v", err)
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

func TestSetStatusConditionNoChange(t *testing.T) {
	conds := []metav1.Condition{{
		Type:    conditionManagedDisabled,
		Status:  metav1.ConditionFalse,
		Reason:  "ManagedEnabled",
		Message: "node managed",
	}}

	if changed := setStatusCondition(&conds, metav1.Condition{
		Type:    conditionManagedDisabled,
		Status:  metav1.ConditionFalse,
		Reason:  "ManagedEnabled",
		Message: "node managed",
	}); changed {
		t.Fatal("expected no change when condition identical")
	}
	if len(conds) != 1 {
		t.Fatalf("expected single condition preserved, got %d", len(conds))
	}
}

func TestSetStatusConditionEmptyType(t *testing.T) {
	var conds []metav1.Condition
	if !setStatusCondition(&conds, metav1.Condition{Status: metav1.ConditionTrue}) {
		t.Fatal("expected change when inserting first empty type condition")
	}
	conds[0].Message = "same"
	if setStatusCondition(&conds, metav1.Condition{Status: metav1.ConditionTrue, Message: "same"}) {
		t.Fatal("expected no change when contents identical")
	}
}

func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := gpuv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add gpu scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	if err := nfdv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add nfd scheme: %v", err)
	}
	return scheme
}

func newTestClient(scheme *runtime.Scheme, objs ...client.Object) client.Client {
	builder := clientfake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gpuv1alpha1.GPUDevice{}, &gpuv1alpha1.GPUNodeInventory{}).
		WithObjects(objs...)

	builder = builder.WithIndex(&gpuv1alpha1.GPUDevice{}, deviceNodeIndexKey, func(obj client.Object) []string {
		device, ok := obj.(*gpuv1alpha1.GPUDevice)
		if !ok || device.Status.NodeName == "" {
			return nil
		}
		return []string{device.Status.NodeName}
	})

	return builder.Build()
}
