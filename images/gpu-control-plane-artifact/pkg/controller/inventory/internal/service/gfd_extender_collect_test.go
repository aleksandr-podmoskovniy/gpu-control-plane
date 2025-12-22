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

package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	common "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/common"
)

func TestCollectNodeDetectionsMissingPodIsSilent(t *testing.T) {
	scheme := newTestScheme(t)
	collector := NewDetectionCollector(newTestClient(t, scheme))

	detections, err := collector.Collect(context.Background(), "node-no-pod")
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
			Namespace: common.WorkloadsNamespace,
			Labels:    map[string]string{"app": common.AppName(common.ComponentGPUFeatureDiscovery)},
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

	scheme := newTestScheme(t)
	collector := NewDetectionCollector(newTestClient(t, scheme, node, pod))

	orig := detectHTTPClient
	detectHTTPClient = server.Client()
	defer func() { detectHTTPClient = orig }()

	if detections, err := collector.Collect(context.Background(), node.Name); err != nil {
		t.Fatalf("unexpected error when gfd-extender returns non-200: %v", err)
	} else if len(detections.byIndex) != 0 || len(detections.byUUID) != 0 {
		t.Fatalf("expected empty detections on non-200, got %+v", detections)
	}
}

func TestCollectNodeDetectionsStoresEntriesByUUID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"index":0,"uuid":"GPU-uuid-1"}]`))
	}))
	defer server.Close()

	host, portStr, _ := strings.Cut(server.Listener.Addr().String(), ":")
	port, _ := strconv.Atoi(portStr)

	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-uuid"}}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gfd-pod-uuid",
			Namespace: common.WorkloadsNamespace,
			Labels:    map[string]string{"app": common.AppName(common.ComponentGPUFeatureDiscovery)},
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

	scheme := newTestScheme(t)
	collector := NewDetectionCollector(newTestClient(t, scheme, node, pod))

	orig := detectHTTPClient
	detectHTTPClient = server.Client()
	defer func() { detectHTTPClient = orig }()

	detections, err := collector.Collect(context.Background(), node.Name)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := detections.byUUID["GPU-uuid-1"]; !ok {
		t.Fatalf("expected entry to be stored by uuid, got %+v", detections.byUUID)
	}
	if _, ok := detections.byIndex["0"]; !ok {
		t.Fatalf("expected entry to be stored by index, got %+v", detections.byIndex)
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
			Namespace: common.WorkloadsNamespace,
			Labels:    map[string]string{"app": common.AppName(common.ComponentGPUFeatureDiscovery)},
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

	scheme := newTestScheme(t)
	collector := NewDetectionCollector(newTestClient(t, scheme, node, pod))

	orig := detectHTTPClient
	detectHTTPClient = server.Client()
	defer func() { detectHTTPClient = orig }()

	if _, err := collector.Collect(context.Background(), node.Name); err == nil {
		t.Fatalf("expected error when gfd-extender returns malformed json")
	}
}

func TestCollectNodeDetectionsWithoutPort(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-no-port"}}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gfd-no-port",
			Namespace: common.WorkloadsNamespace,
			Labels:    map[string]string{"app": common.AppName(common.ComponentGPUFeatureDiscovery)},
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

	scheme := newTestScheme(t)
	collector := NewDetectionCollector(newTestClient(t, scheme, node, pod))

	if detections, err := collector.Collect(context.Background(), node.Name); err != nil {
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
			Namespace: common.WorkloadsNamespace,
			Labels:    map[string]string{"app": common.AppName(common.ComponentGPUFeatureDiscovery)},
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

	scheme := newTestScheme(t)
	collector := NewDetectionCollector(newTestClient(t, scheme, node, pod))

	orig := detectHTTPClient
	detectHTTPClient = server.Client()
	defer func() { detectHTTPClient = orig }()

	detections, err := collector.Collect(context.Background(), node.Name)
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
			Namespace: common.WorkloadsNamespace,
			Labels:    map[string]string{"app": common.AppName(common.ComponentGPUFeatureDiscovery)},
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
			Namespace: common.WorkloadsNamespace,
			Labels:    map[string]string{"app": common.AppName(common.ComponentGPUFeatureDiscovery)},
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

	scheme := newTestScheme(t)
	collector := NewDetectionCollector(newTestClient(t, scheme, node, otherNodePod, notReadyPod))

	detections, err := collector.Collect(context.Background(), node.Name)
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
			Namespace: common.WorkloadsNamespace,
			Labels:    map[string]string{"app": common.AppName(common.ComponentGPUFeatureDiscovery)},
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

	scheme := newTestScheme(t)
	collector := NewDetectionCollector(newTestClient(t, scheme, node, pod))

	orig := detectHTTPClient
	detectHTTPClient = &http.Client{Transport: failingRoundTripper{}}
	defer func() { detectHTTPClient = orig }()

	detections, err := collector.Collect(context.Background(), node.Name)
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
			Namespace: common.WorkloadsNamespace,
			Labels:    map[string]string{"app": common.AppName(common.ComponentGPUFeatureDiscovery)},
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

	scheme := newTestScheme(t)
	collector := NewDetectionCollector(newTestClient(t, scheme, node, pod))

	detections, err := collector.Collect(context.Background(), node.Name)
	if err != nil {
		t.Fatalf("unexpected error on bad URL: %v", err)
	}
	if len(detections.byIndex) != 0 || len(detections.byUUID) != 0 {
		t.Fatalf("expected empty detections on bad URL, got %+v", detections)
	}
}
