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

package workload

import (
	"context"
	"strings"
	"testing"

	"github.com/go-logr/logr/testr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/yaml"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	poolcommon "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/common"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/config"
)

func TestReconcileCreatesDevicePluginResources(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	cl := withPoolDeviceIndexes(fake.NewClientBuilder().WithScheme(scheme)).Build()
	d := NewDeps(testr.New(t), cl, config.WorkloadConfig{
		Namespace:            "gpu-ns",
		DevicePluginImage:    "device-plugin:tag",
		DefaultMIGStrategy:   "single",
		CustomTolerationKeys: []string{"custom-tol"},
		ValidatorImage:       "validator:tag",
	})

	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "alpha", Namespace: "gpu-ns", UID: "12345"},
		Spec: v1alpha1.GPUPoolSpec{
			Resource: v1alpha1.GPUPoolResourceSpec{
				Unit:          "Card",
				SlicesPerUnit: 4,
			},
		},
		Status: v1alpha1.GPUPoolStatus{Capacity: v1alpha1.GPUPoolCapacityStatus{Total: 1}},
	}

	if _, err := Reconcile(context.Background(), d, pool); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	cm := &corev1.ConfigMap{}
	if err := cl.Get(context.Background(), client.ObjectKey{Namespace: "gpu-ns", Name: "nvidia-device-plugin-alpha-config"}, cm); err != nil {
		t.Fatalf("get configmap: %v", err)
	}
	if len(cm.OwnerReferences) != 1 || cm.OwnerReferences[0].Name != "alpha" {
		t.Fatalf("owner reference not set on ConfigMap")
	}

	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(cm.Data["config.yaml"]), &parsed); err != nil {
		t.Fatalf("parse config: %v", err)
	}
	sharing, ok := parsed["sharing"].(map[string]any)
	if !ok {
		t.Fatalf("sharing block missing")
	}
	timeSlicing, ok := sharing["timeSlicing"].(map[string]any)
	if !ok {
		t.Fatalf("timeSlicing block missing")
	}
	resList, ok := timeSlicing["resources"].([]any)
	if !ok || len(resList) != 1 {
		t.Fatalf("unexpected resources: %v", resList)
	}
	res, _ := resList[0].(map[string]any)
	if res["name"] != "alpha" {
		t.Fatalf("unexpected resource name: %v", res["name"])
	}
	switch v := res["replicas"].(type) {
	case int:
		if v != 4 {
			t.Fatalf("expected replicas 4, got %v", v)
		}
	case int64:
		if v != 4 {
			t.Fatalf("expected replicas 4, got %v", v)
		}
	case float64:
		if int(v) != 4 {
			t.Fatalf("expected replicas 4, got %v", v)
		}
	default:
		t.Fatalf("unexpected replicas type %T", v)
	}

	ds := &appsv1.DaemonSet{}
	if err := cl.Get(context.Background(), client.ObjectKey{Namespace: "gpu-ns", Name: "nvidia-device-plugin-alpha"}, ds); err != nil {
		t.Fatalf("get daemonset: %v", err)
	}
	if ds.Spec.Template.Spec.Containers[0].Image != "device-plugin:tag" {
		t.Fatalf("unexpected image: %s", ds.Spec.Template.Spec.Containers[0].Image)
	}
	expectedTol := corev1.Toleration{Key: poolcommon.PoolLabelKey(pool), Operator: corev1.TolerationOpEqual, Value: "alpha", Effect: corev1.TaintEffectNoSchedule}
	if !hasToleration(ds.Spec.Template.Spec.Tolerations, expectedTol) {
		t.Fatalf("pool toleration missing")
	}
	if !hasToleration(ds.Spec.Template.Spec.Tolerations, corev1.Toleration{Key: "custom-tol", Operator: corev1.TolerationOpExists}) {
		t.Fatalf("custom toleration missing")
	}
	reqs := ds.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution
	if reqs == nil || len(reqs.NodeSelectorTerms) != 1 {
		t.Fatalf("node affinity not set")
	}
	me := reqs.NodeSelectorTerms[0].MatchExpressions[0]
	if me.Key != poolcommon.PoolLabelKey(pool) || me.Operator != corev1.NodeSelectorOpIn || len(me.Values) != 1 || me.Values[0] != "alpha" {
		t.Fatalf("unexpected affinity %+v", me)
	}

	validator := &appsv1.DaemonSet{}
	if err := cl.Get(context.Background(), client.ObjectKey{Namespace: "gpu-ns", Name: "nvidia-operator-validator-alpha"}, validator); err != nil {
		t.Fatalf("get validator daemonset: %v", err)
	}
	if len(validator.Spec.Template.Spec.InitContainers) == 0 {
		t.Fatalf("plugin validation init container missing")
	}
	if !mountExists(validator.Spec.Template.Spec.InitContainers[0].VolumeMounts, "/var/lib/kubelet/device-plugins") {
		t.Fatalf("expected kubelet device-plugins mount on validator")
	}
}

func TestReconcileCreatesResourcesWhenPoolHasPendingAssignments(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "device-1"},
		Status: v1alpha1.GPUDeviceStatus{
			PoolRef:    &v1alpha1.GPUPoolReference{Name: "alpha", Namespace: "gpu-ns"},
			State:      v1alpha1.GPUDeviceStatePendingAssignment,
			Hardware:   v1alpha1.GPUDeviceHardware{UUID: "GPU-AAA"},
			Managed:    true,
			AutoAttach: false,
		},
	}

	cl := withPoolDeviceIndexes(fake.NewClientBuilder().WithScheme(scheme)).WithObjects(device).Build()
	d := NewDeps(testr.New(t), cl, config.WorkloadConfig{
		Namespace:         "gpu-ns",
		DevicePluginImage: "device-plugin:tag",
		ValidatorImage:    "validator:tag",
	})

	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "alpha", Namespace: "gpu-ns", UID: "12345"},
		Spec: v1alpha1.GPUPoolSpec{
			Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"},
		},
		Status: v1alpha1.GPUPoolStatus{Capacity: v1alpha1.GPUPoolCapacityStatus{Total: 0}},
	}

	if _, err := Reconcile(context.Background(), d, pool); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	ds := &appsv1.DaemonSet{}
	if err := cl.Get(context.Background(), client.ObjectKey{Namespace: "gpu-ns", Name: "nvidia-device-plugin-alpha"}, ds); err != nil {
		t.Fatalf("get daemonset: %v", err)
	}
}

func TestReconcileEnablesPluginValidationWhenConfigured(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	d := NewDeps(testr.New(t), cl, config.WorkloadConfig{
		Namespace:         "gpu-ns",
		DevicePluginImage: "device-plugin:tag",
		ValidatorImage:    "validator:tag",
	})

	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "alpha", Namespace: "gpu-ns", UID: "1234"},
		Spec: v1alpha1.GPUPoolSpec{
			Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"},
		},
		Status: v1alpha1.GPUPoolStatus{
			Capacity: v1alpha1.GPUPoolCapacityStatus{Total: 1},
		},
	}

	if _, err := Reconcile(context.Background(), d, pool); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	validator := &appsv1.DaemonSet{}
	if err := cl.Get(context.Background(), client.ObjectKey{Namespace: "gpu-ns", Name: "nvidia-operator-validator-alpha"}, validator); err != nil {
		t.Fatalf("get validator daemonset: %v", err)
	}
	if len(validator.Spec.Template.Spec.InitContainers) != 1 {
		t.Fatalf("expected plugin validation init container when enabled, got %d", len(validator.Spec.Template.Spec.InitContainers))
	}
	if validator.Spec.Template.Spec.InitContainers[0].Image != "validator:tag" {
		t.Fatalf("unexpected validator image: %s", validator.Spec.Template.Spec.InitContainers[0].Image)
	}
	if !mountExists(validator.Spec.Template.Spec.InitContainers[0].VolumeMounts, "/var/lib/kubelet/device-plugins") {
		t.Fatalf("expected kubelet device-plugins mount when enabled")
	}
}

func TestReconcileCreatesMIGManagerResources(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	d := NewDeps(testr.New(t), cl, config.WorkloadConfig{
		Namespace:         "gpu-ns",
		DevicePluginImage: "device-plugin:tag",
		MIGManagerImage:   "mig-manager:tag",
		ValidatorImage:    "validator:tag",
	})

	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "beta", UID: "6789"},
		Spec: v1alpha1.GPUPoolSpec{
			Resource: v1alpha1.GPUPoolResourceSpec{
				Unit:       "MIG",
				MIGProfile: "1g.10gb",
			},
		},
		Status: v1alpha1.GPUPoolStatus{Capacity: v1alpha1.GPUPoolCapacityStatus{Total: 1}},
	}

	if _, err := Reconcile(context.Background(), d, pool); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	cm := &corev1.ConfigMap{}
	if err := cl.Get(context.Background(), client.ObjectKey{Namespace: "gpu-ns", Name: "nvidia-mig-manager-beta-config"}, cm); err != nil {
		t.Fatalf("get mig config: %v", err)
	}
	if len(cm.OwnerReferences) != 1 || cm.OwnerReferences[0].Name != "beta" {
		t.Fatalf("owner reference not set on MIG ConfigMap")
	}
	if !strings.Contains(cm.Data["config.yaml"], "1g.10gb") {
		t.Fatalf("config.yaml does not contain expected profiles: %s", cm.Data["config.yaml"])
	}
	if !strings.Contains(cm.Data["config.yaml"], "pciBusId: all") {
		t.Fatalf("config.yaml does not contain expected target: %s", cm.Data["config.yaml"])
	}

	scripts := &corev1.ConfigMap{}
	if err := cl.Get(context.Background(), client.ObjectKey{Namespace: "gpu-ns", Name: "nvidia-mig-manager-beta-scripts"}, scripts); err != nil {
		t.Fatalf("get mig scripts: %v", err)
	}
	if scripts.Data["reconfigure-mig.sh"] == "" || scripts.Data["prestop.sh"] == "" {
		t.Fatalf("scripts must not be empty")
	}

	clients := &corev1.ConfigMap{}
	if err := cl.Get(context.Background(), client.ObjectKey{Namespace: "gpu-ns", Name: "nvidia-mig-manager-beta-gpu-clients"}, clients); err != nil {
		t.Fatalf("get mig clients: %v", err)
	}
	if !strings.Contains(clients.Data["clients.yaml"], "nvidia-dcgm.service") {
		t.Fatalf("clients.yaml missing expected service")
	}

	ds := &appsv1.DaemonSet{}
	if err := cl.Get(context.Background(), client.ObjectKey{Namespace: "gpu-ns", Name: "nvidia-mig-manager-beta"}, ds); err != nil {
		t.Fatalf("get mig daemonset: %v", err)
	}
	if ds.Spec.Template.Spec.Containers[0].Image != "mig-manager:tag" {
		t.Fatalf("unexpected mig-manager image: %s", ds.Spec.Template.Spec.Containers[0].Image)
	}
	if !hasToleration(ds.Spec.Template.Spec.Tolerations, corev1.Toleration{Key: poolcommon.PoolLabelKey(pool), Operator: corev1.TolerationOpEqual, Value: "beta", Effect: corev1.TaintEffectNoSchedule}) {
		t.Fatalf("pool toleration missing on mig-manager")
	}
}

func TestReconcileCleansUpForNonDevicePluginBackend(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	dpCMName := "nvidia-device-plugin-alpha-config"
	dpDSName := "nvidia-device-plugin-alpha"
	migDSName := "nvidia-mig-manager-alpha"
	objs := []client.Object{
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: dpCMName, Namespace: "gpu-ns"}},
		&appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: dpDSName, Namespace: "gpu-ns"}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "nvidia-mig-manager-alpha-config", Namespace: "gpu-ns"}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "nvidia-mig-manager-alpha-scripts", Namespace: "gpu-ns"}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "nvidia-mig-manager-alpha-gpu-clients", Namespace: "gpu-ns"}},
		&appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: migDSName, Namespace: "gpu-ns"}},
	}

	cl := withPoolDeviceIndexes(fake.NewClientBuilder().WithScheme(scheme)).WithObjects(objs...).Build()
	d := NewDeps(testr.New(t), cl, config.WorkloadConfig{
		Namespace:         "gpu-ns",
		DevicePluginImage: "device-plugin:tag",
		ValidatorImage:    "validator:tag",
	})

	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "alpha"},
		Spec:       v1alpha1.GPUPoolSpec{Backend: "DRA"},
	}

	if _, err := Reconcile(context.Background(), d, pool); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if err := cl.Get(context.Background(), client.ObjectKey{Name: dpCMName, Namespace: "gpu-ns"}, &corev1.ConfigMap{}); err == nil {
		t.Fatalf("configmap should be deleted for non-DP backend")
	}
	if err := cl.Get(context.Background(), client.ObjectKey{Name: dpDSName, Namespace: "gpu-ns"}, &appsv1.DaemonSet{}); err == nil {
		t.Fatalf("daemonset should be deleted for non-DP backend")
	}
	if err := cl.Get(context.Background(), client.ObjectKey{Name: migDSName, Namespace: "gpu-ns"}, &appsv1.DaemonSet{}); err == nil {
		t.Fatalf("mig-manager daemonset should be deleted for non-DP backend")
	}
	if err := cl.Get(context.Background(), client.ObjectKey{Name: "nvidia-mig-manager-alpha-config", Namespace: "gpu-ns"}, &corev1.ConfigMap{}); err == nil {
		t.Fatalf("mig-manager config should be deleted for non-DP backend")
	}
	if err := cl.Get(context.Background(), client.ObjectKey{Name: "nvidia-mig-manager-alpha-scripts", Namespace: "gpu-ns"}, &corev1.ConfigMap{}); err == nil {
		t.Fatalf("mig-manager scripts should be deleted for non-DP backend")
	}
	if err := cl.Get(context.Background(), client.ObjectKey{Name: "nvidia-mig-manager-alpha-gpu-clients", Namespace: "gpu-ns"}, &corev1.ConfigMap{}); err == nil {
		t.Fatalf("mig-manager gpu-clients should be deleted for non-DP backend")
	}
}

func hasToleration(list []corev1.Toleration, expected corev1.Toleration) bool {
	for _, t := range list {
		if t.Key == expected.Key && t.Operator == expected.Operator && t.Value == expected.Value && t.Effect == expected.Effect {
			return true
		}
	}
	return false
}

func mountExists(mounts []corev1.VolumeMount, path string) bool {
	for _, m := range mounts {
		if m.MountPath == path {
			return true
		}
	}
	return false
}
