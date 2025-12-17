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
	"reflect"
	"strconv"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	bootstrapmeta "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/bootstrap/meta"
)

func TestApplyDetectionPopulatesHardware(t *testing.T) {
	device := &v1alpha1.GPUDevice{}
	snapshot := deviceSnapshot{Index: "0", UUID: "GPU-AAA"}
	detections := nodeDetection{
		byUUID: map[string]detectGPUEntry{
			"GPU-AAA": {
				Index:   0,
				UUID:    "GPU-AAA",
				Product: "A100",
				MemoryInfo: detectGPUMemory{
					Total: 80 * 1024 * 1024 * 1024,
					Free:  60 * 1024 * 1024 * 1024,
					Used:  20 * 1024 * 1024 * 1024,
				},
				PowerUsage:                  120000,
				PowerManagementDefaultLimit: 150000,
				Utilization: detectGPUUtilization{
					GPU:    75,
					Memory: 40,
				},
				MemoryMiB:          80 * 1024,
				ComputeMajor:       8,
				ComputeMinor:       0,
				NUMANode:           detectionPtrInt32(1),
				SMCount:            detectionPtrInt32(108),
				MemoryBandwidthMiB: detectionPtrInt32(1555),
				PCI: detectGPUPCI{
					Address: "0000:17:00.0",
					Vendor:  "10de",
					Device:  "2203",
					Class:   "0302",
				},
				Board:       "board-1",
				Family:      "ampere",
				Serial:      "serial-123",
				DisplayMode: "Enabled",
				MIG:         detectGPUMIG{Capable: true, ProfilesSupported: []string{"mig-1g.10gb"}},
				PowerState:  0,
			},
		},
		byIndex: map[string]detectGPUEntry{},
	}

	applyDetection(device, snapshot, detections)

	if device.Status.Hardware.Product != "A100" || device.Status.Hardware.PCI.Vendor != "10de" || device.Status.Hardware.PCI.Device != "2203" {
		t.Fatalf("unexpected hardware update: %+v", device.Status.Hardware)
	}
	if !device.Status.Hardware.MIG.Capable || len(device.Status.Hardware.MIG.ProfilesSupported) != 1 || device.Status.Hardware.MIG.ProfilesSupported[0] != "1g.10gb" {
		t.Fatalf("expected MIG profiles propagated, got %+v", device.Status.Hardware.MIG)
	}
}

func TestApplyDetectionMissingEntriesDoesNothing(t *testing.T) {
	device := &v1alpha1.GPUDevice{}
	snapshot := deviceSnapshot{Index: "10", UUID: "GPU-ZZZ"}
	before := device.Status.Hardware
	applyDetection(device, snapshot, nodeDetection{})
	if !reflect.DeepEqual(before, device.Status.Hardware) {
		t.Fatalf("hardware should stay untouched without matching detection: before=%+v after=%+v", before, device.Status.Hardware)
	}
}

func TestNodeDetectionFallbacks(t *testing.T) {
	detections := nodeDetection{
		byUUID: map[string]detectGPUEntry{
			"GPU-A": {UUID: "GPU-A", Index: 1},
		},
		byIndex: map[string]detectGPUEntry{
			"5": {Index: 5, UUID: "GPU-B"},
		},
	}

	if entry, ok := detections.find(deviceSnapshot{Index: "0", UUID: "GPU-A"}); !ok || entry.UUID != "GPU-A" {
		t.Fatalf("expected lookup by UUID, got %+v ok=%v", entry, ok)
	}
	if entry, ok := detections.find(deviceSnapshot{Index: "5"}); !ok || entry.Index != 5 {
		t.Fatalf("expected lookup by index, got %+v ok=%v", entry, ok)
	}
	if _, ok := detections.find(deviceSnapshot{Index: "99"}); ok {
		t.Fatalf("unexpected entry for missing snapshot")
	}
}

func TestDetectGPUPort(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "gfd-extender",
					Ports: []corev1.ContainerPort{{ContainerPort: 1234}},
				},
			},
		},
	}
	if port := detectGPUPort(pod); port != 1234 {
		t.Fatalf("expected gfd-extender port from container, got %d", port)
	}
	pod.Spec.Containers[0].Name = "other"
	if port := detectGPUPort(pod); port != 0 {
		t.Fatalf("expected zero port when container missing, got %d", port)
	}
}

func TestCollectNodeDetectionsMissingPodIsSilent(t *testing.T) {
	rec := &Reconciler{client: newTestClient(newTestScheme(t))}
	detections, err := rec.collectNodeDetections(context.Background(), "node-no-pod")
	if err != nil {
		t.Fatalf("unexpected error when pod missing: %v", err)
	}
	if len(detections.byIndex) != 0 || len(detections.byUUID) != 0 {
		t.Fatalf("expected empty detections, got %+v", detections)
	}
}

func TestCollectNodeDetectionsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "fail", http.StatusInternalServerError)
	}))
	defer server.Close()

	host, portStr, _ := strings.Cut(server.Listener.Addr().String(), ":")
	port, _ := strconv.Atoi(portStr)

	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-http-error"}}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gfd-pod",
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

	rec := &Reconciler{
		client: newTestClient(newTestScheme(t), node, pod),
	}
	orig := detectHTTPClient
	detectHTTPClient = server.Client()
	defer func() { detectHTTPClient = orig }()

	if detections, err := rec.collectNodeDetections(context.Background(), node.Name); err != nil {
		t.Fatalf("unexpected error when gfd-extender returns non-200: %v", err)
	} else if len(detections.byIndex) != 0 || len(detections.byUUID) != 0 {
		t.Fatalf("expected empty detections on non-200, got %+v", detections)
	}
}

func TestCollectNodeDetectionsDecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("invalid-json"))
	}))
	defer server.Close()

	host, portStr, _ := strings.Cut(server.Listener.Addr().String(), ":")
	port, _ := strconv.Atoi(portStr)

	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-decode-error"}}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gfd-pod-decode",
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

	rec := &Reconciler{client: newTestClient(newTestScheme(t), node, pod)}
	orig := detectHTTPClient
	detectHTTPClient = server.Client()
	defer func() { detectHTTPClient = orig }()

	if _, err := rec.collectNodeDetections(context.Background(), node.Name); err == nil {
		t.Fatalf("expected error when gfd-extender returns malformed json")
	}
}

func TestCollectNodeDetectionsWithoutPort(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-no-port"}}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gfd-no-port",
			Namespace: bootstrapmeta.WorkloadsNamespace,
			Labels:    map[string]string{"app": bootstrapmeta.AppName(bootstrapmeta.ComponentGPUFeatureDiscovery)},
		},
		Spec: corev1.PodSpec{
			NodeName: node.Name,
			Containers: []corev1.Container{{
				Name: "gfd-extender",
			}},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			PodIP: "127.0.0.1",
			Conditions: []corev1.PodCondition{{
				Type:   corev1.PodReady,
				Status: corev1.ConditionTrue,
			}},
		},
	}

	rec := &Reconciler{client: newTestClient(newTestScheme(t), node, pod)}
	if detections, err := rec.collectNodeDetections(context.Background(), node.Name); err != nil {
		t.Fatalf("unexpected error when gfd-extender port is missing: %v", err)
	} else if len(detections.byIndex) != 0 || len(detections.byUUID) != 0 {
		t.Fatalf("expected empty detections when port is missing, got %+v", detections)
	}
}

func TestCollectNodeDetectionsSuccessIndexFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"index":1,"memoryInfo":{"Total":1,"Free":1,"Used":0},"powerUsage":10,"utilization":{"Gpu":1,"Memory":1}}]`))
	}))
	defer server.Close()

	host, portStr, _ := strings.Cut(server.Listener.Addr().String(), ":")
	port, _ := strconv.Atoi(portStr)

	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-detect-success"}}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gfd-ok",
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
	rec := &Reconciler{client: newTestClient(newTestScheme(t), node, pod)}
	orig := detectHTTPClient
	detectHTTPClient = server.Client()
	defer func() { detectHTTPClient = orig }()

	detections, err := rec.collectNodeDetections(context.Background(), node.Name)
	if err != nil {
		t.Fatalf("expected successful detection: %v", err)
	}
	entry, ok := detections.byIndex["1"]
	if !ok || entry.Index != 1 || entry.PowerUsage != 10 {
		t.Fatalf("unexpected detection entry: %+v ok=%v", entry, ok)
	}
}

func TestCollectNodeDetectionsSkipsNonReadyAndOtherNode(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-skip"}}
	otherNodePod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gfd-other",
			Namespace: bootstrapmeta.WorkloadsNamespace,
			Labels:    map[string]string{"app": bootstrapmeta.AppName(bootstrapmeta.ComponentGPUFeatureDiscovery)},
		},
		Spec: corev1.PodSpec{
			NodeName: "other-node",
			Containers: []corev1.Container{{
				Name:  "gfd-extender",
				Ports: []corev1.ContainerPort{{ContainerPort: 1234}},
			}},
		},
		Status: corev1.PodStatus{
			Phase:      corev1.PodRunning,
			PodIP:      "127.0.0.2",
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
		},
	}
	notReadyPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gfd-notready",
			Namespace: bootstrapmeta.WorkloadsNamespace,
			Labels:    map[string]string{"app": bootstrapmeta.AppName(bootstrapmeta.ComponentGPUFeatureDiscovery)},
		},
		Spec: corev1.PodSpec{
			NodeName: node.Name,
			Containers: []corev1.Container{{
				Name:  "gfd-extender",
				Ports: []corev1.ContainerPort{{ContainerPort: 1234}},
			}},
		},
		Status: corev1.PodStatus{
			Phase:      corev1.PodPending,
			PodIP:      "127.0.0.1",
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionFalse}},
		},
	}

	rec := &Reconciler{client: newTestClient(newTestScheme(t), node, otherNodePod, notReadyPod)}
	detections, err := rec.collectNodeDetections(context.Background(), node.Name)
	if err != nil {
		t.Fatalf("unexpected error when pods are non-ready/other-node: %v", err)
	}
	if len(detections.byIndex) != 0 && len(detections.byUUID) != 0 {
		t.Fatalf("expected empty detections, got %+v", detections)
	}
}

type failingRoundTripper struct{}

func (f failingRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("rt error")
}

func TestCollectNodeDetectionsDoError(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-do-error"}}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gfd-do-error",
			Namespace: bootstrapmeta.WorkloadsNamespace,
			Labels:    map[string]string{"app": bootstrapmeta.AppName(bootstrapmeta.ComponentGPUFeatureDiscovery)},
		},
		Spec: corev1.PodSpec{
			NodeName: node.Name,
			Containers: []corev1.Container{{
				Name:  "gfd-extender",
				Ports: []corev1.ContainerPort{{ContainerPort: 1234}},
			}},
		},
		Status: corev1.PodStatus{
			Phase:      corev1.PodRunning,
			PodIP:      "127.0.0.1",
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
		},
	}

	rec := &Reconciler{client: newTestClient(newTestScheme(t), node, pod)}
	orig := detectHTTPClient
	detectHTTPClient = &http.Client{Transport: failingRoundTripper{}}
	defer func() { detectHTTPClient = orig }()

	detections, err := rec.collectNodeDetections(context.Background(), node.Name)
	if err != nil {
		t.Fatalf("unexpected error on HTTP failure: %v", err)
	}
	if len(detections.byIndex) != 0 || len(detections.byUUID) != 0 {
		t.Fatalf("expected empty detections on HTTP failure, got %+v", detections)
	}
}

func TestCollectNodeDetectionsBadURL(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-bad-url"}}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gfd-bad-url",
			Namespace: bootstrapmeta.WorkloadsNamespace,
			Labels:    map[string]string{"app": bootstrapmeta.AppName(bootstrapmeta.ComponentGPUFeatureDiscovery)},
		},
		Spec: corev1.PodSpec{
			NodeName: node.Name,
			Containers: []corev1.Container{{
				Name:  "gfd-extender",
				Ports: []corev1.ContainerPort{{ContainerPort: 1234}},
			}},
		},
		Status: corev1.PodStatus{
			Phase:      corev1.PodRunning,
			PodIP:      "bad host\n",
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
		},
	}

	rec := &Reconciler{client: newTestClient(newTestScheme(t), node, pod)}
	detections, err := rec.collectNodeDetections(context.Background(), node.Name)
	if err != nil {
		t.Fatalf("unexpected error on bad URL: %v", err)
	}
	if len(detections.byIndex) != 0 || len(detections.byUUID) != 0 {
		t.Fatalf("expected empty detections on bad URL, got %+v", detections)
	}
}

func TestIsPodReadyFalseCases(t *testing.T) {
	pod := &corev1.Pod{
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
		},
	}
	if isPodReady(pod) {
		t.Fatalf("pending pod should not be ready")
	}
	pod.Status.Phase = corev1.PodRunning
	pod.Status.PodIP = "1.2.3.4"
	if isPodReady(pod) {
		t.Fatalf("pod without ready condition should not be ready")
	}
	pod.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionFalse}}
	if isPodReady(pod) {
		t.Fatalf("pod with ready=false should not be ready")
	}
}

func detectionPtrInt32(v int32) *int32 { return &v }
