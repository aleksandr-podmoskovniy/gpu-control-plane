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
	"fmt"
	"strings"
	"testing"

	"github.com/go-logr/logr/testr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/yaml"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
)

func TestRendererNameAndMissingClient(t *testing.T) {
	h := &RendererHandler{cfg: RenderConfig{Namespace: "ns", DevicePluginImage: "img", ValidatorImage: "val"}}
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
	h := &RendererHandler{log: log, client: fake.NewClientBuilder().WithScheme(scheme).Build(), cfg: RenderConfig{DevicePluginImage: "img", ValidatorImage: "val"}}
	if _, err := h.HandlePool(context.Background(), &v1alpha1.GPUPool{}); err == nil {
		t.Fatalf("expected namespace validation error")
	}

	// device-plugin image missing
	h = &RendererHandler{log: log, client: fake.NewClientBuilder().WithScheme(scheme).Build(), cfg: RenderConfig{Namespace: "ns"}}
	if _, err := h.HandlePool(context.Background(), &v1alpha1.GPUPool{}); err == nil {
		t.Fatalf("expected device-plugin image validation error")
	}

	// validator image missing
	h = &RendererHandler{log: log, client: fake.NewClientBuilder().WithScheme(scheme).Build(), cfg: RenderConfig{Namespace: "ns", DevicePluginImage: "img"}}
	if _, err := h.HandlePool(context.Background(), &v1alpha1.GPUPool{Status: v1alpha1.GPUPoolStatus{Capacity: v1alpha1.GPUPoolCapacityStatus{Total: 1}}}); err == nil {
		t.Fatalf("expected validator image validation error")
	}

	// unsupported provider should no-op
	h = NewRendererHandler(log, fake.NewClientBuilder().WithScheme(scheme).Build(), RenderConfig{Namespace: "ns", DevicePluginImage: "img", ValidatorImage: "val"})
	pool := &v1alpha1.GPUPool{Spec: v1alpha1.GPUPoolSpec{Provider: "Other"}, Status: v1alpha1.GPUPoolStatus{Capacity: v1alpha1.GPUPoolCapacityStatus{Total: 1}}}
	if _, err := h.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("expected no error for other provider: %v", err)
	}

	// backend cleanup path
	ds := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "nvidia-device-plugin-pool", Namespace: "ns"}}
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "nvidia-device-plugin-pool-config", Namespace: "ns"}}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ds, cm).Build()
	h = NewRendererHandler(log, cl, RenderConfig{Namespace: "ns", DevicePluginImage: "img", ValidatorImage: "val"})
	pool = &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}, Spec: v1alpha1.GPUPoolSpec{Backend: "DRA"}}
	if _, err := h.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("cleanup path failed: %v", err)
	}
	if err := cl.Get(context.Background(), client.ObjectKeyFromObject(ds), &appsv1.DaemonSet{}); !apierrors.IsNotFound(err) {
		t.Fatalf("expected ds deleted, got %v", err)
	}
}

func TestRendererValidatorImageFallback(t *testing.T) {
	t.Setenv("NVIDIA_VALIDATOR_IMAGE", "")
	h := NewRendererHandler(testr.New(t), nil, RenderConfig{Namespace: "ns", DevicePluginImage: "dp:tag"})
	if h.cfg.ValidatorImage != "dp:tag" {
		t.Fatalf("expected validator image fallback to device-plugin, got %s", h.cfg.ValidatorImage)
	}
}

func TestRendererEnvDefaults(t *testing.T) {
	t.Setenv("POD_NAMESPACE", "ns-env")
	t.Setenv("DEFAULT_MIG_STRATEGY", "single")
	t.Setenv("NVIDIA_DEVICE_PLUGIN_IMAGE", "dp:env")
	t.Setenv("NVIDIA_MIG_MANAGER_IMAGE", "mig:env")
	t.Setenv("NVIDIA_VALIDATOR_IMAGE", "val:env")

	h := NewRendererHandler(testr.New(t), nil, RenderConfig{})
	if h.cfg.Namespace != "ns-env" {
		t.Fatalf("expected namespace from env, got %s", h.cfg.Namespace)
	}
	if h.cfg.DevicePluginImage != "dp:env" || h.cfg.MIGManagerImage != "mig:env" {
		t.Fatalf("env images not applied: %+v", h.cfg)
	}
	if h.cfg.DefaultMIGStrategy != "single" {
		t.Fatalf("expected strategy single, got %s", h.cfg.DefaultMIGStrategy)
	}
	if h.cfg.ValidatorImage != "val:env" {
		t.Fatalf("expected validator from env, got %s", h.cfg.ValidatorImage)
	}
}

func TestReconcileValidatorMissingImage(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	h := &RendererHandler{client: cl, cfg: RenderConfig{Namespace: "ns"}}
	if err := h.reconcileValidator(context.Background(), &v1alpha1.GPUPool{}); err == nil {
		t.Fatalf("expected error for missing validator image")
	}
}

func TestReconcileValidatorSuccess(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	h := &RendererHandler{client: cl, cfg: RenderConfig{Namespace: "ns", ValidatorImage: "val"}}
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}}
	if err := h.reconcileValidator(context.Background(), pool); err != nil {
		t.Fatalf("reconcileValidator: %v", err)
	}
	if err := cl.Get(context.Background(), client.ObjectKey{Namespace: "ns", Name: "nvidia-operator-validator-pool"}, &appsv1.DaemonSet{}); err != nil {
		t.Fatalf("validator ds not created: %v", err)
	}
}

func TestReconcileValidatorCreateError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	base := fake.NewClientBuilder().WithScheme(scheme).Build()
	errClient := &failingCreateClient{
		Client: base,
		errsByName: map[string]error{
			"nvidia-operator-validator-pool": apierrors.NewBadRequest("boom"),
		},
	}
	h := NewRendererHandler(testr.New(t), errClient, RenderConfig{Namespace: "ns", ValidatorImage: "img"})
	if err := h.reconcileValidator(context.Background(), &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}}); err == nil {
		t.Fatalf("expected validator create error")
	}
}

func TestHandlePoolMIGSkipWhenNoImage(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	h := NewRendererHandler(testr.New(t), cl, RenderConfig{Namespace: "ns", DevicePluginImage: "img", ValidatorImage: "val"})

	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool"},
		Spec: v1alpha1.GPUPoolSpec{
			Resource: v1alpha1.GPUPoolResourceSpec{Unit: "MIG", MIGProfile: "1g.10gb"},
		},
		Status: v1alpha1.GPUPoolStatus{Capacity: v1alpha1.GPUPoolCapacityStatus{Total: 1}},
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
		Status: v1alpha1.GPUPoolStatus{Capacity: v1alpha1.GPUPoolCapacityStatus{Total: 1}},
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
		ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: "ns"},
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
		Sharing struct {
			TimeSlicing struct {
				Resources []struct {
					Name     string `json:"name"`
					Replicas int32  `json:"replicas"`
				} `json:"resources"`
			} `json:"timeSlicing"`
		} `json:"sharing"`
	}
	if err := yaml.Unmarshal([]byte(cm.Data["config.yaml"]), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(cfg.Sharing.TimeSlicing.Resources) != 2 {
		t.Fatalf("expected two time-slicing resources, got %d", len(cfg.Sharing.TimeSlicing.Resources))
	}
	if cfg.Sharing.TimeSlicing.Resources[0].Name != "gpu.deckhouse.io/pool" || cfg.Sharing.TimeSlicing.Resources[0].Replicas != 5 {
		t.Fatalf("default resource override not applied: %+v", cfg.Sharing.TimeSlicing.Resources[0])
	}
	if cfg.Sharing.TimeSlicing.Resources[1].Name != "gpu.deckhouse.io/custom" || cfg.Sharing.TimeSlicing.Resources[1].Replicas != 2 {
		t.Fatalf("custom resource override not applied: %+v", cfg.Sharing.TimeSlicing.Resources[1])
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

func TestDevicePluginConfigMapOmitsTimeSlicingWhenSingleReplica(t *testing.T) {
	h := NewRendererHandler(testr.New(t), nil, RenderConfig{Namespace: "ns"})
	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool"},
		Spec: v1alpha1.GPUPoolSpec{
			Resource: v1alpha1.GPUPoolResourceSpec{
				Unit:          "Card",
				SlicesPerUnit: 1,
			},
		},
	}
	cm := h.devicePluginConfigMap(pool)
	if strings.Contains(cm.Data["config.yaml"], "timeSlicing") {
		t.Fatalf("timeSlicing should be omitted when replicas=1, got:\n%s", cm.Data["config.yaml"])
	}
}

func TestReconcileFailures(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	errClient := &failingCreateClient{Client: cl, err: apierrors.NewConflict(v1alpha1.GroupVersion.WithResource("configmaps").GroupResource(), "cm", nil)}
	h := NewRendererHandler(testr.New(t), errClient, RenderConfig{Namespace: "ns", DevicePluginImage: "img", MIGManagerImage: "mig"})

	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool"},
		Spec:       v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"}},
		Status:     v1alpha1.GPUPoolStatus{Capacity: v1alpha1.GPUPoolCapacityStatus{Total: 1}},
	}
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
		err:    apierrors.NewBadRequest("fail"),
		errsByName: map[string]error{
			"nvidia-device-plugin-pool-config": apierrors.NewBadRequest("fail"),
		},
	}
	h := NewRendererHandler(testr.New(t), dpErrClient, RenderConfig{Namespace: "ns", DevicePluginImage: "img"})
	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool"},
		Spec:       v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"}},
		Status:     v1alpha1.GPUPoolStatus{Capacity: v1alpha1.GPUPoolCapacityStatus{Total: 1}},
	}
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

func TestCleanupPoolResourcesRemovesValidator(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	objs := []client.Object{
		&appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "nvidia-device-plugin-pool", Namespace: "ns"}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "nvidia-device-plugin-pool-config", Namespace: "ns"}},
		&appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "nvidia-operator-validator-pool", Namespace: "ns"}},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	h := NewRendererHandler(testr.New(t), cl, RenderConfig{Namespace: "ns"})
	if err := h.cleanupPoolResources(context.Background(), "pool"); err != nil {
		t.Fatalf("cleanupPoolResources: %v", err)
	}
	for _, name := range []string{"nvidia-device-plugin-pool", "nvidia-operator-validator-pool"} {
		if err := cl.Get(context.Background(), client.ObjectKey{Namespace: "ns", Name: name}, &appsv1.DaemonSet{}); !apierrors.IsNotFound(err) {
			t.Fatalf("%s not cleaned: %v", name, err)
		}
	}
	if err := cl.Get(context.Background(), client.ObjectKey{Namespace: "ns", Name: "nvidia-device-plugin-pool-config"}, &corev1.ConfigMap{}); !apierrors.IsNotFound(err) {
		t.Fatalf("config not cleaned: %v", err)
	}
}

func TestCleanupPoolResourcesNoObjects(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	h := NewRendererHandler(testr.New(t), cl, RenderConfig{Namespace: "ns"})
	if err := h.cleanupPoolResources(context.Background(), "pool"); err != nil {
		t.Fatalf("cleanup with no objects should succeed: %v", err)
	}
}

func TestCleanupPoolResourcesValidatorError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	errs := map[string]error{
		"nvidia-operator-validator-pool": apierrors.NewBadRequest("fail"),
	}
	cl := &errorDeleteClient{Client: fake.NewClientBuilder().WithScheme(scheme).Build(), errs: errs}
	h := NewRendererHandler(testr.New(t), cl, RenderConfig{Namespace: "ns"})
	if err := h.cleanupPoolResources(context.Background(), "pool"); err == nil {
		t.Fatalf("expected validator delete error")
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

func TestAddOwnerResolvesKind(t *testing.T) {
	// namespaced pool without explicit Kind should resolve to GPUPool
	nsPool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool-a", Namespace: "ns", UID: "uid-a"},
	}
	obj := &corev1.ConfigMap{}
	addOwner(obj, nsPool)
	if len(obj.OwnerReferences) != 0 {
		t.Fatalf("expected no owner refs for cross-namespace resource")
	}

	// explicit Kind must be preserved even for cluster-scoped pool
	clusterPool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool-b", UID: "uid-b"},
		TypeMeta:   metav1.TypeMeta{Kind: "CustomKind"},
	}
	obj2 := &corev1.ConfigMap{}
	addOwner(obj2, clusterPool)
	if got := obj2.OwnerReferences[0].Kind; got != "CustomKind" {
		t.Fatalf("expected CustomKind, got %s", got)
	}
}

func TestCreateOrUpdateSkipsEqualObjects(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	existing := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "ns"},
		Data:       map[string]string{"k": "v"},
	}
	base := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()
	cc := &countingClient{Client: base}
	h := NewRendererHandler(testr.New(t), cc, RenderConfig{Namespace: "ns", DevicePluginImage: "img"})
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: "ns", UID: "uid"}}

	want := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "ns"},
		Data:       map[string]string{"k": "v"},
	}
	// first call injects owner reference
	if err := h.createOrUpdate(context.Background(), want, pool); err != nil {
		t.Fatalf("createOrUpdate returned error: %v", err)
	}
	if cc.updates != 1 {
		t.Fatalf("expected one update for owner injection, got %d", cc.updates)
	}

	// change data to trigger update
	want.Data["k2"] = "v2"
	if err := h.createOrUpdate(context.Background(), want, pool); err != nil {
		t.Fatalf("createOrUpdate returned error: %v", err)
	}
	if cc.updates != 2 {
		t.Fatalf("expected second update for data change, got %d", cc.updates)
	}
}

func TestCreateOrUpdateDaemonSetOwnerInjection(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "ds", Namespace: "ns"},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "x"}},
			Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "x"}}},
		},
	}
	base := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ds).Build()
	cc := &countingClient{Client: base}
	h := NewRendererHandler(testr.New(t), cc, RenderConfig{Namespace: "ns", DevicePluginImage: "img"})
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: "ns", UID: "uid"}}

	want := ds.DeepCopy()
	if err := h.createOrUpdate(context.Background(), want, pool); err != nil {
		t.Fatalf("createOrUpdate returned error: %v", err)
	}
	if cc.updates != 1 {
		t.Fatalf("expected one update to inject owner, got %d", cc.updates)
	}
	if err := h.createOrUpdate(context.Background(), want, pool); err != nil {
		t.Fatalf("createOrUpdate returned error: %v", err)
	}
	if cc.updates != 1 {
		t.Fatalf("expected no extra updates when daemonset unchanged, got %d", cc.updates)
	}
}

func TestCreateOrUpdateCreatePathAndOwnerPresent(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	cc := &countingClient{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	h := NewRendererHandler(testr.New(t), cc, RenderConfig{Namespace: "ns", DevicePluginImage: "img"})
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: "ns", UID: "uid"}}

	// create new configmap
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "new-cm", Namespace: "ns"}, Data: map[string]string{"a": "b"}}
	if err := h.createOrUpdate(context.Background(), cm, pool); err != nil {
		t.Fatalf("createOrUpdate returned error: %v", err)
	}
	if cc.creates != 1 || cc.updates != 0 {
		t.Fatalf("expected create path, got creates=%d updates=%d", cc.creates, cc.updates)
	}

	// owner already present -> no update
	withOwner := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "owned",
			Namespace: "ns",
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: v1alpha1.GroupVersion.String(),
				Kind:       "GPUPool",
				Name:       "pool",
				Controller: ptr.To(true),
			}},
		},
		Data: map[string]string{"x": "y"},
	}
	cc = &countingClient{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(withOwner).Build()}
	h = NewRendererHandler(testr.New(t), cc, RenderConfig{Namespace: "ns", DevicePluginImage: "img"})
	if err := h.createOrUpdate(context.Background(), withOwner.DeepCopy(), pool); err != nil {
		t.Fatalf("createOrUpdate returned error: %v", err)
	}
	if cc.updates != 0 {
		t.Fatalf("expected no update when owner already present, got %d", cc.updates)
	}
}

func TestCreateOrUpdateUnsupported(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	cc := &countingClient{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	h := NewRendererHandler(testr.New(t), cc, RenderConfig{Namespace: "ns", DevicePluginImage: "img"})
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: "ns", UID: "uid"}}

	if err := h.createOrUpdate(context.Background(), &corev1.Service{}, pool); err == nil {
		t.Fatalf("expected error for unsupported object type")
	}
}

func TestCreateOrUpdateConfigMapSkipWhenEqualAndOwned(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "owned-cm",
			Namespace: "ns",
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: v1alpha1.GroupVersion.String(),
				Kind:       "GPUPool",
				Name:       "pool",
				Controller: ptr.To(true),
			}},
		},
		Data: map[string]string{"a": "b"},
	}
	cc := &countingClient{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()}
	h := NewRendererHandler(testr.New(t), cc, RenderConfig{Namespace: "ns", DevicePluginImage: "img"})
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: "ns", UID: "uid"}}
	if err := h.createOrUpdate(context.Background(), cm.DeepCopy(), pool); err != nil {
		t.Fatalf("createOrUpdate returned error: %v", err)
	}
	if cc.updates != 0 {
		t.Fatalf("expected no update when configmap already matches, got %d", cc.updates)
	}
}

func TestCreateOrUpdateCreatesDaemonSetWhenMissing(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	cc := &countingClient{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	h := NewRendererHandler(testr.New(t), cc, RenderConfig{Namespace: "ns", DevicePluginImage: "img"})
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: "ns", UID: "uid"}}
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "new-ds", Namespace: "ns"},
		Spec:       appsv1.DaemonSetSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "y"}}, Template: corev1.PodTemplateSpec{}},
	}
	if err := h.createOrUpdate(context.Background(), ds, pool); err != nil {
		t.Fatalf("createOrUpdate returned error: %v", err)
	}
	if cc.creates != 1 || cc.updates != 0 {
		t.Fatalf("expected daemonset create, got creates=%d updates=%d", cc.creates, cc.updates)
	}
}

func TestHasOwnerClusterPool(t *testing.T) {
	obj := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: v1alpha1.GroupVersion.String(),
				Kind:       "ClusterGPUPool",
				Name:       "cluster-pool",
			}},
		},
	}
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "cluster-pool"}, TypeMeta: metav1.TypeMeta{Kind: "ClusterGPUPool"}}
	if !hasOwner(obj, pool) {
		t.Fatalf("expected hasOwner true for cluster pool")
	}
}

func TestCreateOrUpdateGetError(t *testing.T) {
	h := NewRendererHandler(testr.New(t), &errGetClientRenderer{}, RenderConfig{Namespace: "ns", DevicePluginImage: "img"})
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: "ns", UID: "uid"}}
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "ns"}}
	if err := h.createOrUpdate(context.Background(), cm, pool); err == nil {
		t.Fatalf("expected error when client.Get fails")
	}
}

func TestCreateOrUpdateDaemonSetSkipWhenEqual(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	existing := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ds-owned",
			Namespace: "ns",
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: v1alpha1.GroupVersion.String(),
				Kind:       "GPUPool",
				Name:       "pool",
				Controller: ptr.To(true),
			}},
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "x"}},
			Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "x"}}},
		},
	}
	cc := &countingClient{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()}
	h := NewRendererHandler(testr.New(t), cc, RenderConfig{Namespace: "ns", DevicePluginImage: "img"})
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: "ns", UID: "uid"}}
	want := existing.DeepCopy()
	if err := h.createOrUpdate(context.Background(), want, pool); err != nil {
		t.Fatalf("createOrUpdate returned error: %v", err)
	}
	if cc.updates != 0 {
		t.Fatalf("expected no update when daemonset already matches, got %d", cc.updates)
	}
}

func TestCreateOrUpdateDaemonSetGetError(t *testing.T) {
	h := NewRendererHandler(testr.New(t), &errGetClientRenderer{}, RenderConfig{Namespace: "ns", DevicePluginImage: "img"})
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: "ns", UID: "uid"}}
	ds := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "ds", Namespace: "ns"}}
	if err := h.createOrUpdate(context.Background(), ds, pool); err == nil {
		t.Fatalf("expected error when client.Get fails for daemonset")
	}
}

func TestHasOwnerClusterKindInference(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: v1alpha1.GroupVersion.String(),
				Kind:       "ClusterGPUPool",
				Name:       "pool",
			}},
		},
	}
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}}
	if !hasOwner(cm, pool) {
		t.Fatalf("expected hasOwner true when Kind inferred for cluster pool")
	}
}

type countingClient struct {
	client.Client
	creates int
	updates int
}

func (c *countingClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	c.creates++
	return c.Client.Create(ctx, obj, opts...)
}

func (c *countingClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	c.updates++
	return c.Client.Update(ctx, obj, opts...)
}

type errGetClientRenderer struct{ client.Client }

func (errGetClientRenderer) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	return apierrors.NewForbidden(corev1.Resource("configmaps"), "cm", fmt.Errorf("boom"))
}

func TestBuildCustomTolerations(t *testing.T) {
	tols := buildCustomTolerations([]string{"a", "", "a", "b"})
	if len(tols) != 2 {
		t.Fatalf("expected 2 tolerations, got %d", len(tols))
	}
	keys := map[string]bool{tols[0].Key: true, tols[1].Key: true}
	if !keys["a"] || !keys["b"] {
		t.Fatalf("unexpected keys: %v", keys)
	}
	if buildCustomTolerations(nil) != nil {
		t.Fatalf("nil input should return nil slice")
	}
}

func TestMergeTolerations(t *testing.T) {
	base := []corev1.Toleration{
		{Key: "a", Operator: corev1.TolerationOpExists},
	}
	extra := []corev1.Toleration{
		{Key: "a", Operator: corev1.TolerationOpExists},
		{Key: "b", Operator: corev1.TolerationOpEqual, Value: "v"},
	}
	merged := mergeTolerations(base, extra)
	if len(merged) != 2 {
		t.Fatalf("expected deduplicated tolerations, got %d: %v", len(merged), merged)
	}
	if merged[0].Key != "a" || merged[1].Key != "b" {
		t.Fatalf("unexpected order or keys: %v", merged)
	}
	merged = mergeTolerations(base, nil)
	if len(merged) != 1 || merged[0].Key != "a" {
		t.Fatalf("merge with nil extra should return base: %v", merged)
	}
}

func TestDevicePluginConfigMapClusterPrefix(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	h := NewRendererHandler(testr.New(t), fake.NewClientBuilder().WithScheme(scheme).Build(), RenderConfig{Namespace: "ns"})
	pool := &v1alpha1.GPUPool{
		TypeMeta:   metav1.TypeMeta{Kind: "ClusterGPUPool"},
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-a"},
		Spec:       v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"}},
		Status:     v1alpha1.GPUPoolStatus{Capacity: v1alpha1.GPUPoolCapacityStatus{Total: 1}},
	}
	cm := h.devicePluginConfigMap(pool)
	cfg := cm.Data["config.yaml"]
	if !strings.Contains(cfg, "cluster.gpu.deckhouse.io/cluster-a") {
		t.Fatalf("expected cluster prefix in device plugin config, got %s", cfg)
	}
}

func TestDevicePluginConfigMapSharingBranches(t *testing.T) {
	h := NewRendererHandler(testr.New(t), nil, RenderConfig{Namespace: "ns"})
	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool"},
		Spec: v1alpha1.GPUPoolSpec{
			Resource: v1alpha1.GPUPoolResourceSpec{
				Unit:          "Card",
				SlicesPerUnit: 1,
				TimeSlicingResources: []v1alpha1.GPUPoolTimeSlicingResource{
					{Name: "", SlicesPerUnit: 2},
				},
			},
		},
	}
	cm := h.devicePluginConfigMap(pool)
	if !strings.Contains(cm.Data["config.yaml"], "timeSlicing") {
		t.Fatalf("expected sharing block present")
	}
	// no sharing when replicas ==1 and overrides removed
	pool.Spec.Resource.TimeSlicingResources = nil
	pool.Spec.Resource.SlicesPerUnit = 1
	cm = h.devicePluginConfigMap(pool)
	if strings.Contains(cm.Data["config.yaml"], "timeSlicing") {
		t.Fatalf("expected sharing block absent when replicas=1")
	}
}

func TestPoolNodeTolerationsDedup(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node1"},
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{
				{Key: "a", Effect: corev1.TaintEffectNoSchedule},
				{Key: "a", Effect: corev1.TaintEffectNoSchedule},
				{Key: "b", Effect: corev1.TaintEffectNoExecute},
			},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	h := NewRendererHandler(testr.New(t), cl, RenderConfig{Namespace: "ns"})
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "p"}, Status: v1alpha1.GPUPoolStatus{Nodes: []v1alpha1.GPUPoolNodeStatus{{Name: "node1"}}}}
	tols := h.poolNodeTolerations(context.Background(), pool)
	if len(tols) != 2 {
		t.Fatalf("expected 2 unique tolerations, got %v", tols)
	}
}

func TestPoolNodeTolerationsNilClient(t *testing.T) {
	h := NewRendererHandler(testr.New(t), nil, RenderConfig{Namespace: "ns"})
	tols := h.poolNodeTolerations(context.Background(), &v1alpha1.GPUPool{})
	if tols != nil {
		t.Fatalf("expected nil tolerations when client is nil, got %v", tols)
	}
}

func TestPoolNodeTolerationsMissingNode(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	h := NewRendererHandler(testr.New(t), cl, RenderConfig{Namespace: "ns"})
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "p"}, Status: v1alpha1.GPUPoolStatus{Nodes: []v1alpha1.GPUPoolNodeStatus{{Name: "absent"}}}}
	if tols := h.poolNodeTolerations(context.Background(), pool); len(tols) != 0 {
		t.Fatalf("expected empty tolerations when node missing, got %v", tols)
	}
}

func TestDevicePluginConfigMapInt32Replicas(t *testing.T) {
	h := NewRendererHandler(testr.New(t), nil, RenderConfig{Namespace: "ns"})
	rep := int32(3)
	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool"},
		Spec: v1alpha1.GPUPoolSpec{
			Resource: v1alpha1.GPUPoolResourceSpec{
				Unit: "Card",
				TimeSlicingResources: []v1alpha1.GPUPoolTimeSlicingResource{
					{Name: "custom", SlicesPerUnit: rep},
				},
			},
		},
	}
	cm := h.devicePluginConfigMap(pool)
	if !strings.Contains(cm.Data["config.yaml"], "custom") || !strings.Contains(cm.Data["config.yaml"], "3") {
		t.Fatalf("expected custom time slicing replicas captured, got %s", cm.Data["config.yaml"])
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
