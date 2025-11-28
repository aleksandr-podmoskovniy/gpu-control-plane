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
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr/testr"
	"github.com/prometheus/client_golang/prometheus/testutil"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
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

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	bootstrapmeta "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/bootstrap/meta"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/config"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
	moduleconfig "github.com/aleksandr-podmoskovniy/gpu-control-plane/pkg/moduleconfig"

	nfdv1alpha1 "sigs.k8s.io/node-feature-discovery/api/nfd/v1alpha1"
)

type trackingHandler struct {
	name    string
	state   v1alpha1.GPUDeviceState
	result  contracts.Result
	handled []string
}

type errorHandler struct {
	err error
}

func (errorHandler) Name() string {
	return "error"
}

func (h errorHandler) HandleDevice(context.Context, *v1alpha1.GPUDevice) (contracts.Result, error) {
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
	if device, ok := obj.(*v1alpha1.GPUDevice); ok {
		device.Status.NodeName = "worker-indexed"
	}
	if extract != nil {
		vals := extract(obj)
		c.values = append(c.values, vals)
	}
	return nil
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

type failingGetListClient struct {
	client.Client
	getErr         error
	listErr        error
	nodeFeatureErr error
}

func (f *failingGetListClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	switch obj.(type) {
	case *nfdv1alpha1.NodeFeature:
		if f.nodeFeatureErr != nil {
			return f.nodeFeatureErr
		}
	}
	if f.getErr != nil {
		return f.getErr
	}
	return f.Client.Get(ctx, key, obj, opts...)
}

func (f *failingGetListClient) List(context.Context, client.ObjectList, ...client.ListOption) error {
	if f.listErr != nil {
		return f.listErr
	}
	return nil
}

type requeueHandler struct {
	result contracts.Result
}

func (h requeueHandler) Name() string { return "requeue-handler" }
func (h requeueHandler) HandleDevice(context.Context, *v1alpha1.GPUDevice) (contracts.Result, error) {
	return h.result, nil
}

type dummyCache struct{}

func (dummyCache) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	return nil
}

func (dummyCache) List(context.Context, client.ObjectList, ...client.ListOption) error {
	return nil
}

func (dummyCache) GetInformer(context.Context, client.Object, ...cache.InformerGetOption) (cache.Informer, error) {
	return nil, nil
}

func (dummyCache) GetInformerForKind(context.Context, schema.GroupVersionKind, ...cache.InformerGetOption) (cache.Informer, error) {
	return nil, nil
}

func (dummyCache) RemoveInformer(context.Context, client.Object) error {
	return nil
}

func (dummyCache) Start(context.Context) error {
	return nil
}

func (dummyCache) WaitForCacheSync(context.Context) bool {
	return true
}

func (dummyCache) IndexField(context.Context, client.Object, string, client.IndexerFunc) error {
	return nil
}

type fakeCache struct{}

func (fakeCache) RemoveInformer(context.Context, client.Object) error                  { return nil }
func (fakeCache) RemoveInformerForKind(context.Context, schema.GroupVersionKind) error { return nil }

func (fakeCache) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	return nil
}
func (fakeCache) List(context.Context, client.ObjectList, ...client.ListOption) error { return nil }
func (fakeCache) GetInformer(context.Context, client.Object, ...cache.InformerGetOption) (cache.Informer, error) {
	return nil, nil
}
func (fakeCache) GetInformerForKind(context.Context, schema.GroupVersionKind, ...cache.InformerGetOption) (cache.Informer, error) {
	return nil, nil
}
func (fakeCache) Start(context.Context) error           { return nil }
func (fakeCache) WaitForCacheSync(context.Context) bool { return true }
func (fakeCache) IndexField(context.Context, client.Object, string, client.IndexerFunc) error {
	return nil
}

type fakeControllerBuilder struct {
	name           string
	forObjects     []client.Object
	ownedObjects   []client.Object
	watchedSources []source.Source
	options        controller.Options
	completed      bool
	completeErr    error
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
	b.watchedSources = append(b.watchedSources, src)
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
		extract(&v1alpha1.GPUDevice{})
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

func (h *trackingHandler) HandleDevice(_ context.Context, device *v1alpha1.GPUDevice) (contracts.Result, error) {
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

func (h resultHandler) HandleDevice(context.Context, *v1alpha1.GPUDevice) (contracts.Result, error) {
	return h.result, nil
}

func defaultModuleSettings() config.ModuleSettings {
	return config.DefaultSystem().Module
}

func moduleStoreFrom(settings config.ModuleSettings) *config.ModuleConfigStore {
	state, err := config.ModuleSettingsToState(settings)
	if err != nil {
		panic(err)
	}
	return config.NewModuleConfigStore(state)
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
		state:  v1alpha1.GPUDeviceStateReserved,
		result: contracts.Result{RequeueAfter: 10 * time.Second},
	}

	inventory := &v1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{
			Name: node.Name,
			OwnerReferences: []metav1.OwnerReference{
				{APIVersion: "v1", Kind: "Node", Name: node.Name, UID: node.UID},
			},
		},
		Spec: v1alpha1.GPUNodeInventorySpec{NodeName: node.Name},
	}

	client := newTestClient(scheme, node, feature, inventory)

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(module), []contracts.InventoryHandler{handler})
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
	device := &v1alpha1.GPUDevice{}
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
	if device.Status.State != v1alpha1.GPUDeviceStateReserved {
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
	if mig.Strategy != v1alpha1.GPUMIGStrategySingle {
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

	fetchedInventory := &v1alpha1.GPUNodeInventory{}
	if err := client.Get(ctx, types.NamespacedName{Name: node.Name}, fetchedInventory); err != nil {
		t.Fatalf("inventory not found: %v", err)
	}
	if fetchedInventory.Spec.NodeName != node.Name {
		t.Fatalf("inventory spec node mismatch: %q", fetchedInventory.Spec.NodeName)
	}
	if !fetchedInventory.Status.Hardware.Present {
		t.Fatal("inventory should mark hardware present")
	}
	if len(fetchedInventory.Status.Hardware.Devices) != 1 {
		t.Fatalf("expected 1 device in inventory, got %d", len(fetchedInventory.Status.Hardware.Devices))
	}
	if fetchedInventory.Status.Hardware.Devices[0].InventoryID != device.Status.InventoryID {
		t.Fatalf("inventory device id mismatch")
	}
	inventoryDevice := fetchedInventory.Status.Hardware.Devices[0]
	if inventoryDevice.Product != "NVIDIA A100-PCIE-40GB" {
		t.Fatalf("unexpected inventory product: %s", inventoryDevice.Product)
	}
	if inventoryDevice.MIG.Strategy != v1alpha1.GPUMIGStrategySingle {
		t.Fatalf("unexpected inventory MIG strategy: %s", inventoryDevice.MIG.Strategy)
	}
	if len(inventoryDevice.MIG.Types) != 1 || inventoryDevice.MIG.Types[0].Name != "mig-1g.10gb" || inventoryDevice.MIG.Types[0].Count != 2 {
		t.Fatalf("unexpected inventory MIG types: %+v", inventoryDevice.MIG.Types)
	}
	if inventoryDevice.UUID != "GPU-TEST-UUID-0001" {
		t.Fatalf("unexpected inventory UUID: %s", inventoryDevice.UUID)
	}
	if fetchedInventory.Status.Driver.Version != "535.86.05" {
		t.Fatalf("unexpected driver version: %s", fetchedInventory.Status.Driver.Version)
	}
	if fetchedInventory.Status.Driver.CUDAVersion != "12.2" {
		t.Fatalf("unexpected cuda version: %s", fetchedInventory.Status.Driver.CUDAVersion)
	}
	if !fetchedInventory.Status.Driver.ToolkitReady {
		t.Fatal("expected driver toolkit ready true")
	}
	if cond := getCondition(fetchedInventory.Status.Conditions, conditionInventoryComplete); cond == nil || cond.Status != metav1.ConditionTrue {
		t.Fatalf("expected InventoryComplete=true, got %+v", cond)
	}
	if cond := getCondition(fetchedInventory.Status.Conditions, conditionManagedDisabled); cond == nil || cond.Status != metav1.ConditionFalse {
		t.Fatalf("expected ManagedDisabled=false, got %+v", cond)
	}
}

func TestReconcileHandlesNamespacedNodeFeature(t *testing.T) {
	module := defaultModuleSettings()
	scheme := newTestScheme(t)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-namespaced",
			UID:  types.UID("node-worker-namespaced"),
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
			},
		},
	}

	feature := &nfdv1alpha1.NodeFeature{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "gpu-operator",
			Name:      node.Name,
			Labels: map[string]string{
				nodeFeatureNodeNameLabel: node.Name,
			},
		},
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
								"index":         "0",
								"uuid":          "GPU-TEST-UUID-NAMESPACED",
								"precision":     "fp32,fp16",
								"memory.total":  "40536 MiB",
								"compute.major": "8",
								"compute.minor": "0",
								"product":       "NVIDIA A100-PCIE-40GB",
							}},
						},
					},
				},
			},
		},
	}

	inventory := &v1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{
			Name: node.Name,
			OwnerReferences: []metav1.OwnerReference{
				{APIVersion: "v1", Kind: "Node", Name: node.Name, UID: node.UID},
			},
		},
		Spec: v1alpha1.GPUNodeInventorySpec{NodeName: node.Name},
	}

	client := newTestClient(scheme, node, feature, inventory)
	handler := &trackingHandler{state: v1alpha1.GPUDeviceStateReserved}
	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(module), []contracts.InventoryHandler{handler})
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

	nodeSnapshot := buildNodeSnapshot(node, feature, managedPolicyFrom(module))
	if len(nodeSnapshot.Devices) != 1 {
		t.Fatalf("expected single device snapshot, got %d", len(nodeSnapshot.Devices))
	}

	deviceName := buildDeviceName(node.Name, nodeSnapshot.Devices[0])
	device := &v1alpha1.GPUDevice{}
	if err := client.Get(ctx, types.NamespacedName{Name: deviceName}, device); err != nil {
		t.Fatalf("device not found: %v", err)
	}
	if device.Status.NodeName != node.Name {
		t.Fatalf("expected NodeName %q, got %q", node.Name, device.Status.NodeName)
	}

	if err := client.Get(ctx, types.NamespacedName{Name: node.Name}, inventory); err != nil {
		t.Fatalf("inventory not found: %v", err)
	}
	cond := getCondition(inventory.Status.Conditions, conditionInventoryComplete)
	if cond == nil || cond.Status != metav1.ConditionTrue || cond.Reason != reasonInventorySynced {
		t.Fatalf("expected InventoryComplete=true reason=%s, got %+v", reasonInventorySynced, cond)
	}
}

func TestReconcileSchedulesDefaultResync(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-resync"}}

	client := newTestClient(scheme, node)

	module := defaultModuleSettings()
	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(module), nil)
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
	if res.RequeueAfter != 0 {
		t.Fatalf("expected no resync when no devices are discovered, got %s", res.RequeueAfter)
	}
}

func TestNewAppliesDefaultsAndPolicies(t *testing.T) {
	module := defaultModuleSettings()
	cfg := config.ControllerConfig{Workers: 0, ResyncPeriod: 0}

	rec, err := New(testr.New(t), cfg, moduleStoreFrom(module), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	if rec.cfg.Workers != 1 {
		t.Fatalf("expected workers default to 1, got %d", rec.cfg.Workers)
	}
	if rec.getResyncPeriod() != defaultResyncPeriod {
		t.Fatalf("expected default resync period %s, got %s", defaultResyncPeriod, rec.getResyncPeriod())
	}
	if rec.fallbackManaged.LabelKey != module.ManagedNodes.LabelKey {
		t.Fatalf("unexpected managed label key %s", rec.fallbackManaged.LabelKey)
	}
	if string(rec.fallbackApproval.mode) != string(module.DeviceApproval.Mode) {
		t.Fatalf("unexpected approval mode %s", rec.fallbackApproval.mode)
	}
}

func TestNewReturnsErrorOnInvalidSelector(t *testing.T) {
	state := moduleconfig.DefaultState()
	state.Settings.DeviceApproval.Mode = moduleconfig.DeviceApprovalModeSelector
	state.Settings.DeviceApproval.Selector = &metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{Key: "gpu.deckhouse.io/device.vendor", Operator: metav1.LabelSelectorOperator("Invalid")},
		},
	}
	store := config.NewModuleConfigStore(state)

	_, err := New(testr.New(t), config.ControllerConfig{}, store, nil)
	if err == nil {
		t.Fatalf("expected error due to invalid selector")
	}
}

func TestCurrentPoliciesUsesStoreState(t *testing.T) {
	store := config.NewModuleConfigStore(moduleconfig.DefaultState())
	rec, err := New(testr.New(t), config.ControllerConfig{}, store, nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}

	updated := moduleconfig.DefaultState()
	updated.Settings.ManagedNodes.LabelKey = "gpu.deckhouse.io/custom"
	updated.Settings.ManagedNodes.EnabledByDefault = false
	updated.Settings.DeviceApproval.Mode = moduleconfig.DeviceApprovalModeSelector
	updated.Settings.DeviceApproval.Selector = &metav1.LabelSelector{
		MatchLabels: map[string]string{"gpu.deckhouse.io/product": "a100"},
	}
	store.Update(updated)

	managed, approval := rec.currentPolicies()
	if managed.LabelKey != "gpu.deckhouse.io/custom" || managed.EnabledByDefault {
		t.Fatalf("expected managed settings from store, got %+v", managed)
	}
	if !approval.AutoAttach(true, labels.Set{"gpu.deckhouse.io/product": "a100"}) {
		t.Fatalf("expected selector-based auto attach to match labels")
	}
}

func TestCurrentPoliciesFallsBackOnError(t *testing.T) {
	store := config.NewModuleConfigStore(moduleconfig.DefaultState())
	rec, err := New(testr.New(t), config.ControllerConfig{}, store, nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}

	fallbackManaged := rec.fallbackManaged
	fallbackApproval := rec.fallbackApproval
	rec.log = testr.New(t)

	invalid := moduleconfig.DefaultState()
	invalid.Settings.DeviceApproval.Mode = moduleconfig.DeviceApprovalModeSelector
	invalid.Settings.DeviceApproval.Selector = &metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{Key: "", Operator: metav1.LabelSelectorOpIn},
		},
	}
	invalid.Settings.ManagedNodes.LabelKey = "  "
	store.Update(invalid)

	managed, approval := rec.currentPolicies()
	if managed != fallbackManaged {
		t.Fatalf("expected fallback managed policy, got %+v", managed)
	}
	if approval != fallbackApproval {
		t.Fatalf("expected fallback approval policy, got %+v", approval)
	}
}

func TestCurrentPoliciesWithoutStoreUsesFallback(t *testing.T) {
	rec, err := New(testr.New(t), config.ControllerConfig{}, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}

	managed, approval := rec.currentPolicies()
	if managed != rec.fallbackManaged {
		t.Fatalf("expected fallback managed settings, got %+v", managed)
	}
	if approval != rec.fallbackApproval {
		t.Fatalf("expected fallback approval policy, got %+v", approval)
	}
}

func TestNewDefaultsLabelKeyWhenEmpty(t *testing.T) {
	module := defaultModuleSettings()
	module.ManagedNodes.LabelKey = "   "

	rec, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(module), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	if rec.fallbackManaged.LabelKey != defaultManagedNodeLabelKey {
		t.Fatalf("expected label key defaulted to %s, got %s", defaultManagedNodeLabelKey, rec.fallbackManaged.LabelKey)
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

	primary := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "worker-b-0-10de-2230",
			Labels: map[string]string{deviceNodeLabelKey: "worker-b", deviceIndexLabelKey: "0"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			NodeName: "worker-b",
			Managed:  true,
		},
	}
	orphan := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "obsolete-device",
			Labels: map[string]string{deviceNodeLabelKey: "worker-b", deviceIndexLabelKey: "99"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			NodeName: "worker-b",
		},
	}

	client := newTestClient(scheme, node, primary, orphan)

	handler := &trackingHandler{name: "noop"}
	module := defaultModuleSettings()
	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(module), []contracts.InventoryHandler{handler})
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

	if err := client.Get(ctx, types.NamespacedName{Name: orphan.Name}, &v1alpha1.GPUDevice{}); err == nil || !apierrors.IsNotFound(err) {
		t.Fatalf("orphan device should be deleted, got err=%v", err)
	}

	updated := &v1alpha1.GPUDevice{}
	if err := client.Get(ctx, types.NamespacedName{Name: primary.Name}, updated); err != nil {
		t.Fatalf("failed to get primary device: %v", err)
	}
	if updated.Status.Managed {
		t.Fatal("expected managed flag to be false after reconcile")
	}
	if updated.Labels[deviceIndexLabelKey] != "0" {
		t.Fatalf("expected index label to remain 0, got %s", updated.Labels[deviceIndexLabelKey])
	}

	inventory := &v1alpha1.GPUNodeInventory{}
	if err := client.Get(ctx, types.NamespacedName{Name: node.Name}, inventory); err != nil {
		t.Fatalf("inventory missing: %v", err)
	}
	if len(inventory.Status.Hardware.Devices) != 1 {
		t.Fatalf("inventory devices mismatch: %#v", inventory.Status.Hardware)
	}
	if cond := getCondition(inventory.Status.Conditions, conditionManagedDisabled); cond == nil || cond.Status != metav1.ConditionTrue {
		t.Fatalf("expected ManagedDisabled=true, got %+v", cond)
	}
	if cond := getCondition(inventory.Status.Conditions, conditionInventoryComplete); cond == nil || cond.Status != metav1.ConditionFalse {
		t.Fatalf("expected InventoryComplete=false, got %+v", cond)
	}
}

func TestReconcileTelemetryErrorDoesNotFail(t *testing.T) {
	module := defaultModuleSettings()
	scheme := newTestScheme(t)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-telemetry",
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "1db5",
				"gpu.deckhouse.io/device.00.class":  "0302",
			},
		},
	}

	baseClient := newTestClient(scheme, node)
	client := &delegatingClient{
		Client: baseClient,
		list: func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
			if _, ok := list.(*corev1.PodList); ok {
				return errors.New("pods list failed")
			}
			return baseClient.List(ctx, list, opts...)
		},
	}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(module), nil)
	if err != nil {
		t.Fatalf("unexpected reconciler error: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)
	reconciler.store = nil
	reconciler.fallbackMonitoring = true

	if _, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}}); err != nil {
		t.Fatalf("expected reconcile to succeed even when telemetry unavailable: %v", err)
	}
}

func TestReconcileCollectsTelemetryWhenAvailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := fmt.Fprintln(w, "dcgm_exporter_last_update_time_seconds 100"); err != nil {
			t.Fatalf("write metrics: %v", err)
		}
		if _, err := fmt.Fprintln(w, "DCGM_FI_DEV_GPU_TEMP{gpu=\"0\"} 50"); err != nil {
			t.Fatalf("write metrics: %v", err)
		}
	}))
	defer server.Close()

	host, portStr, _ := strings.Cut(server.Listener.Addr().String(), ":")
	port, _ := strconv.Atoi(portStr)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-telemetry-ok",
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "1db5",
				"gpu.deckhouse.io/device.00.class":  "0302",
			},
		},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dcgm-exporter-ok",
			Namespace: bootstrapmeta.WorkloadsNamespace,
			Labels:    map[string]string{"app": bootstrapmeta.AppName(bootstrapmeta.ComponentDCGMExporter)},
		},
		Spec: corev1.PodSpec{
			NodeName: node.Name,
			Containers: []corev1.Container{{
				Name:  "dcgm-exporter",
				Ports: []corev1.ContainerPort{{ContainerPort: int32(port)}},
			}},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			PodIP: host,
			Conditions: []corev1.PodCondition{{
				Type:   corev1.PodReady,
				Status: corev1.ConditionTrue,
			}},
		},
	}

	module := defaultModuleSettings()
	scheme := newTestScheme(t)
	baseClient := newTestClient(scheme, node, pod)

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(module), nil)
	if err != nil {
		t.Fatalf("unexpected reconciler error: %v", err)
	}
	reconciler.client = baseClient
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)
	reconciler.store = nil
	reconciler.fallbackMonitoring = true

	origClient := telemetryHTTPClient
	telemetryHTTPClient = server.Client()
	defer func() { telemetryHTTPClient = origClient }()

	if _, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}}); err != nil {
		t.Fatalf("expected reconcile to succeed: %v", err)
	}
}

func TestReconcileCollectsDetectionWhenAvailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := fmt.Fprint(w, `[{"index":0,"uuid":"GPU-AAA","product":"A100","memoryInfo":{"Total":1073741824,"Free":536870912,"Used":536870912},"powerUsage":1000,"powerManagementDefaultLimit":2000,"utilization":{"Gpu":50,"Memory":25},"memoryMiB":1024,"computeMajor":8,"computeMinor":0,"pci":{"address":"0000:17:00.0","vendor":"10de","device":"2203","class":"0302"},"pcie":{"generation":4,"width":16},"board":"board-1","family":"ampere","serial":"serial-1","displayMode":"Enabled","mig":{"capable":true,"profilesSupported":["mig-1g.10gb"]}}]`); err != nil {
			t.Fatalf("write detections: %v", err)
		}
	}))
	defer server.Close()

	host, portStr, _ := strings.Cut(server.Listener.Addr().String(), ":")
	port, _ := strconv.Atoi(portStr)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-detect-ok",
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "2203",
				"gpu.deckhouse.io/device.00.class":  "0302",
			},
		},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gpu-feature-discovery-detect",
			Namespace: bootstrapmeta.WorkloadsNamespace,
			Labels:    map[string]string{"app": bootstrapmeta.AppName(bootstrapmeta.ComponentGPUFeatureDiscovery)},
		},
		Spec: corev1.PodSpec{
			NodeName: node.Name,
			Containers: []corev1.Container{{
				Name:  "gfd-extender",
				Ports: []corev1.ContainerPort{{ContainerPort: int32(port)}},
			}},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			PodIP: host,
			Conditions: []corev1.PodCondition{{
				Type:   corev1.PodReady,
				Status: corev1.ConditionTrue,
			}},
		},
	}

	module := defaultModuleSettings()
	scheme := newTestScheme(t)
	baseClient := newTestClient(scheme, node, pod)

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(module), nil)
	if err != nil {
		t.Fatalf("unexpected reconciler error: %v", err)
	}
	reconciler.client = baseClient
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)
	reconciler.store = nil
	reconciler.fallbackMonitoring = true

	origDetectClient := detectHTTPClient
	detectHTTPClient = server.Client()
	defer func() { detectHTTPClient = origDetectClient }()

	detections, err := reconciler.collectNodeDetections(context.Background(), node.Name)
	if err != nil {
		t.Fatalf("expected detections collected: %v", err)
	}
	entry, ok := detections.find(deviceSnapshot{Index: "0"})
	if !ok {
		t.Fatalf("detection entry for index 0 not found: %+v", detections)
	}
	if entry.Product != "A100" || entry.PCI.Address != "0000:17:00.0" || entry.ComputeMajor != 8 {
		t.Fatalf("unexpected detection data: %+v", entry)
	}
}

func TestReconcilePassesDetectionsToDevice(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := fmt.Fprint(w, `[{"index":0,"uuid":"GPU-AAA","product":"A100","memoryInfo":{"Total":1024,"Free":512,"Used":512},"powerUsage":1000,"utilization":{"Gpu":10,"Memory":5},"memoryMiB":1024,"computeMajor":8,"computeMinor":0}]`); err != nil {
			t.Fatalf("write detections: %v", err)
		}
	}))
	defer server.Close()

	host, portStr, _ := strings.Cut(server.Listener.Addr().String(), ":")
	port, _ := strconv.Atoi(portStr)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-detect-reconcile",
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "2203",
				"gpu.deckhouse.io/device.00.class":  "0302",
			},
		},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gfd-extender-reconcile",
			Namespace: bootstrapmeta.WorkloadsNamespace,
			Labels:    map[string]string{"app": bootstrapmeta.AppName(bootstrapmeta.ComponentGPUFeatureDiscovery)},
		},
		Spec: corev1.PodSpec{
			NodeName: node.Name,
			Containers: []corev1.Container{{
				Name:  "gfd-extender",
				Ports: []corev1.ContainerPort{{ContainerPort: int32(port)}},
			}},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			PodIP: host,
			Conditions: []corev1.PodCondition{{
				Type:   corev1.PodReady,
				Status: corev1.ConditionTrue,
			}},
		},
	}

	module := defaultModuleSettings()
	scheme := newTestScheme(t)
	baseClient := newTestClient(scheme, node, pod)

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(module), nil)
	if err != nil {
		t.Fatalf("unexpected reconciler error: %v", err)
	}
	reconciler.client = baseClient
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)
	reconciler.store = nil
	reconciler.fallbackMonitoring = true

	origDetectClient := detectHTTPClient
	detectHTTPClient = server.Client()
	defer func() { detectHTTPClient = origDetectClient }()

	if _, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}}); err != nil {
		t.Fatalf("expected reconcile to succeed: %v", err)
	}

	device := &v1alpha1.GPUDevice{}
	if err := baseClient.Get(context.Background(), types.NamespacedName{Name: "worker-detect-reconcile-0-10de-2203"}, device); err != nil {
		t.Fatalf("expected device created: %v", err)
	}
}

func TestReconcileNodeNotFoundTriggersCleanup(t *testing.T) {
	module := defaultModuleSettings()
	scheme := newTestScheme(t)
	baseClient := newTestClient(scheme)

	rec, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(module), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	rec.client = &delegatingClient{
		Client: baseClient,
		get: func(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
			return apierrors.NewNotFound(schema.GroupResource{Group: "", Resource: "nodes"}, "missing")
		},
	}
	rec.scheme = scheme
	rec.recorder = record.NewFakeRecorder(32)

	if _, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "missing-node"}}); err != nil {
		t.Fatalf("expected reconcile to succeed on missing node: %v", err)
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
	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "worker-meta-0-10de-1db5",
			Labels: map[string]string{},
		},
		Status: v1alpha1.GPUDeviceStatus{},
	}

	baseClient := newTestClient(scheme, node, device)
	var deviceGets int
	client := &delegatingClient{
		Client: baseClient,
		get: func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if _, ok := obj.(*v1alpha1.GPUDevice); ok {
				deviceGets++
			}
			return baseClient.Get(ctx, key, obj, opts...)
		},
	}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
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

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = runtime.NewScheme() // missing core types to force owner reference failure
	reconciler.recorder = record.NewFakeRecorder(32)

	snapshot := deviceSnapshot{Index: "0", Vendor: "10de", Device: "1db5", Class: "0302"}
	_, _, err = reconciler.reconcileDevice(context.Background(), node, snapshot, map[string]string{}, true, reconciler.fallbackApproval, nodeTelemetry{}, nodeDetection{})
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
			if _, ok := obj.(*v1alpha1.GPUDevice); ok {
				return getErr
			}
			return baseClient.Get(ctx, key, obj, opts...)
		},
	}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	snapshot := deviceSnapshot{Index: "0", Vendor: "10de", Device: "1db5", Class: "0302"}
	_, _, err = reconciler.reconcileDevice(context.Background(), node, snapshot, map[string]string{}, true, reconciler.fallbackApproval, nodeTelemetry{}, nodeDetection{})
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
	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "worker-metadata-error-0-10de-1db5",
			Labels: map[string]string{},
		},
		Status: v1alpha1.GPUDeviceStatus{},
	}

	baseClient := newTestClient(scheme, node, device)
	patchErr := errors.New("patch failed")
	client := &delegatingClient{Client: baseClient, patch: func(context.Context, client.Object, client.Patch, ...client.PatchOption) error {
		return patchErr
	}}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	snapshot := deviceSnapshot{Index: "0", Vendor: "10de", Device: "1db5", Class: "0302"}
	_, _, err = reconciler.reconcileDevice(context.Background(), node, snapshot, map[string]string{}, true, reconciler.fallbackApproval, nodeTelemetry{}, nodeDetection{})
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

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	snapshot := deviceSnapshot{Index: "0", Vendor: "10de", Device: "1db5", Class: "0302"}
	_, _, err = reconciler.reconcileDevice(context.Background(), node, snapshot, map[string]string{}, true, reconciler.fallbackApproval, nodeTelemetry{}, nodeDetection{})
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

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	snapshot := deviceSnapshot{Index: "0", Vendor: "10de", Device: "1db5", Class: "0302"}
	device, result, err := reconciler.reconcileDevice(context.Background(), node, snapshot, map[string]string{}, true, reconciler.fallbackApproval, nodeTelemetry{}, nodeDetection{})
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
	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "worker-nochange-0-10de-1db5",
			Labels: map[string]string{deviceNodeLabelKey: "worker-nochange", deviceIndexLabelKey: "0"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			NodeName:    "worker-nochange",
			InventoryID: buildInventoryID("worker-nochange", deviceSnapshot{Index: "0", Vendor: "10de", Device: "1db5"}),
			Managed:     true,
			Hardware: v1alpha1.GPUDeviceHardware{
				PCI:               v1alpha1.PCIAddress{Vendor: "10de", Device: "1db5", Class: "0302"},
				Product:           "Existing",
				MemoryMiB:         1024,
				MIG:               v1alpha1.GPUMIGConfig{},
				Precision:         v1alpha1.GPUPrecision{Supported: []string{"fp32"}},
				ComputeCapability: &v1alpha1.GPUComputeCapability{Major: 8, Minor: 0},
			},
		},
	}

	baseClient := newTestClient(scheme, node, device)
	tracker := &trackingStatusWriter{StatusWriter: baseClient.Status()}
	client := &delegatingClient{Client: baseClient, statusWriter: tracker}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
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
	_, _, err = reconciler.reconcileDevice(context.Background(), node, snapshot, map[string]string{}, true, reconciler.fallbackApproval, nodeTelemetry{}, nodeDetection{})
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
	primary := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "worker-delete-error-0-10de-2230",
			Labels: map[string]string{deviceNodeLabelKey: "worker-delete-error", deviceIndexLabelKey: "0"},
		},
		Status: v1alpha1.GPUDeviceStatus{NodeName: "worker-delete-error"},
	}
	orphan := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "worker-delete-error-orphan",
			Labels: map[string]string{deviceNodeLabelKey: "worker-delete-error", deviceIndexLabelKey: "99"},
		},
		Status: v1alpha1.GPUDeviceStatus{NodeName: "worker-delete-error"},
	}

	baseClient := newTestClient(scheme, node, primary, orphan)
	delErr := errors.New("delete failure")
	client := &delegatingClient{
		Client: baseClient,
		delete: func(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
			if dev, ok := obj.(*v1alpha1.GPUDevice); ok && dev.Name == orphan.Name {
				return delErr
			}
			return baseClient.Delete(ctx, obj, opts...)
		},
	}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
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

func TestReconcileReturnsDeviceListError(t *testing.T) {
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
	feature := &nfdv1alpha1.NodeFeature{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-list-error"},
	}

	baseClient := newTestClient(scheme, node, feature)
	listErr := errors.New("list failure")
	client := &delegatingClient{
		Client: baseClient,
		list: func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
			if _, ok := list.(*v1alpha1.GPUDeviceList); ok {
				return listErr
			}
			return baseClient.List(ctx, list, opts...)
		},
	}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	_, err = reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}})
	if !errors.Is(err, listErr) {
		t.Fatalf("expected device list error, got %v", err)
	}
}

func TestReconcileCleanupOnMissingNode(t *testing.T) {
	scheme := newTestScheme(t)
	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "worker-c-0-10de-1db5",
			Labels: map[string]string{deviceNodeLabelKey: "worker-c", deviceIndexLabelKey: "0"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			NodeName: "worker-c",
		},
	}
	inventory := &v1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-c"},
		Spec:       v1alpha1.GPUNodeInventorySpec{NodeName: "worker-c"},
	}

	client := newTestClient(scheme, device, inventory)

	module := defaultModuleSettings()
	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(module), nil)
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

	if err := client.Get(ctx, types.NamespacedName{Name: device.Name}, &v1alpha1.GPUDevice{}); err == nil || !apierrors.IsNotFound(err) {
		t.Fatalf("expected device to be removed, err=%v", err)
	}
	if err := client.Get(ctx, types.NamespacedName{Name: inventory.Name}, &v1alpha1.GPUNodeInventory{}); err == nil || !apierrors.IsNotFound(err) {
		t.Fatalf("expected inventory to be removed, err=%v", err)
	}
}

func TestFindNodeFeaturePrefersExactMatch(t *testing.T) {
	scheme := newTestScheme(t)
	exact := &nfdv1alpha1.NodeFeature{ObjectMeta: metav1.ObjectMeta{Name: "worker-exact", ResourceVersion: "5"}}
	labeled := &nfdv1alpha1.NodeFeature{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "worker-exact-labeled",
			Namespace:       "gpu-operator",
			ResourceVersion: "7",
			Labels:          map[string]string{nodeFeatureNodeNameLabel: "worker-exact"},
		},
	}

	client := newTestClient(scheme, exact, labeled)
	reconciler := &Reconciler{client: client}

	feature, err := reconciler.findNodeFeature(context.Background(), "worker-exact")
	if err != nil {
		t.Fatalf("findNodeFeature returned error: %v", err)
	}
	if feature == nil || feature.GetName() != "worker-exact" {
		t.Fatalf("expected exact NodeFeature, got %+v", feature)
	}
}

func TestFindNodeFeatureSelectsNewestByResourceVersion(t *testing.T) {
	scheme := newTestScheme(t)
	older := &nfdv1alpha1.NodeFeature{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "nfd-old",
			Namespace:       "gpu-operator",
			ResourceVersion: "5",
			Labels:          map[string]string{nodeFeatureNodeNameLabel: "worker-rv"},
		},
	}
	newer := &nfdv1alpha1.NodeFeature{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "nfd-new",
			Namespace:       "gpu-operator",
			ResourceVersion: "8",
			Labels:          map[string]string{nodeFeatureNodeNameLabel: "worker-rv"},
		},
	}

	client := newTestClient(scheme, older, newer)
	reconciler := &Reconciler{client: client}

	feature, err := reconciler.findNodeFeature(context.Background(), "worker-rv")
	if err != nil {
		t.Fatalf("findNodeFeature returned error: %v", err)
	}
	if feature == nil || feature.GetName() != "nfd-new" {
		t.Fatalf("expected newest NodeFeature, got %+v", feature)
	}
}

func TestFindNodeFeatureReturnsNilWhenMissing(t *testing.T) {
	reconciler := &Reconciler{client: newTestClient(newTestScheme(t))}
	feature, err := reconciler.findNodeFeature(context.Background(), "absent")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if feature != nil {
		t.Fatalf("expected nil feature, got %+v", feature)
	}
}

func TestResourceVersionNewer(t *testing.T) {
	cases := []struct {
		name      string
		candidate string
		current   string
		expect    bool
	}{
		{"empty candidate", "", "10", false},
		{"empty current", "5", "", true},
		{"numeric greater", "6", "5", true},
		{"numeric smaller", "4", "5", false},
		{"candidate numeric current non-numeric", "5", "xyz", true},
		{"candidate non-numeric current numeric", "abc", "4", false},
		{"both non-numeric", "def", "abc", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resourceVersionNewer(tc.candidate, tc.current); got != tc.expect {
				t.Fatalf("resourceVersionNewer(%q,%q)=%v, want %v", tc.candidate, tc.current, got, tc.expect)
			}
		})
	}
}

func TestFindNodeFeatureGetError(t *testing.T) {
	scheme := newTestScheme(t)
	baseClient := newTestClient(scheme)
	boom := errors.New("get failed")
	client := &delegatingClient{
		Client: baseClient,
		get: func(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
			return boom
		},
	}

	reconciler := &Reconciler{client: client}

	_, err := reconciler.findNodeFeature(context.Background(), "node-error")
	if !errors.Is(err, boom) {
		t.Fatalf("expected get error, got %v", err)
	}
}

func TestFindNodeFeatureListError(t *testing.T) {
	scheme := newTestScheme(t)
	baseClient := newTestClient(scheme)
	boom := errors.New("list failed")
	client := &delegatingClient{
		Client: baseClient,
		get: func(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
			return apierrors.NewNotFound(schema.GroupResource{Group: "nfd", Resource: "nodefeatures"}, "node")
		},
		list: func(context.Context, client.ObjectList, ...client.ListOption) error {
			return boom
		},
	}

	reconciler := &Reconciler{client: client}

	_, err := reconciler.findNodeFeature(context.Background(), "node-list")
	if !errors.Is(err, boom) {
		t.Fatalf("expected list error, got %v", err)
	}
}

func TestChooseNodeFeaturePrefersExactName(t *testing.T) {
	items := []nfdv1alpha1.NodeFeature{
		{ObjectMeta: metav1.ObjectMeta{Name: "other", ResourceVersion: "1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node-1", ResourceVersion: "2"}},
	}
	selected := chooseNodeFeature(items, "node-1")
	if selected == nil || selected.GetName() != "node-1" {
		t.Fatalf("expected exact match, got %+v", selected)
	}
}

func TestChooseNodeFeatureUsesNewestResourceVersion(t *testing.T) {
	items := []nfdv1alpha1.NodeFeature{
		{ObjectMeta: metav1.ObjectMeta{Name: "nf-1", ResourceVersion: "5"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "nf-2", ResourceVersion: "7"}},
	}
	selected := chooseNodeFeature(items, "node")
	if selected == nil || selected.GetName() != "nf-2" {
		t.Fatalf("expected latest resource version, got %+v", selected)
	}
}

func TestChooseNodeFeatureHandlesEmptySlice(t *testing.T) {
	if chooseNodeFeature(nil, "node") != nil {
		t.Fatal("expected nil when slice empty")
	}
}

func TestReconcileDefaultsResyncPeriodWhenZero(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-no-requeue",
			UID:  types.UID("node-no-requeue"),
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "1db5",
				"gpu.deckhouse.io/device.00.class":  "0302",
			},
		},
	}

	client := newTestClient(scheme, node)
	reconciler, err := New(testr.New(t), config.ControllerConfig{ResyncPeriod: 0}, moduleStoreFrom(defaultModuleSettings()), nil)
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
		t.Fatalf("expected default resync period of %s, got %+v", defaultResyncPeriod, res)
	}
}

func TestReconcileSkipsFollowupWhenResyncDisabled(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-no-followup",
			UID:  types.UID("worker-no-followup"),
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "1db5",
				"gpu.deckhouse.io/device.00.class":  "0302",
			},
		},
	}

	client := newTestClient(scheme, node)
	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)
	reconciler.setResyncPeriod(0)

	res, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}})
	if err != nil {
		t.Fatalf("unexpected reconcile error: %v", err)
	}
	if res.Requeue {
		t.Fatalf("expected no requeue, got %+v", res)
	}
	if res.RequeueAfter != 0 {
		t.Fatalf("expected no delayed requeue, got %+v", res)
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
	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(module), nil)
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
	if res.RequeueAfter != defaultResyncPeriod {
		t.Fatalf("expected default resync period (%s), got %+v", defaultResyncPeriod, res)
	}

	inventory := &v1alpha1.GPUNodeInventory{}
	if err := client.Get(ctx, types.NamespacedName{Name: node.Name}, inventory); err != nil {
		t.Fatalf("expected inventory to be created, got error: %v", err)
	}
	condition := apimeta.FindStatusCondition(inventory.Status.Conditions, conditionInventoryComplete)
	if condition == nil {
		t.Fatalf("expected inventory condition to be set")
	}
	if condition.Status != metav1.ConditionFalse || condition.Reason != reasonNodeFeatureMissing {
		t.Fatalf("expected inventory condition (false, %s), got status=%s reason=%s", reasonNodeFeatureMissing, condition.Status, condition.Reason)
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

	inventory := &v1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{
			Name: node.Name,
			OwnerReferences: []metav1.OwnerReference{
				{APIVersion: "v1", Kind: "Node", Name: node.Name, UID: node.UID},
			},
		},
		Spec: v1alpha1.GPUNodeInventorySpec{NodeName: node.Name},
	}

	client := newTestClient(scheme, node, feature, inventory)
	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	ctx := context.Background()
	res, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}})
	if err != nil {
		t.Fatalf("unexpected reconcile error: %v", err)
	}
	if res.RequeueAfter != 0 {
		t.Fatalf("expected no resync when node lacks devices, got %+v", res)
	}

	updated := &v1alpha1.GPUNodeInventory{}
	if err := client.Get(ctx, types.NamespacedName{Name: node.Name}, updated); err != nil {
		t.Fatalf("expected inventory to exist, got error: %v", err)
	}
	if updated.Status.Hardware.Present {
		t.Fatalf("expected hardware.present=false, got true")
	}
	if len(updated.Status.Hardware.Devices) != 0 {
		t.Fatalf("expected no devices recorded, got %d", len(updated.Status.Hardware.Devices))
	}
	cond := apimeta.FindStatusCondition(updated.Status.Conditions, conditionInventoryComplete)
	if cond == nil || cond.Status != metav1.ConditionFalse || cond.Reason != reasonNoDevicesDiscovered {
		t.Fatalf("expected inventory condition (false, %s), got %+v", reasonNoDevicesDiscovered, cond)
	}
	if value := testutil.ToFloat64(inventoryDevicesGauge.WithLabelValues(node.Name)); value != 0 {
		t.Fatalf("expected devices gauge 0, got %f", value)
	}
}

func TestReconcileDeletesExistingInventoryWhenDevicesDisappear(t *testing.T) {
	inventoryDevicesGauge.Reset()
	inventoryConditionGauge.Reset()
	inventoryDeviceStateGauge.Reset()

	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-stale",
			UID:  types.UID("worker-stale"),
		},
	}
	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name: "stale-device",
		},
	}
	device.Status.NodeName = node.Name
	device.Status.InventoryID = "stale-inventory"

	inventory := &v1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{
			Name: node.Name,
		},
		Spec: v1alpha1.GPUNodeInventorySpec{
			NodeName: node.Name,
		},
	}

	client := newTestClient(scheme, node, device, inventory)

	inventoryDevicesGauge.WithLabelValues(node.Name).Set(5)
	inventoryConditionGauge.WithLabelValues(node.Name, conditionManagedDisabled).Set(1)
	inventoryConditionGauge.WithLabelValues(node.Name, conditionInventoryComplete).Set(1)

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
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
	if res.RequeueAfter != 0 {
		t.Fatalf("expected no follow-up when node no longer has devices, got %+v", res)
	}

	if err := client.Get(ctx, types.NamespacedName{Name: device.Name}, &v1alpha1.GPUDevice{}); !apierrors.IsNotFound(err) {
		if err != nil {
			t.Fatalf("expected GPUDevice to be deleted, got error: %v", err)
		} else {
			t.Fatalf("expected GPUDevice to be deleted, but resource still exists")
		}
	}
	persisted := &v1alpha1.GPUNodeInventory{}
	if err := client.Get(ctx, types.NamespacedName{Name: node.Name}, persisted); err != nil {
		t.Fatalf("expected inventory to persist, got error: %v", err)
	}
	if persisted.Status.Hardware.Present {
		t.Fatalf("expected hardware.present=false after cleanup")
	}
	if len(persisted.Status.Hardware.Devices) != 0 {
		t.Fatalf("expected no devices recorded, got %d", len(persisted.Status.Hardware.Devices))
	}
	if value := testutil.ToFloat64(inventoryDevicesGauge.WithLabelValues(node.Name)); value != 0 {
		t.Fatalf("expected devices gauge 0, got %f", value)
	}
}

func TestReconcileReturnsInventoryGetError(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-get-inventory",
			UID:  types.UID("worker-get-inventory"),
		},
	}

	baseClient := newTestClient(scheme, node)
	getErr := errors.New("inventory get failure")
	client := &delegatingClient{
		Client: baseClient,
		get: func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			switch obj.(type) {
			case *corev1.Node:
				return baseClient.Get(ctx, key, obj, opts...)
			case *v1alpha1.GPUNodeInventory:
				return getErr
			default:
				return baseClient.Get(ctx, key, obj, opts...)
			}
		},
	}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	_, err = reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}})
	if !errors.Is(err, getErr) {
		t.Fatalf("expected inventory get error, got %v", err)
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

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
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

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
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

func TestReconcileReturnsErrorOnNodeFeatureListFailure(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-feature-list-error",
			UID:  types.UID("node-feature-list-error"),
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "1db5",
				"gpu.deckhouse.io/device.00.class":  "0302",
			},
		},
	}

	baseClient := newTestClient(scheme, node)
	listErr := errors.New("feature list failed")
	client := &delegatingClient{
		Client: baseClient,
		list: func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
			switch list.(type) {
			case *nfdv1alpha1.NodeFeatureList:
				return listErr
			default:
				return baseClient.List(ctx, list, opts...)
			}
		},
	}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	if _, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}}); !errors.Is(err, listErr) {
		t.Fatalf("expected feature list error, got %v", err)
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

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
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
	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), []contracts.InventoryHandler{errorHandler{err: handlerErr}})
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
	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), []contracts.InventoryHandler{
		resultHandler{name: "requeue", result: contracts.Result{Requeue: true, RequeueAfter: 45 * time.Second}},
		resultHandler{name: "after", result: contracts.Result{RequeueAfter: 10 * time.Second}},
	})
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)
	reconciler.setResyncPeriod(0)

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
	module := defaultModuleSettings()
	module.Inventory.ResyncPeriod = "5s"
	reconciler, err := New(testr.New(t), config.ControllerConfig{ResyncPeriod: 5 * time.Second}, moduleStoreFrom(module), nil)
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
	if res.RequeueAfter != 0 {
		t.Fatalf("expected no resync when there are no devices, got %s", res.RequeueAfter)
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
	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
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
	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   deviceName,
			Labels: map[string]string{deviceNodeLabelKey: "worker-conflict", deviceIndexLabelKey: "0"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			NodeName: "worker-conflict",
		},
	}

	baseClient := newTestClient(scheme, node, device)
	conflictWriter := &conflictStatusWriter{StatusWriter: baseClient.Status()}
	client := &delegatingClient{Client: baseClient, statusWriter: conflictWriter}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)
	reconciler.setResyncPeriod(0)

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
	inventory := &v1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{Name: node.Name},
		Spec:       v1alpha1.GPUNodeInventorySpec{NodeName: "stale"},
	}

	baseClient := newTestClient(scheme, node, inventory)
	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = baseClient
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	if _, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}}); err != nil {
		t.Fatalf("unexpected reconcile error: %v", err)
	}

	updated := &v1alpha1.GPUNodeInventory{}
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

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
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

func TestReconcileNodeInventoryMarksNoDevicesDiscovered(t *testing.T) {
	inventoryDevicesGauge.Reset()
	inventoryConditionGauge.Reset()
	inventoryDeviceStateGauge.Reset()

	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-no-devices-inventory",
			UID:  types.UID("worker-no-devices-inventory"),
		},
	}

	existingInventory := &v1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{
			Name: node.Name,
			OwnerReferences: []metav1.OwnerReference{
				{APIVersion: "v1", Kind: "Node", Name: node.Name, UID: node.UID},
			},
		},
		Spec: v1alpha1.GPUNodeInventorySpec{NodeName: node.Name},
	}

	client := newTestClient(scheme, node, existingInventory)
	reconciler := &Reconciler{
		client:   client,
		scheme:   scheme,
		recorder: record.NewFakeRecorder(32),
		log:      testr.New(t),
	}
	reconciler.setResyncPeriod(defaultResyncPeriod)

	snapshot := nodeSnapshot{
		Managed:         true,
		FeatureDetected: true,
		Labels:          map[string]string{},
	}

	if err := reconciler.reconcileNodeInventory(context.Background(), node, snapshot, nil, ManagedNodesPolicy{EnabledByDefault: true}, nodeDetection{}); err != nil {
		t.Fatalf("unexpected reconcileNodeInventory error: %v", err)
	}

	inventory := &v1alpha1.GPUNodeInventory{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: node.Name}, inventory); err != nil {
		t.Fatalf("expected inventory to exist, got error: %v", err)
	}

	condition := apimeta.FindStatusCondition(inventory.Status.Conditions, conditionInventoryComplete)
	if condition == nil {
		t.Fatalf("expected inventory condition to be present")
	}
	if condition.Reason != reasonNoDevicesDiscovered || condition.Status != metav1.ConditionFalse {
		t.Fatalf("expected condition=%s/false, got reason=%s status=%s", reasonNoDevicesDiscovered, condition.Reason, condition.Status)
	}
}

func TestReconcileNodeInventorySkipsCreationWhenNoDevices(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-no-devices-new",
			UID:  types.UID("worker-no-devices-new"),
		},
	}
	client := newTestClient(scheme, node)
	reconciler := &Reconciler{
		client:   client,
		scheme:   scheme,
		recorder: record.NewFakeRecorder(32),
		log:      testr.New(t),
	}

	snapshot := nodeSnapshot{
		Managed:         true,
		FeatureDetected: true,
		Labels:          map[string]string{},
	}

	if err := reconciler.reconcileNodeInventory(context.Background(), node, snapshot, nil, ManagedNodesPolicy{EnabledByDefault: true}, nodeDetection{}); err != nil {
		t.Fatalf("unexpected reconcileNodeInventory error: %v", err)
	}

	inventory := &v1alpha1.GPUNodeInventory{}
	err := client.Get(context.Background(), types.NamespacedName{Name: node.Name}, inventory)
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected inventory to be absent, got err=%v obj=%#v", err, inventory)
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
	reconciler := &Reconciler{client: client, scheme: runtime.NewScheme(), recorder: record.NewFakeRecorder(32), fallbackManaged: ManagedNodesPolicy{LabelKey: "gpu.deckhouse.io/enabled", EnabledByDefault: true}}

	snapshot := buildNodeSnapshot(node, nil, reconciler.fallbackManaged)

	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "dev-owner-ref",
			Labels: map[string]string{deviceIndexLabelKey: "00"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "owner-ref-00",
		},
	}

	err := reconciler.reconcileNodeInventory(context.Background(), node, snapshot, []*v1alpha1.GPUDevice{device}, reconciler.fallbackManaged, nodeDetection{})
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
	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "worker-unknown-device-orphan",
			Labels: map[string]string{deviceNodeLabelKey: "worker-unknown-device", deviceIndexLabelKey: "99"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			NodeName:    "worker-unknown-device",
			InventoryID: "worker-unknown-device-99",
			Hardware:    v1alpha1.GPUDeviceHardware{Product: "Existing", MemoryMiB: 256},
		},
	}

	client := newTestClient(scheme, node, device)
	reconciler := &Reconciler{client: client, scheme: scheme, recorder: record.NewFakeRecorder(32), fallbackManaged: managedPolicyFrom(defaultModuleSettings())}

	snapshot := nodeSnapshot{Managed: true, Devices: []deviceSnapshot{}}
	if err := reconciler.reconcileNodeInventory(context.Background(), node, snapshot, []*v1alpha1.GPUDevice{device}, reconciler.fallbackManaged, nodeDetection{}); err != nil {
		t.Fatalf("unexpected reconcileNodeInventory error: %v", err)
	}
	updated := &v1alpha1.GPUNodeInventory{}
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
	device := &v1alpha1.GPUDevice{
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
	device := &v1alpha1.GPUDevice{
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
	device := &v1alpha1.GPUDevice{ObjectMeta: metav1.ObjectMeta{Name: "metadata-device"}}
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

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), []contracts.InventoryHandler{errHandler})
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
	inventoryConditionGauge.WithLabelValues(nodeName, conditionInventoryComplete).Set(1)

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
	if len(builder.watchedSources) != 1 || builder.watchedSources[0] != fakeSource {
		t.Fatalf("expected node feature source to be passed to builder, got %d watchers", len(builder.watchedSources))
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

func TestRequeueAllNodes(t *testing.T) {
	scheme := newTestScheme(t)
	deviceA := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "dev-a"},
		Status:     v1alpha1.GPUDeviceStatus{NodeName: "node-a"},
	}
	deviceB := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "dev-b"},
		Status:     v1alpha1.GPUDeviceStatus{NodeName: "node-b"},
	}
	reconciler := &Reconciler{client: newTestClient(scheme, deviceA, deviceB)}

	reqs := reconciler.requeueAllNodes(context.Background())
	if len(reqs) != 2 {
		t.Fatalf("expected two requests, got %#v", reqs)
	}
	expected := map[string]struct{}{"node-a": {}, "node-b": {}}
	for _, req := range reqs {
		if _, ok := expected[req.Name]; !ok {
			t.Fatalf("unexpected request %#v", req)
		}
	}

	withEmpty := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "dev-empty"},
		Status:     v1alpha1.GPUDeviceStatus{NodeName: ""},
	}
	reconciler = &Reconciler{client: newTestClient(scheme, deviceA, deviceB, withEmpty)}
	if reqs := reconciler.requeueAllNodes(context.Background()); len(reqs) != 2 {
		t.Fatalf("expected empty node to be skipped, got %v", reqs)
	}
}

func TestRequeueAllNodesHandlesError(t *testing.T) {
	reconciler := &Reconciler{
		client: &failingListClient{err: errors.New("list fail")},
		log:    testr.New(t),
	}

	if reqs := reconciler.requeueAllNodes(context.Background()); len(reqs) != 0 {
		t.Fatalf("expected empty result on error, got %#v", reqs)
	}
}

func TestRequeueAllNodesEmptyList(t *testing.T) {
	scheme := newTestScheme(t)
	reconciler := &Reconciler{client: newTestClient(scheme)}
	reqs := reconciler.requeueAllNodes(context.Background())
	if len(reqs) != 0 {
		t.Fatalf("expected no requests for empty device list, got %v", reqs)
	}

	reconciler.log = testr.New(t)
	reconciler.client = &failingListClient{err: errors.New("list err")}
	if reqs := reconciler.requeueAllNodes(context.Background()); len(reqs) != 0 {
		t.Fatalf("expected error branch to return empty slice, got %v", reqs)
	}

	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "dev-no-node"},
		Status:     v1alpha1.GPUDeviceStatus{NodeName: ""},
	}
	reconciler = &Reconciler{client: newTestClient(scheme, device)}
	if reqs := reconciler.requeueAllNodes(context.Background()); len(reqs) != 0 {
		t.Fatalf("expected devices without nodeName to be skipped, got %v", reqs)
	}
}

func TestMapModuleConfigRequeuesNodes(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-map"}}
	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "dev-map"},
		Status:     v1alpha1.GPUDeviceStatus{NodeName: node.Name},
	}
	reconciler := &Reconciler{
		client: newTestClient(scheme, node, device),
	}

	reqs := reconciler.mapModuleConfig(context.Background(), nil)
	if len(reqs) != 1 || reqs[0].Name != node.Name {
		t.Fatalf("unexpected requests returned from mapModuleConfig: %#v", reqs)
	}
}

func TestMapModuleConfigSkipsNodesWithoutDevices(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-empty"}}
	reconciler := &Reconciler{
		client: newTestClient(scheme, node),
	}

	if reqs := reconciler.mapModuleConfig(context.Background(), nil); len(reqs) != 0 {
		t.Fatalf("expected no requests when there are no GPU devices, got %#v", reqs)
	}
}

func TestMapModuleConfigCleansResourcesWhenDisabled(t *testing.T) {
	scheme := newTestScheme(t)
	inv := &v1alpha1.GPUNodeInventory{ObjectMeta: metav1.ObjectMeta{Name: "node1"}}
	dev := &v1alpha1.GPUDevice{ObjectMeta: metav1.ObjectMeta{Name: "dev1"}, Status: v1alpha1.GPUDeviceStatus{NodeName: "node1"}}
	client := newTestClient(scheme, inv, dev)

	store := config.NewModuleConfigStore(moduleconfig.State{Enabled: false, Settings: moduleconfig.DefaultState().Settings})

	rec := &Reconciler{
		client:          client,
		store:           store,
		fallbackManaged: ManagedNodesPolicy{LabelKey: "gpu.deckhouse.io/enabled", EnabledByDefault: true},
	}
	if reqs := rec.mapModuleConfig(context.Background(), nil); len(reqs) != 0 {
		t.Fatalf("expected no requeues when module disabled")
	}
	if err := client.Get(context.Background(), types.NamespacedName{Name: "node1"}, &v1alpha1.GPUNodeInventory{}); !apierrors.IsNotFound(err) {
		t.Fatalf("inventory should be deleted on disable, got %v", err)
	}
	if err := client.Get(context.Background(), types.NamespacedName{Name: "dev1"}, &v1alpha1.GPUDevice{}); !apierrors.IsNotFound(err) {
		t.Fatalf("device should be deleted on disable, got %v", err)
	}
}

func TestMapModuleConfigCleanupErrorIsIgnored(t *testing.T) {
	rec := &Reconciler{
		client: &listErrorClient{err: errors.New("cleanup fail")},
		log:    testr.New(t),
		store:  config.NewModuleConfigStore(moduleconfig.State{Enabled: false, Settings: moduleconfig.DefaultState().Settings}),
	}
	if reqs := rec.mapModuleConfig(context.Background(), nil); reqs != nil {
		t.Fatalf("expected nil requests when cleanup fails, got %#v", reqs)
	}
}

func TestCleanupAllInventoriesListError(t *testing.T) {
	rec := &Reconciler{
		client: &listErrorClient{err: errors.New("list inv")},
	}
	if err := rec.cleanupAllInventories(context.Background()); err == nil || !strings.Contains(err.Error(), "list inv") {
		t.Fatalf("expected list error, got %v", err)
	}
}

func TestCleanupAllInventoriesDevicesListError(t *testing.T) {
	scheme := newTestScheme(t)
	inv := &v1alpha1.GPUNodeInventory{ObjectMeta: metav1.ObjectMeta{Name: "node1"}}
	dev := &v1alpha1.GPUDevice{ObjectMeta: metav1.ObjectMeta{Name: "dev1"}, Status: v1alpha1.GPUDeviceStatus{NodeName: "node1"}}
	base := newTestClient(scheme, inv, dev)
	rec := &Reconciler{
		client: &listErrorClient{Client: base, err: errors.New("list devs"), failOnSecond: true},
	}
	if err := rec.cleanupAllInventories(context.Background()); err == nil || !strings.Contains(err.Error(), "list devs") {
		t.Fatalf("expected device list error, got %v", err)
	}
}

func TestSetupWithDependenciesAddsModuleWatcher(t *testing.T) {
	scheme := newTestScheme(t)
	indexer := &fakeFieldIndexer{}
	builder := &fakeControllerBuilder{}
	fakeSource := &fakeSyncingSource{}
	rec, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}

	deps := setupDependencies{
		client:            newTestClient(scheme),
		scheme:            scheme,
		recorder:          record.NewFakeRecorder(1),
		indexer:           indexer,
		cache:             dummyCache{},
		nodeFeatureSource: fakeSource,
		builder:           builder,
	}

	if err := rec.setupWithDependencies(context.Background(), deps); err != nil {
		t.Fatalf("setupWithDependencies returned error: %v", err)
	}
	if len(builder.watchedSources) != 2 {
		t.Fatalf("expected two watchers (node feature and module config), got %d", len(builder.watchedSources))
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

	rec, err := New(testr.New(t), config.ControllerConfig{Workers: 2}, moduleStoreFrom(defaultModuleSettings()), nil)
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
	if len(builder.watchedSources) == 0 || builder.watchedSources[0] != fakeSource {
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

func TestSetupWithDependenciesModuleWatcher(t *testing.T) {
	scheme := newTestScheme(t)
	builder := &fakeControllerBuilder{}
	rec := &Reconciler{
		cfg: config.ControllerConfig{Workers: 1},
	}
	var watcherCalled bool
	rec.moduleWatcherFactory = func(c cache.Cache, b controllerBuilder) controllerBuilder {
		if c == nil {
			t.Fatalf("expected cache to be passed")
		}
		watcherCalled = true
		return b
	}

	deps := setupDependencies{
		client:            newTestClient(scheme),
		scheme:            scheme,
		recorder:          record.NewFakeRecorder(1),
		indexer:           &fakeFieldIndexer{},
		nodeFeatureSource: &fakeSyncingSource{},
		builder:           builder,
		cache:             &fakeCache{},
	}

	if err := rec.setupWithDependencies(context.Background(), deps); err != nil {
		t.Fatalf("setupWithDependencies returned error: %v", err)
	}
	if !watcherCalled {
		t.Fatalf("expected module watcher factory to be invoked")
	}
}

func TestReconcileDeviceSetsPCIAddress(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-addr",
			UID:  types.UID("uid-addr"),
		},
	}
	rec, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	rec.client = newTestClient(scheme, node)
	rec.scheme = scheme
	rec.recorder = record.NewFakeRecorder(4)

	snapshot := deviceSnapshot{
		Index:      "0",
		Vendor:     "10de",
		Device:     "1db5",
		Class:      "0300",
		PCIAddress: "0000:17:00.0",
		Product:    "NVIDIA-TEST",
		MemoryMiB:  1024,
	}
	device, _, err := rec.reconcileDevice(context.Background(), node, snapshot, map[string]string{}, true, rec.fallbackApproval, nodeTelemetry{}, nodeDetection{})
	if err != nil {
		t.Fatalf("reconcileDevice returned error: %v", err)
	}
	if device.Status.Hardware.PCI.Address != snapshot.PCIAddress {
		t.Fatalf("expected pci address %s, got %s", snapshot.PCIAddress, device.Status.Hardware.PCI.Address)
	}
}

func TestReconcileNodeNotFoundCleansUp(t *testing.T) {
	rec, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	rec.client = newTestClient(newTestScheme(t))
	rec.scheme = newTestScheme(t)
	rec.recorder = record.NewFakeRecorder(16)

	if _, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "absent-node"}}); err != nil {
		t.Fatalf("expected cleanup path to succeed, got %v", err)
	}
}

func TestReconcileGetNodeError(t *testing.T) {
	rec, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	rec.client = &failingGetListClient{getErr: errors.New("get node error")}
	if _, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "node"}}); err == nil {
		t.Fatalf("expected reconcile to return get error")
	}
}

func TestReconcileListError(t *testing.T) {
	rec, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	rec.scheme = newTestScheme(t)
	rec.recorder = record.NewFakeRecorder(16)

	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-listerr"}}
	rec.client = &failingGetListClient{
		Client:  newTestClient(rec.scheme, node),
		listErr: errors.New("list error"),
	}

	if _, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}}); err == nil {
		t.Fatalf("expected reconcile to return list error")
	}
}

func TestReconcileDeletesOrphanDevice(t *testing.T) {
	rec, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	scheme := newTestScheme(t)
	rec.scheme = scheme
	rec.recorder = record.NewFakeRecorder(64)
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-orphan", Labels: map[string]string{"gpu.deckhouse.io/device.00.vendor": "10de", "gpu.deckhouse.io/device.00.device": "1db5", "gpu.deckhouse.io/device.00.class": "0300"}}}
	orphan := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "orphan"},
		Status:     v1alpha1.GPUDeviceStatus{NodeName: "node-orphan"},
	}
	rec.client = newTestClient(scheme, node, orphan)

	if _, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}}); err != nil {
		t.Fatalf("reconcile returned error: %v", err)
	}
	dev := &v1alpha1.GPUDevice{}
	err = rec.client.Get(context.Background(), types.NamespacedName{Name: "orphan"}, dev)
	if err == nil {
		t.Fatalf("expected orphan device to be deleted")
	}
}

func TestReconcileAggregatesRequeue(t *testing.T) {
	scheme := newTestScheme(t)
	handlerResult := contracts.Result{Requeue: true, RequeueAfter: time.Second}
	handler := requeueHandler{result: handlerResult}
	rec, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), []contracts.InventoryHandler{handler})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	rec.scheme = scheme
	rec.recorder = record.NewFakeRecorder(16)
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-requeue", Labels: map[string]string{"gpu.deckhouse.io/device.00.vendor": "10de", "gpu.deckhouse.io/device.00.device": "1db5", "gpu.deckhouse.io/device.00.class": "0300"}}}
	rec.client = newTestClient(scheme, node)

	res, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}})
	if err != nil {
		t.Fatalf("reconcile returned error: %v", err)
	}
	if !res.Requeue || res.RequeueAfter != time.Second {
		t.Fatalf("expected requeue result, got %+v", res)
	}
}

func TestReconcileNodeFeatureError(t *testing.T) {
	rec, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	rec.scheme = newTestScheme(t)
	rec.recorder = record.NewFakeRecorder(1)

	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-nferr"}}
	rec.client = &failingGetListClient{
		Client:         newTestClient(rec.scheme, node),
		nodeFeatureErr: errors.New("nf error"),
	}

	if _, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}}); err == nil {
		t.Fatalf("expected reconcile to return nodefeature error")
	}
}

func TestReconcileMonitoringDisabledSkipsTelemetry(t *testing.T) {
	settings := defaultModuleSettings()
	settings.Monitoring.ServiceMonitor = false
	rec, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(settings), nil)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	rec.scheme = newTestScheme(t)
	rec.recorder = record.NewFakeRecorder(8)
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-no-mon", Labels: map[string]string{"gpu.deckhouse.io/device.00.vendor": "10de", "gpu.deckhouse.io/device.00.device": "1db5", "gpu.deckhouse.io/device.00.class": "0300"}}}
	rec.client = newTestClient(rec.scheme, node)

	if _, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}}); err != nil {
		t.Fatalf("reconcile returned error: %v", err)
	}
}

func TestReconcileNoDevices(t *testing.T) {
	rec, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	rec.scheme = newTestScheme(t)
	rec.recorder = record.NewFakeRecorder(4)
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-empty"}}
	rec.client = newTestClient(rec.scheme, node)

	res, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}})
	if err != nil {
		t.Fatalf("reconcile returned error: %v", err)
	}
	if res.Requeue || res.RequeueAfter != 0 {
		t.Fatalf("unexpected requeue for empty devices: %+v", res)
	}
}

func TestReconcileFindNodeFeatureError(t *testing.T) {
	rec, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	rec.scheme = newTestScheme(t)
	rec.recorder = record.NewFakeRecorder(8)
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "nf-error"}}
	rec.client = &failingGetListClient{
		Client:         newTestClient(rec.scheme, node),
		nodeFeatureErr: errors.New("nf get error"),
	}
	if _, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}}); err == nil {
		t.Fatalf("expected reconcile to fail when NodeFeature lookup fails")
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
			reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(module), nil)
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
			device := &v1alpha1.GPUDevice{}
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
	policy := DeviceApprovalPolicy{mode: moduleconfig.DeviceApprovalModeAutomatic}
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
	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "worker-auto-0-10de-1db5",
			Labels: map[string]string{deviceNodeLabelKey: "worker-auto", deviceIndexLabelKey: "0"},
		},
		Status: v1alpha1.GPUDeviceStatus{},
	}

	module := defaultModuleSettings()
	module.DeviceApproval.Mode = config.DeviceApprovalModeAutomatic

	baseClient := newTestClient(scheme, node, device)
	tracker := &trackingStatusWriter{StatusWriter: baseClient.Status()}
	client := &delegatingClient{Client: baseClient, statusWriter: tracker}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(module), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	snapshot := deviceSnapshot{Index: "0", Vendor: "10de", Device: "1db5", Class: "0302"}
	_, _, err = reconciler.reconcileDevice(context.Background(), node, snapshot, map[string]string{}, true, reconciler.fallbackApproval, nodeTelemetry{}, nodeDetection{})
	if err != nil {
		t.Fatalf("unexpected reconcileDevice error: %v", err)
	}
	if tracker.patches == 0 {
		t.Fatalf("expected status patch to be emitted")
	}
	updated := &v1alpha1.GPUDevice{}
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
	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   deviceName,
			Labels: map[string]string{deviceNodeLabelKey: node.Name, deviceIndexLabelKey: "0"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			NodeName:    node.Name,
			InventoryID: buildInventoryID(node.Name, deviceSnapshot{Index: "0", Vendor: "10de", Device: "1db5", Class: "0302"}),
			Managed:     true,
			Hardware: v1alpha1.GPUDeviceHardware{
				Product:   "NVIDIA TEST",
				PCI:       v1alpha1.PCIAddress{Vendor: "10de", Device: "1db5", Class: "0302"},
				MemoryMiB: 16384,
				ComputeCapability: &v1alpha1.GPUComputeCapability{
					Major: 8,
					Minor: 6,
				},
				Precision: v1alpha1.GPUPrecision{
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

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
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

	_, result, err := reconciler.reconcileDevice(context.Background(), node, snapshot, map[string]string{}, true, reconciler.fallbackApproval, nodeTelemetry{}, nodeDetection{})
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
			if _, ok := list.(*v1alpha1.GPUDeviceList); ok {
				return cleanupErr
			}
			return base.List(ctx, list, opts...)
		},
	}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
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

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)
	reconciler.setResyncPeriod(0)

	res, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}})
	if err != nil {
		t.Fatalf("unexpected reconcile error: %v", err)
	}
	if res.Requeue || res.RequeueAfter != 0 {
		t.Fatalf("expected no requeue and no resync, got %+v", res)
	}
}

func TestDeleteInventoryRemovesResource(t *testing.T) {
	scheme := newTestScheme(t)
	inventory := &v1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{Name: "node-cleanup"},
	}
	fixtureClient := newTestClient(scheme, inventory)

	reconciler := &Reconciler{client: fixtureClient}

	if err := reconciler.deleteInventory(context.Background(), "node-cleanup"); err != nil {
		t.Fatalf("deleteInventory returned error: %v", err)
	}

	err := fixtureClient.Get(context.Background(), types.NamespacedName{Name: "node-cleanup"}, &v1alpha1.GPUNodeInventory{})
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected inventory to be deleted, got err=%v", err)
	}

	if err := reconciler.deleteInventory(context.Background(), "missing-node"); err != nil {
		t.Fatalf("deleteInventory should ignore missing objects, got %v", err)
	}

	baseClient := newTestClient(scheme, &v1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{Name: "node-delete-race"},
	})
	delClient := &delegatingClient{
		Client: baseClient,
		delete: func(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
			return apierrors.NewNotFound(schema.GroupResource{Group: v1alpha1.GroupVersion.Group, Resource: "gpunodeinventories"}, obj.GetName())
		},
	}
	reconciler.client = delClient

	if err := reconciler.deleteInventory(context.Background(), "node-delete-race"); err != nil {
		t.Fatalf("deleteInventory should ignore not found error from delete, got %v", err)
	}
}

func TestDeleteInventoryPropagatesGetError(t *testing.T) {
	scheme := newTestScheme(t)
	base := newTestClient(scheme)
	boom := errors.New("get failure")

	client := &delegatingClient{
		Client: base,
		get: func(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
			return boom
		},
	}

	reconciler := &Reconciler{client: client}

	if err := reconciler.deleteInventory(context.Background(), "node-error"); !errors.Is(err, boom) {
		t.Fatalf("expected error %v, got %v", boom, err)
	}
}
func TestApplyInventoryResyncUpdatesPeriod(t *testing.T) {
	reconciler := &Reconciler{}
	reconciler.setResyncPeriod(2 * time.Minute)

	state := moduleconfig.DefaultState()
	state.Inventory.ResyncPeriod = "15s"

	reconciler.applyInventoryResync(state)

	if got := reconciler.getResyncPeriod(); got != 15*time.Second {
		t.Fatalf("expected resync period 15s, got %s", got)
	}
}

func TestApplyInventoryResyncIgnoresInvalidValue(t *testing.T) {
	reconciler := &Reconciler{}
	reconciler.setResyncPeriod(45 * time.Second)

	state := moduleconfig.DefaultState()
	state.Inventory.ResyncPeriod = "not-a-duration"

	reconciler.applyInventoryResync(state)

	if got := reconciler.getResyncPeriod(); got != 45*time.Second {
		t.Fatalf("resync period should remain unchanged, got %s", got)
	}
}

func TestApplyInventoryResyncIgnoresEmptyValue(t *testing.T) {
	reconciler := &Reconciler{}
	reconciler.setResyncPeriod(time.Minute)

	state := moduleconfig.DefaultState()
	state.Inventory.ResyncPeriod = ""

	reconciler.applyInventoryResync(state)

	if got := reconciler.getResyncPeriod(); got != time.Minute {
		t.Fatalf("resync period should remain unchanged when value empty, got %s", got)
	}
}

func TestRefreshInventorySettingsReadsStore(t *testing.T) {
	state := moduleconfig.DefaultState()
	state.Inventory.ResyncPeriod = "90s"
	store := config.NewModuleConfigStore(state)

	reconciler := &Reconciler{store: store}
	reconciler.setResyncPeriod(30 * time.Second)

	reconciler.refreshInventorySettings()

	if got := reconciler.getResyncPeriod(); got != 90*time.Second {
		t.Fatalf("expected resync period from store (90s), got %s", got)
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
	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-refetch-0-10de-1db5",
		},
	}

	base := newTestClient(scheme, node, device)
	var getCalls int
	client := &delegatingClient{
		Client: base,
		get: func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if _, ok := obj.(*v1alpha1.GPUDevice); ok {
				getCalls++
				if getCalls > 1 {
					return errors.New("device refetch failure")
				}
			}
			return base.Get(ctx, key, obj, opts...)
		},
		statusWriter: base.Status(),
	}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
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
	}, map[string]string{}, true, reconciler.fallbackApproval, nodeTelemetry{}, nodeDetection{})
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
	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "worker-patch-error-0-10de-1db5",
			Labels: map[string]string{deviceNodeLabelKey: "old-node"},
		},
		Status: v1alpha1.GPUDeviceStatus{NodeName: "old-node"},
	}

	base := newTestClient(scheme, node, device)
	client := &delegatingClient{
		Client:       base,
		statusWriter: &errorStatusWriter{StatusWriter: base.Status(), err: errors.New("status patch failure")},
	}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
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
	}, map[string]string{}, true, reconciler.fallbackApproval, nodeTelemetry{}, nodeDetection{})
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

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
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
	}, map[string]string{}, true, reconciler.fallbackApproval, nodeTelemetry{}, nodeDetection{})
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
	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-update-fields-0-10de-2230",
		},
		Status: v1alpha1.GPUDeviceStatus{
			NodeName:   "worker-update-fields",
			Managed:    true,
			AutoAttach: false,
			Hardware: v1alpha1.GPUDeviceHardware{
				Product: "Old Product",
				Precision: v1alpha1.GPUPrecision{
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

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(module), nil)
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
		MIG:          v1alpha1.GPUMIGConfig{Capable: true, Strategy: v1alpha1.GPUMIGStrategyMixed},
		UUID:         "GPU-NEW-UUID",
		ComputeMajor: 8,
		ComputeMinor: 0,
	}

	updated, _, err := reconciler.reconcileDevice(context.Background(), node, snapshot, map[string]string{}, true, reconciler.fallbackApproval, nodeTelemetry{}, nodeDetection{})
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

	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-handler-error-0-10de-1db5"},
		Status:     v1alpha1.GPUDeviceStatus{NodeName: "worker-handler-error"},
	}
	if err := controllerutil.SetOwnerReference(node, device, scheme); err != nil {
		t.Fatalf("set owner reference: %v", err)
	}
	base := newTestClient(scheme, node, device)
	errHandler := errorHandler{err: errors.New("handler failure")}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
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
	}, map[string]string{}, true, reconciler.fallbackApproval, nodeTelemetry{}, nodeDetection{})
	if err == nil || !strings.Contains(err.Error(), "handler failure") {
		t.Fatalf("expected handler failure, got %v", err)
	}
}

func TestEnsureDeviceMetadataOwnerReferenceError(t *testing.T) {
	reconciler := &Reconciler{
		scheme: runtime.NewScheme(),
	}
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-meta-error"}}
	device := &v1alpha1.GPUDevice{}

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

	devices := []*v1alpha1.GPUDevice{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "worker-inv-create-dev0",
				Labels: map[string]string{deviceIndexLabelKey: "0"},
			},
			Status: v1alpha1.GPUDeviceStatus{InventoryID: "worker-inv-create-0"},
		},
	}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	err = reconciler.reconcileNodeInventory(context.Background(), node, nodeSnapshot{
		Devices: []deviceSnapshot{{Index: "0", Vendor: "10de", Device: "1db5", Class: "0302"}},
	}, devices, reconciler.fallbackManaged, nodeDetection{})
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
			if _, ok := obj.(*v1alpha1.GPUNodeInventory); ok {
				return getErr
			}
			return base.Get(ctx, key, obj, opts...)
		},
	}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	if err := reconciler.reconcileNodeInventory(context.Background(), node, nodeSnapshot{}, nil, reconciler.fallbackManaged, nodeDetection{}); !errors.Is(err, getErr) {
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

	devices := []*v1alpha1.GPUDevice{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "worker-inv-create-fail-dev0",
				Labels: map[string]string{deviceIndexLabelKey: "0"},
			},
			Status: v1alpha1.GPUDeviceStatus{InventoryID: "worker-inv-create-fail-0"},
		},
	}

	base := newTestClient(scheme, node, devices[0])
	createErr := errors.New("inventory create failure")
	client := &delegatingClient{
		Client: base,
		create: func(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
			if _, ok := obj.(*v1alpha1.GPUNodeInventory); ok {
				return createErr
			}
			return base.Create(ctx, obj, opts...)
		},
	}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	err = reconciler.reconcileNodeInventory(context.Background(), node, nodeSnapshot{
		Devices: []deviceSnapshot{{Index: "0", Vendor: "10de", Device: "1db5", Class: "0302"}},
	}, devices, reconciler.fallbackManaged, nodeDetection{})
	if !errors.Is(err, createErr) {
		t.Fatalf("expected create error, got %v", err)
	}
}

func TestReconcileNodeInventoryOwnerReferenceUpdateError(t *testing.T) {
	scheme := runtime.NewScheme()
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-inv-update"}}
	inventory := &v1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{Name: node.Name},
		Spec:       v1alpha1.GPUNodeInventorySpec{NodeName: node.Name},
	}
	client := newTestClient(newTestScheme(t), node, inventory)

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	err = reconciler.reconcileNodeInventory(context.Background(), node, nodeSnapshot{}, nil, reconciler.fallbackManaged, nodeDetection{})
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
	inventory := &v1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{Name: node.Name},
		Spec:       v1alpha1.GPUNodeInventorySpec{NodeName: "old-node"},
	}

	base := newTestClient(scheme, node, inventory)
	patchErr := errors.New("inventory patch failure")
	client := &delegatingClient{
		Client: base,
		patch: func(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
			if _, ok := obj.(*v1alpha1.GPUNodeInventory); ok {
				return patchErr
			}
			return base.Patch(ctx, obj, patch, opts...)
		},
	}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	err = reconciler.reconcileNodeInventory(context.Background(), node, nodeSnapshot{}, nil, reconciler.fallbackManaged, nodeDetection{})
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
	inventory := &v1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{Name: node.Name},
		Spec:       v1alpha1.GPUNodeInventorySpec{NodeName: "old"},
	}

	base := newTestClient(scheme, node, inventory)
	var getCalls int
	client := &delegatingClient{
		Client: base,
		patch: func(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
			return base.Patch(ctx, obj, patch, opts...)
		},
		get: func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if _, ok := obj.(*v1alpha1.GPUNodeInventory); ok {
				getCalls++
				if getCalls > 1 {
					return errors.New("inventory refetch failure")
				}
			}
			return base.Get(ctx, key, obj, opts...)
		},
	}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
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
	}, []*v1alpha1.GPUDevice{{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "worker-inv-refetch-device",
			Labels: map[string]string{deviceIndexLabelKey: "0"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "worker-inv-refetch-0-10de-1db5",
			Hardware: v1alpha1.GPUDeviceHardware{
				Product:   "GPU",
				MemoryMiB: 2048,
				MIG:       v1alpha1.GPUMIGConfig{},
			},
		},
	}}, reconciler.fallbackManaged, nodeDetection{})
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
	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "worker-inv-precision-device",
			Labels: map[string]string{deviceIndexLabelKey: "0"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "worker-inv-precision-0-10de-1db5",
		},
	}

	client := newTestClient(scheme, node, device)

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
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

	if err := reconciler.reconcileNodeInventory(context.Background(), node, snapshot, []*v1alpha1.GPUDevice{device}, reconciler.fallbackManaged, nodeDetection{}); err != nil {
		t.Fatalf("unexpected reconcile error: %v", err)
	}

	inventory := &v1alpha1.GPUNodeInventory{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: node.Name}, inventory); err != nil {
		t.Fatalf("get inventory: %v", err)
	}
	if len(inventory.Status.Hardware.Devices) != 1 {
		t.Fatalf("expected 1 device, got %+v", inventory.Status.Hardware.Devices)
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
	devices := []*v1alpha1.GPUDevice{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "worker-inv-sort-dev1",
				Labels: map[string]string{deviceIndexLabelKey: "1"},
			},
			Status: v1alpha1.GPUDeviceStatus{
				InventoryID: "worker-inv-sort-1",
				Hardware:    v1alpha1.GPUDeviceHardware{},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "worker-inv-sort-dev0",
				Labels: map[string]string{deviceIndexLabelKey: "0"},
			},
			Status: v1alpha1.GPUDeviceStatus{
				InventoryID: "worker-inv-sort-0",
				Hardware:    v1alpha1.GPUDeviceHardware{},
			},
		},
	}

	client := newTestClient(scheme, node, devices[0], devices[1])

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
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
	}, devices, reconciler.fallbackManaged, nodeDetection{}); err != nil {
		t.Fatalf("unexpected reconcile error: %v", err)
	}

	inventory := &v1alpha1.GPUNodeInventory{}
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
	inventory := &v1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{
			Name: node.Name,
			OwnerReferences: []metav1.OwnerReference{
				{APIVersion: "v1", Kind: "Node", Name: node.Name, UID: node.UID},
			},
		},
		Spec: v1alpha1.GPUNodeInventorySpec{NodeName: node.Name},
	}
	client := newTestClient(scheme, node, inventory)

	module := defaultModuleSettings()
	module.ManagedNodes.LabelKey = ""

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(module), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	if err := reconciler.reconcileNodeInventory(context.Background(), node, nodeSnapshot{Managed: false}, nil, ManagedNodesPolicy{}, nodeDetection{}); err != nil {
		t.Fatalf("unexpected reconcile error: %v", err)
	}
	updatedInventory := &v1alpha1.GPUNodeInventory{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: node.Name}, updatedInventory); err != nil {
		t.Fatalf("get inventory: %v", err)
	}
	cond := getCondition(updatedInventory.Status.Conditions, conditionManagedDisabled)
	if cond == nil || cond.Message != fmt.Sprintf("node is marked with %s=false", defaultManagedNodeLabelKey) {
		t.Fatalf("expected default managed label usage, got %+v", cond)
	}
}

func TestMapNodeFeatureToNode(t *testing.T) {
	feature := &nfdv1alpha1.NodeFeature{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-d"},
		Spec: nfdv1alpha1.NodeFeatureSpec{
			Labels: map[string]string{
				gfdProductLabel: "NVIDIA A100",
			},
		},
	}
	reqs := mapNodeFeatureToNode(context.Background(), feature)
	if len(reqs) != 1 || reqs[0].Name != "worker-d" {
		t.Fatalf("unexpected requests: %+v", reqs)
	}

	noName := &nfdv1alpha1.NodeFeature{}
	if reqs := mapNodeFeatureToNode(context.Background(), noName); len(reqs) != 0 {
		t.Fatalf("expected empty requests, got %+v", reqs)
	}

	noGPU := &nfdv1alpha1.NodeFeature{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-e"},
	}
	if reqs := mapNodeFeatureToNode(context.Background(), noGPU); len(reqs) != 0 {
		t.Fatalf("expected empty requests for feature without GPU labels, got %+v", reqs)
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
	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "worker-delete-0",
			Labels: map[string]string{deviceNodeLabelKey: "worker-delete", deviceIndexLabelKey: "0"},
		},
		Status: v1alpha1.GPUDeviceStatus{NodeName: "worker-delete"},
	}
	base := newTestClient(scheme, device)
	deleteErr := errors.New("device delete failure")

	client := &delegatingClient{
		Client: base,
		delete: func(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
			switch obj.(type) {
			case *v1alpha1.GPUDevice:
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
	inventory := &v1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-inventory"},
		Spec:       v1alpha1.GPUNodeInventorySpec{NodeName: "worker-inventory"},
	}
	base := newTestClient(scheme, inventory)
	deleteErr := errors.New("inventory delete failure")

	client := &delegatingClient{
		Client: base,
		delete: func(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
			switch obj.(type) {
			case *v1alpha1.GPUNodeInventory:
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
		WithStatusSubresource(&v1alpha1.GPUDevice{}, &v1alpha1.GPUNodeInventory{}).
		WithObjects(objs...)

	builder = builder.WithIndex(&v1alpha1.GPUDevice{}, deviceNodeIndexKey, func(obj client.Object) []string {
		device, ok := obj.(*v1alpha1.GPUDevice)
		if !ok || device.Status.NodeName == "" {
			return nil
		}
		return []string{device.Status.NodeName}
	})

	return builder.Build()
}
