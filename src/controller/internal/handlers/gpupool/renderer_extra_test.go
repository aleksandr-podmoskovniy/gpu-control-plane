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

package gpupool

import (
	"context"
	"strings"
	"testing"

	"github.com/go-logr/logr/testr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/yaml"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
)

func TestRendererNameAndMissingClient(t *testing.T) {
	h := &RendererHandler{cfg: RenderConfig{Namespace: "ns", DevicePluginImage: "img"}}
	if h.Name() != "renderer" {
		t.Fatalf("unexpected name %s", h.Name())
	}
	if _, err := h.HandlePool(context.Background(), &v1alpha1.GPUPool{}); err == nil {
		t.Fatalf("expected error when client is nil")
	}
}

func TestHandlePoolValidations(t *testing.T) {
	log := testr.New(t)
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	// namespace missing
	h := &RendererHandler{log: log, client: fake.NewClientBuilder().WithScheme(scheme).Build(), cfg: RenderConfig{DevicePluginImage: "img"}}
	if _, err := h.HandlePool(context.Background(), &v1alpha1.GPUPool{}); err == nil {
		t.Fatalf("expected namespace validation error")
	}

	// device-plugin image missing
	h = &RendererHandler{log: log, client: fake.NewClientBuilder().WithScheme(scheme).Build(), cfg: RenderConfig{Namespace: "ns"}}
	if _, err := h.HandlePool(context.Background(), &v1alpha1.GPUPool{}); err == nil {
		t.Fatalf("expected device-plugin image validation error")
	}

	// unsupported provider should no-op
	h = NewRendererHandler(log, fake.NewClientBuilder().WithScheme(scheme).Build(), RenderConfig{Namespace: "ns", DevicePluginImage: "img"})
	pool := &v1alpha1.GPUPool{Spec: v1alpha1.GPUPoolSpec{Provider: "Other"}}
	if _, err := h.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("expected no error for other provider: %v", err)
	}

	// backend cleanup path
	ds := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "nvidia-device-plugin-pool", Namespace: "ns"}}
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "nvidia-device-plugin-pool-config", Namespace: "ns"}}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ds, cm).Build()
	h = NewRendererHandler(log, cl, RenderConfig{Namespace: "ns", DevicePluginImage: "img"})
	pool = &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}, Spec: v1alpha1.GPUPoolSpec{Backend: "DRA"}}
	if _, err := h.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("cleanup path failed: %v", err)
	}
	if err := cl.Get(context.Background(), client.ObjectKeyFromObject(ds), &appsv1.DaemonSet{}); !apierrors.IsNotFound(err) {
		t.Fatalf("expected ds deleted, got %v", err)
	}
}

func TestHandlePoolMIGSkipWhenNoImage(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	h := NewRendererHandler(testr.New(t), cl, RenderConfig{Namespace: "ns", DevicePluginImage: "img"})

	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool"},
		Spec: v1alpha1.GPUPoolSpec{
			Resource: v1alpha1.GPUPoolResourceSpec{Unit: "MIG", MIGProfile: "1g.10gb"},
		},
	}
	if _, err := h.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("HandlePool MIG without image: %v", err)
	}
	// device plugin rendered
	if err := cl.Get(context.Background(), client.ObjectKey{Namespace: "ns", Name: "nvidia-device-plugin-pool"}, &appsv1.DaemonSet{}); err != nil {
		t.Fatalf("expected device-plugin ds: %v", err)
	}
	// mig manager skipped
	if err := cl.Get(context.Background(), client.ObjectKey{Namespace: "ns", Name: "nvidia-mig-manager-pool"}, &appsv1.DaemonSet{}); !apierrors.IsNotFound(err) {
		t.Fatalf("expected mig manager to be skipped")
	}
}

func TestHandlePoolFullMIGRendering(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	h := NewRendererHandler(testr.New(t), cl, RenderConfig{
		Namespace:          "ns",
		DevicePluginImage:  "dp:1",
		MIGManagerImage:    "mig:1",
		DefaultMIGStrategy: "single",
	})

	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool"},
		Spec: v1alpha1.GPUPoolSpec{
			Resource: v1alpha1.GPUPoolResourceSpec{
				Unit:          "MIG",
				MIGProfile:    "3g.40gb",
				SlicesPerUnit: 2,
				MIGLayout: []v1alpha1.GPUPoolMIGDeviceLayout{
					{
						UUIDs:         []string{"uuid-1"},
						Profiles:      []v1alpha1.GPUPoolMIGProfile{{Name: "3g.40gb", Count: ptrTo[int32](1)}},
						SlicesPerUnit: ptrTo[int32](4),
					},
					{
						PCIBusIDs: []string{"0000:01:00.0"},
						Profiles:  []v1alpha1.GPUPoolMIGProfile{{Name: "1g.10gb"}},
					},
					{
						DeviceFilter: []string{"0x1234"},
						Profiles:     []v1alpha1.GPUPoolMIGProfile{{Name: "2g.20gb", SlicesPerUnit: ptrTo[int32](3)}},
					},
					{
						Profiles: []v1alpha1.GPUPoolMIGProfile{{Name: "1g.10gb", Count: ptrTo[int32](2)}},
					},
				},
			},
		},
	}

	if _, err := h.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("HandlePool MIG: %v", err)
	}

	// Ensure all objects exist
	for _, name := range []string{
		"nvidia-device-plugin-pool",
		"nvidia-mig-manager-pool",
	} {
		if err := cl.Get(context.Background(), client.ObjectKey{Namespace: "ns", Name: name}, &appsv1.DaemonSet{}); err != nil {
			t.Fatalf("%s missing: %v", name, err)
		}
	}
	for _, name := range []string{
		"nvidia-device-plugin-pool-config",
		"nvidia-mig-manager-pool-config",
		"nvidia-mig-manager-pool-scripts",
		"nvidia-mig-manager-pool-gpu-clients",
	} {
		if err := cl.Get(context.Background(), client.ObjectKey{Namespace: "ns", Name: name}, &corev1.ConfigMap{}); err != nil {
			t.Fatalf("%s missing: %v", name, err)
		}
	}

	// device-plugin config should pick replicas from last slices per unit override (3)
	cm := &corev1.ConfigMap{}
	_ = cl.Get(context.Background(), client.ObjectKey{Namespace: "ns", Name: "nvidia-device-plugin-pool-config"}, cm)
	if !strings.Contains(cm.Data["config.yaml"], "replicas: 3") {
		t.Fatalf("expected replicas override in config, got:\n%s", cm.Data["config.yaml"])
	}
}

func TestDevicePluginConfigMapTimeSlicingOverrides(t *testing.T) {
	h := NewRendererHandler(testr.New(t), nil, RenderConfig{Namespace: "ns"})
	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool"},
		Spec: v1alpha1.GPUPoolSpec{
			Resource: v1alpha1.GPUPoolResourceSpec{
				Unit:          "Card",
				SlicesPerUnit: 1,
				TimeSlicingResources: []v1alpha1.GPUPoolTimeSlicingResource{
					{Name: "", SlicesPerUnit: 5},
					{Name: "gpu.deckhouse.io/custom", SlicesPerUnit: 2},
				},
			},
		},
	}
	cm := h.devicePluginConfigMap(pool)
	var cfg struct {
		Shared struct {
			TimeSlicing struct {
				Resources []struct {
					Name     string `json:"name"`
					Replicas int32  `json:"replicas"`
				} `json:"resources"`
			} `json:"timeSlicing"`
		} `json:"shared"`
	}
	if err := yaml.Unmarshal([]byte(cm.Data["config.yaml"]), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(cfg.Shared.TimeSlicing.Resources) != 2 {
		t.Fatalf("expected two time-slicing resources, got %d", len(cfg.Shared.TimeSlicing.Resources))
	}
	if cfg.Shared.TimeSlicing.Resources[0].Name != "gpu.deckhouse.io/pool" || cfg.Shared.TimeSlicing.Resources[0].Replicas != 5 {
		t.Fatalf("default resource override not applied: %+v", cfg.Shared.TimeSlicing.Resources[0])
	}
	if cfg.Shared.TimeSlicing.Resources[1].Name != "gpu.deckhouse.io/custom" || cfg.Shared.TimeSlicing.Resources[1].Replicas != 2 {
		t.Fatalf("custom resource override not applied: %+v", cfg.Shared.TimeSlicing.Resources[1])
	}
}

func TestDevicePluginConfigMapIgnoresInvalidTimeSlicing(t *testing.T) {
	h := NewRendererHandler(testr.New(t), nil, RenderConfig{Namespace: "ns"})
	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool"},
		Spec: v1alpha1.GPUPoolSpec{
			Resource: v1alpha1.GPUPoolResourceSpec{
				Unit:          "Card",
				SlicesPerUnit: 4,
				TimeSlicingResources: []v1alpha1.GPUPoolTimeSlicingResource{
					{Name: "invalid", SlicesPerUnit: 0},
				},
			},
		},
	}
	cm := h.devicePluginConfigMap(pool)
	if !strings.Contains(cm.Data["config.yaml"], "replicas: 4") {
		t.Fatalf("expected fallback to default replicas when timeSlicingResources invalid, got:\n%s", cm.Data["config.yaml"])
	}
}

func TestReconcileFailures(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	errClient := &failingCreateClient{Client: cl, err: apierrors.NewConflict(v1alpha1.GroupVersion.WithResource("configmaps").GroupResource(), "cm", nil)}
	h := NewRendererHandler(testr.New(t), errClient, RenderConfig{Namespace: "ns", DevicePluginImage: "img", MIGManagerImage: "mig"})

	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}, Spec: v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"}}}
	if err := h.reconcileDevicePlugin(context.Background(), pool); err == nil {
		t.Fatalf("expected error from reconcileDevicePlugin")
	}
	if err := h.reconcileMIGManager(context.Background(), &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}, Spec: v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "MIG", MIGProfile: "1g.10gb"}}}); err == nil {
		t.Fatalf("expected error from reconcileMIGManager")
	}
}

func TestMIGManagerConfigDefaults(t *testing.T) {
	h := &RendererHandler{cfg: RenderConfig{Namespace: "ns"}}
	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool"},
		Spec:       v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "MIG", MIGProfile: "1g.10gb"}},
	}
	cm := h.migManagerConfigMap(pool)
	if cm.Data["config.yaml"] == "" {
		t.Fatalf("expected config generated")
	}
}

func TestCleanupErrorsPropagation(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	errs := map[string]error{
		"nvidia-device-plugin-pool-config": apierrors.NewBadRequest("boom"),
	}
	h := NewRendererHandler(testr.New(t), &errorDeleteClient{Client: cl, errs: errs}, RenderConfig{Namespace: "ns"})
	if err := h.cleanupPoolResources(context.Background(), "pool"); err == nil {
		t.Fatalf("expected error from cleanupPoolResources")
	}
	errs = map[string]error{
		"nvidia-mig-manager-pool-config": apierrors.NewBadRequest("boom"),
	}
	h = NewRendererHandler(testr.New(t), &errorDeleteClient{Client: cl, errs: errs}, RenderConfig{Namespace: "ns"})
	if err := h.cleanupMIGResources(context.Background(), "pool"); err == nil {
		t.Fatalf("expected error from cleanupMIGResources")
	}
}

func TestHandlePoolReconcileErrors(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	base := fake.NewClientBuilder().WithScheme(scheme).Build()
	// device-plugin error
	dpErrClient := &failingCreateClient{
		Client: base,
		errsByName: map[string]error{
			"nvidia-device-plugin-pool-config": apierrors.NewBadRequest("fail"),
		},
	}
	h := NewRendererHandler(testr.New(t), dpErrClient, RenderConfig{Namespace: "ns", DevicePluginImage: "img"})
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}, Spec: v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"}}}
	if _, err := h.HandlePool(context.Background(), pool); err == nil {
		t.Fatalf("expected device-plugin reconcile error")
	}

	// MIG manager error
	migErrClient := &failingCreateClient{
		Client: base,
		errsByName: map[string]error{
			"nvidia-mig-manager-pool-config": apierrors.NewBadRequest("fail"),
		},
	}
	h = NewRendererHandler(testr.New(t), migErrClient, RenderConfig{Namespace: "ns", DevicePluginImage: "img", MIGManagerImage: "mig"})
	pool.Spec.Resource.Unit = "MIG"
	pool.Spec.Resource.MIGProfile = "1g.10gb"
	if _, err := h.HandlePool(context.Background(), pool); err == nil {
		t.Fatalf("expected mig manager reconcile error")
	}

	// cleanup path errors when backend changed
	errDeleteClient := &errorDeleteClient{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		errs: map[string]error{
			"nvidia-device-plugin-pool": apierrors.NewBadRequest("delete fail"),
		},
	}
	h = NewRendererHandler(testr.New(t), errDeleteClient, RenderConfig{Namespace: "ns", DevicePluginImage: "img"})
	pool.Spec.Backend = "DRA"
	if _, err := h.HandlePool(context.Background(), pool); err == nil {
		t.Fatalf("expected cleanup error in HandlePool")
	}

	// cleanupMIGResources error when unit is Card
	errDeleteClient = &errorDeleteClient{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		errs: map[string]error{
			"nvidia-mig-manager-pool": apierrors.NewBadRequest("fail"),
		},
	}
	h = NewRendererHandler(testr.New(t), errDeleteClient, RenderConfig{Namespace: "ns", DevicePluginImage: "img"})
	pool.Spec.Backend = ""
	pool.Spec.Resource.Unit = "Card"
	if _, err := h.HandlePool(context.Background(), pool); err == nil {
		t.Fatalf("expected cleanupMIGResources error")
	}
}

func TestHandlePoolRemovesMIGForCardPools(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	// pre-create MIG resources to ensure cleanup path hit
	migDS := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "nvidia-mig-manager-pool", Namespace: "ns"}}
	migCM := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "nvidia-mig-manager-pool-config", Namespace: "ns"}}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(migDS, migCM).Build()
	h := NewRendererHandler(testr.New(t), cl, RenderConfig{Namespace: "ns", DevicePluginImage: "dp:1"})
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}, Spec: v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"}}}
	if _, err := h.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("HandlePool: %v", err)
	}
	if err := cl.Get(context.Background(), client.ObjectKeyFromObject(migDS), &appsv1.DaemonSet{}); !apierrors.IsNotFound(err) {
		t.Fatalf("expected mig ds removed, got %v", err)
	}
}

func TestReconcileDevicePluginDaemonSetError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	base := fake.NewClientBuilder().WithScheme(scheme).Build()
	cl := &failingCreateClient{
		Client: base,
		err:    apierrors.NewConflict(v1alpha1.GroupVersion.WithResource("daemonsets").GroupResource(), "ds", nil),
	}
	// allow ConfigMap create by pre-populating error only for daemonset create/update
	cl.allowNames = map[string]bool{"nvidia-device-plugin-pool-config": true}
	h := NewRendererHandler(testr.New(t), cl, RenderConfig{Namespace: "ns", DevicePluginImage: "img"})
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}, Spec: v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"}}}
	if err := h.reconcileDevicePlugin(context.Background(), pool); err == nil {
		t.Fatalf("expected daemonset error")
	}
}

func TestReconcileMIGManagerErrorLaterStage(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	base := fake.NewClientBuilder().WithScheme(scheme).Build()
	cl := &failingCreateClient{
		Client: base,
		errsByName: map[string]error{
			"nvidia-mig-manager-pool-gpu-clients": apierrors.NewBadRequest("fail"),
		},
	}
	h := NewRendererHandler(testr.New(t), cl, RenderConfig{Namespace: "ns", DevicePluginImage: "img", MIGManagerImage: "mig"})
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}, Spec: v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "MIG", MIGProfile: "1g.10gb"}}}
	if err := h.reconcileMIGManager(context.Background(), pool); err == nil {
		t.Fatalf("expected error from clients config")
	}
}

func TestReconcileMIGManagerErrorsAtConfigAndScripts(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	base := fake.NewClientBuilder().WithScheme(scheme).Build()

	// config error
	clientConfigErr := &failingCreateClient{
		Client: base,
		errsByName: map[string]error{
			"nvidia-mig-manager-pool-config": apierrors.NewConflict(v1alpha1.GroupVersion.WithResource("configmaps").GroupResource(), "cm", nil),
		},
	}
	h := NewRendererHandler(testr.New(t), clientConfigErr, RenderConfig{Namespace: "ns", DevicePluginImage: "img", MIGManagerImage: "mig"})
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}, Spec: v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "MIG", MIGProfile: "1g.10gb"}}}
	if err := h.reconcileMIGManager(context.Background(), pool); err == nil {
		t.Fatalf("expected config error")
	}

	// scripts error
	clientScriptsErr := &failingCreateClient{
		Client: base,
		errsByName: map[string]error{
			"nvidia-mig-manager-pool-scripts": apierrors.NewConflict(v1alpha1.GroupVersion.WithResource("configmaps").GroupResource(), "cm", nil),
		},
	}
	h = NewRendererHandler(testr.New(t), clientScriptsErr, RenderConfig{Namespace: "ns", DevicePluginImage: "img", MIGManagerImage: "mig"})
	if err := h.reconcileMIGManager(context.Background(), pool); err == nil {
		t.Fatalf("expected scripts error")
	}

	// daemonset error
	clientDSErr := &failingCreateClient{
		Client: base,
		errsByName: map[string]error{
			"nvidia-mig-manager-pool": apierrors.NewConflict(v1alpha1.GroupVersion.WithResource("daemonsets").GroupResource(), "ds", nil),
		},
	}
	h = NewRendererHandler(testr.New(t), clientDSErr, RenderConfig{Namespace: "ns", DevicePluginImage: "img", MIGManagerImage: "mig"})
	if err := h.reconcileMIGManager(context.Background(), pool); err == nil {
		t.Fatalf("expected daemonset error")
	}
}

func TestBuildMIGDevicesEmptyLayoutsSkipped(t *testing.T) {
	h := NewRendererHandler(testr.New(t), nil, RenderConfig{})
	pool := &v1alpha1.GPUPool{
		Spec: v1alpha1.GPUPoolSpec{
			Resource: v1alpha1.GPUPoolResourceSpec{
				Unit:       "MIG",
				MIGProfile: "",
				MIGLayout: []v1alpha1.GPUPoolMIGDeviceLayout{
					{}, // should be skipped
					{UUIDs: []string{"uuid-1"}, Profiles: []v1alpha1.GPUPoolMIGProfile{{Name: "1g.10gb"}}},
				},
			},
		},
	}
	devices := h.buildMIGDevices(pool)
	if len(devices) != 1 {
		t.Fatalf("expected one device layout, got %d", len(devices))
	}

	// ensure MIGProfile fallback used when layout profiles empty
	pool.Spec.Resource.MIGProfile = "2g.20gb"
	pool.Spec.Resource.MIGLayout = []v1alpha1.GPUPoolMIGDeviceLayout{{UUIDs: []string{"uuid-2"}}}
	devices = h.buildMIGDevices(pool)
	if len(devices) != 1 || devices[0]["migDevices"].([]map[string]any)[0]["profile"] != "2g.20gb" {
		t.Fatalf("expected fallback MIG profile applied, got %+v", devices)
	}
}

func TestAddOwnerIdempotent(t *testing.T) {
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool", UID: "uid"}, Spec: v1alpha1.GPUPoolSpec{}}
	cm := &corev1.ConfigMap{}
	addOwner(cm, pool)
	// second call should not duplicate
	addOwner(cm, pool)
	if len(cm.OwnerReferences) != 1 {
		t.Fatalf("expected single owner ref, got %d", len(cm.OwnerReferences))
	}
}

type failingCreateClient struct {
	client.Client
	err        error
	allowNames map[string]bool
	errsByName map[string]error
}

func (f *failingCreateClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if f.errsByName != nil {
		if err, ok := f.errsByName[obj.GetName()]; ok {
			return err
		}
	}
	if f.allowNames != nil && f.allowNames[obj.GetName()] {
		return f.Client.Create(ctx, obj, opts...)
	}
	return f.err
}

func (f *failingCreateClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	if f.errsByName != nil {
		if err, ok := f.errsByName[obj.GetName()]; ok {
			return err
		}
	}
	if f.allowNames != nil && f.allowNames[obj.GetName()] {
		return f.Client.Update(ctx, obj, opts...)
	}
	return f.err
}
