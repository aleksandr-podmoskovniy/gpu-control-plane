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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	nfdv1alpha1 "sigs.k8s.io/node-feature-discovery/api/nfd/v1alpha1"
)

func TestCollectNodeTelemetrySuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `
dcgm_exporter_last_update_time_seconds 42
DCGM_FI_DEV_GPU_TEMP{gpu="0"} 60
DCGM_FI_DEV_ECC_DBE_AGG_TOTAL{uuid="GPU-123"} 7
DCGM_FI_DEV_GPU_TEMP{uuid="UUID-ONLY"} 70
dcgm_exporter_last_update_time_seconds bad
`)
	}))
	defer server.Close()

	host, portStr, _ := strings.Cut(server.Listener.Addr().String(), ":")
	port, _ := strconv.Atoi(portStr)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dcgm-exporter-test",
			Namespace: "d8-gpu-control-plane",
			Labels:    map[string]string{"app": "gpu-control-plane-dcgm-exporter"},
		},
		Spec: corev1.PodSpec{
			NodeName: "node-telemetry",
			Containers: []corev1.Container{{
				Name:  "dcgm-exporter",
				Ports: []corev1.ContainerPort{{ContainerPort: int32(port)}},
			}},
		},
		Status: corev1.PodStatus{
			Phase:  corev1.PodRunning,
			PodIP:  host,
			PodIPs: []corev1.PodIP{{IP: host}},
			Conditions: []corev1.PodCondition{{
				Type:   corev1.PodReady,
				Status: corev1.ConditionTrue,
			}},
		},
	}
	scheme := newTestScheme(t)
	reconciler := &Reconciler{client: newTestClient(scheme, pod)}

	telemetry, err := reconciler.collectNodeTelemetry(context.Background(), "node-telemetry")
	if err != nil {
		t.Fatalf("collectNodeTelemetry returned error: %v", err)
	}
	snap := deviceSnapshot{Index: "0", UUID: "GPU-123"}
	tp, ok := telemetry.find(snap)
	if !ok || tp.temperatureC == nil || *tp.temperatureC != 60 || tp.eccTotal == nil || *tp.eccTotal != 7 {
		t.Fatalf("expected telemetry applied, got %+v", tp)
	}
}

func TestCollectNodeTelemetryNoExporter(t *testing.T) {
	scheme := newTestScheme(t)
	reconciler := &Reconciler{client: newTestClient(scheme)}

	if _, err := reconciler.collectNodeTelemetry(context.Background(), "node-missing"); err == nil {
		t.Fatalf("expected error when exporter pod missing")
	}

	notReady := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dcgm-exporter-notready",
			Namespace: "d8-gpu-control-plane",
			Labels:    map[string]string{"app": "gpu-control-plane-dcgm-exporter"},
		},
		Spec: corev1.PodSpec{NodeName: "node-notready"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			PodIP: "127.0.0.1",
			Conditions: []corev1.PodCondition{{
				Type:   corev1.PodReady,
				Status: corev1.ConditionFalse,
			}},
		},
	}
	reconciler.client = newTestClient(scheme, notReady)
	if _, err := reconciler.collectNodeTelemetry(context.Background(), "node-notready"); err == nil {
		t.Fatalf("expected error when exporter pod not ready")
	}

	reconciler.client = &failingListClient{err: errors.New("list pods fail")}
	if _, err := reconciler.collectNodeTelemetry(context.Background(), "node-any"); err == nil {
		t.Fatalf("expected error when pod list fails")
	}
}

func TestCollectNodeTelemetryScrapeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	host, portStr, _ := strings.Cut(server.Listener.Addr().String(), ":")
	port, _ := strconv.Atoi(portStr)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dcgm-exporter-fail",
			Namespace: "d8-gpu-control-plane",
			Labels:    map[string]string{"app": "gpu-control-plane-dcgm-exporter"},
		},
		Spec: corev1.PodSpec{
			NodeName: "node-scrape-error",
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

	scheme := newTestScheme(t)
	reconciler := &Reconciler{client: newTestClient(scheme, pod)}
	origClient := telemetryHTTPClient
	telemetryHTTPClient = server.Client()
	defer func() { telemetryHTTPClient = origClient }()

	if _, err := reconciler.collectNodeTelemetry(context.Background(), "node-scrape-error"); err == nil {
		t.Fatalf("expected scrape error to propagate")
	}
}

func TestScrapeExporterMetricsErrors(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "bad-pod"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			PodIP: "127.0.0.1",
			Conditions: []corev1.PodCondition{{
				Type:   corev1.PodReady,
				Status: corev1.ConditionTrue,
			}},
		},
	}
	if _, err := scrapeExporterMetrics(context.Background(), pod); err == nil {
		t.Fatalf("expected error due to missing endpoint")
	}

	noIP := &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodRunning}}
	if _, err := scrapeExporterMetrics(context.Background(), noIP); err == nil {
		t.Fatalf("expected error when pod has no IP")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "DCGM_FI_DEV_GPU_TEMP 1")
	}))
	defer server.Close()
	host, portStr, _ := strings.Cut(server.Listener.Addr().String(), ":")
	port, _ := strconv.Atoi(portStr)
	pod.Status.PodIP = host
	pod.Spec.Containers = []corev1.Container{{Name: "dcgm-exporter", Ports: []corev1.ContainerPort{{ContainerPort: int32(port)}}}}
	if telemetry, err := scrapeExporterMetrics(context.Background(), pod); err != nil {
		t.Fatalf("unexpected error when heartbeat missing: %v", err)
	} else if len(telemetry.byIndex) != 0 && len(telemetry.byUUID) != 0 {
		t.Fatalf("expected empty telemetry when heartbeat missing, got %+v", telemetry)
	}

	badHeartbeat := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "dcgm_exporter_last_update_time_seconds NaN\n")
	}))
	defer badHeartbeat.Close()
	host, portStr, _ = strings.Cut(badHeartbeat.Listener.Addr().String(), ":")
	port, _ = strconv.Atoi(portStr)
	pod.Status.PodIP = host
	pod.Spec.Containers[0].Ports = []corev1.ContainerPort{{ContainerPort: int32(port)}}
	if telemetry, err := scrapeExporterMetrics(context.Background(), pod); err != nil {
		t.Fatalf("expected telemetry even with invalid heartbeat: %v", err)
	} else if len(telemetry.byIndex) != 0 || len(telemetry.byUUID) != 0 {
		t.Fatalf("expected empty telemetry on invalid heartbeat, got %+v", telemetry)
	}
}

func TestScrapeExporterMetricsNonOK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	host, portStr, _ := strings.Cut(server.Listener.Addr().String(), ":")
	port, _ := strconv.Atoi(portStr)

	pod := &corev1.Pod{
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			PodIP: host,
			Conditions: []corev1.PodCondition{{
				Type:   corev1.PodReady,
				Status: corev1.ConditionTrue,
			}},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:  "dcgm-exporter",
				Ports: []corev1.ContainerPort{{ContainerPort: int32(port)}},
			}},
		},
	}
	if _, err := scrapeExporterMetrics(context.Background(), pod); err == nil {
		t.Fatalf("expected error on non-200 status")
	}
}

func TestPodReadyAndExporterPort(t *testing.T) {
	pod := &corev1.Pod{
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			PodIP: "10.0.0.1",
			Conditions: []corev1.PodCondition{{
				Type:   corev1.PodReady,
				Status: corev1.ConditionTrue,
			}},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:  "dcgm-exporter",
				Ports: []corev1.ContainerPort{{ContainerPort: 1234}},
			}},
		},
	}
	if !podReady(pod) {
		t.Fatalf("expected podReady to be true")
	}
	if exporterPort(pod) != 1234 {
		t.Fatalf("expected exporterPort=1234, got %d", exporterPort(pod))
	}

	unready := pod.DeepCopy()
	unready.Status.Conditions[0].Status = corev1.ConditionFalse
	if podReady(unready) {
		t.Fatalf("expected podReady to be false when not ready")
	}

	noPort := pod.DeepCopy()
	noPort.Spec.Containers = []corev1.Container{{Name: "dcgm-exporter"}}
	if exporterPort(noPort) != 9400 {
		t.Fatalf("expected default exporter port 9400, got %d", exporterPort(noPort))
	}

	noExporter := pod.DeepCopy()
	noExporter.Spec.Containers = []corev1.Container{{Name: "other"}}
	if exporterPort(noExporter) != 9400 {
		t.Fatalf("expected default exporter port when no exporter container, got %d", exporterPort(noExporter))
	}

	pending := pod.DeepCopy()
	pending.Status.Phase = corev1.PodPending
	if podReady(pending) {
		t.Fatalf("expected podReady false for pending pod")
	}

	noReadyCond := pod.DeepCopy()
	noReadyCond.Status.Conditions = nil
	if podReady(noReadyCond) {
		t.Fatalf("expected podReady false without Ready condition")
	}

	noIP := pod.DeepCopy()
	noIP.Status.PodIP = ""
	if podReady(noIP) {
		t.Fatalf("expected podReady false without pod IP")
	}
}

func TestNodeFeatureMappingRequiresLabels(t *testing.T) {
	feature := &nfdv1alpha1.NodeFeature{
		ObjectMeta: metav1.ObjectMeta{Name: "nf-1"},
		Spec:       nfdv1alpha1.NodeFeatureSpec{Labels: map[string]string{}},
	}
	reqs := mapNodeFeatureToNode(context.Background(), feature)
	if len(reqs) != 0 {
		t.Fatalf("expected no reconcile requests for feature without labels, got %v", reqs)
	}

	nameMissing := &nfdv1alpha1.NodeFeature{
		ObjectMeta: metav1.ObjectMeta{},
		Spec:       nfdv1alpha1.NodeFeatureSpec{Labels: map[string]string{"gpu.deckhouse.io/device.00.vendor": "10de", "gpu.deckhouse.io/device.00.device": "1db5", "gpu.deckhouse.io/device.00.class": "0300"}},
	}
	if reqs := mapNodeFeatureToNode(context.Background(), nameMissing); len(reqs) != 0 {
		t.Fatalf("expected no requests for feature without name")
	}
}
