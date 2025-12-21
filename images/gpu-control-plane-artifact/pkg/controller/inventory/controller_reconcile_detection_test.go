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
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/config"
	bootstrapmeta "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/bootstrap/meta"
	invservice "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory/internal/service"
	"github.com/go-logr/logr/testr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
)

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

	detections, err := reconciler.collectNodeDetections(context.Background(), node.Name)
	if err != nil {
		t.Fatalf("expected detections collected: %v", err)
	}

	device := &v1alpha1.GPUDevice{}
	invservice.ApplyDetection(device, deviceSnapshot{Index: "0"}, detections)
	if device.Status.Hardware.Product != "A100" {
		t.Fatalf("expected detected product to be applied, got %q", device.Status.Hardware.Product)
	}
	if device.Status.Hardware.UUID != "GPU-AAA" {
		t.Fatalf("expected detected UUID to be applied, got %q", device.Status.Hardware.UUID)
	}
	if device.Status.Hardware.PCI.Address != "0000:17:00.0" {
		t.Fatalf("expected detected PCI address to be applied, got %q", device.Status.Hardware.PCI.Address)
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

	if _, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}}); err != nil {
		t.Fatalf("expected reconcile to succeed: %v", err)
	}

	device := &v1alpha1.GPUDevice{}
	if err := baseClient.Get(context.Background(), types.NamespacedName{Name: "worker-detect-reconcile-0-10de-2203"}, device); err != nil {
		t.Fatalf("expected device created: %v", err)
	}

	if device.Status.Hardware.Product != "A100" {
		t.Fatalf("expected product to be populated from detections, got %q", device.Status.Hardware.Product)
	}
	if device.Status.Hardware.UUID != "GPU-AAA" {
		t.Fatalf("expected uuid to be populated from detections, got %q", device.Status.Hardware.UUID)
	}
}
