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
	"io"
	"net/http"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/bootstrap/meta"
)

func TestCollectNodeTelemetryListErrorExtra(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	cli := &failingListClient{Client: fake.NewClientBuilder().WithScheme(scheme).Build(), err: errors.New("list failed")}
	r := &Reconciler{client: cli}
	if _, err := r.collectNodeTelemetry(context.Background(), "node1"); err == nil {
		t.Fatalf("expected list error")
	}
}

func TestCollectNodeTelemetryNoExporterExtra(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	// pod on another node -> exporter stays nil
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: meta.WorkloadsNamespace,
			Labels:    map[string]string{"app": meta.AppName(meta.ComponentDCGMExporter)},
		},
		Spec: corev1.PodSpec{NodeName: "other"},
	}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod).Build()
	r := &Reconciler{client: cli}
	telemetry, err := r.collectNodeTelemetry(context.Background(), "node1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(telemetry.byUUID) != 0 || len(telemetry.byIndex) != 0 {
		t.Fatalf("expected empty telemetry when exporter not found")
	}
}

func TestCollectNodeTelemetryScrapeErrorExtra(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: meta.WorkloadsNamespace,
			Labels:    map[string]string{"app": meta.AppName(meta.ComponentDCGMExporter)},
		},
		Spec: corev1.PodSpec{NodeName: "node1"},
		Status: corev1.PodStatus{
			PodIP: "127.0.0.1",
			Conditions: []corev1.PodCondition{{
				Type:   corev1.PodReady,
				Status: corev1.ConditionTrue,
			}},
		},
	}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod).Build()
	r := &Reconciler{client: cli}

	origClient := telemetryHTTPClient
	defer func() { telemetryHTTPClient = origClient }()
	telemetryHTTPClient = &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("boom")
	})}

	telemetry, err := r.collectNodeTelemetry(context.Background(), "node1")
	if err != nil {
		t.Fatalf("collectNodeTelemetry should swallow scrape errors, got %v", err)
	}
	if len(telemetry.byUUID) != 0 || len(telemetry.byIndex) != 0 {
		t.Fatalf("expected empty telemetry on scrape error")
	}
}

func TestCollectNodeTelemetryNonOKResponseExtra(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: meta.WorkloadsNamespace,
			Labels:    map[string]string{"app": meta.AppName(meta.ComponentDCGMExporter)},
		},
		Spec: corev1.PodSpec{NodeName: "node1"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			PodIP: "127.0.0.1",
			Conditions: []corev1.PodCondition{{
				Type:   corev1.PodReady,
				Status: corev1.ConditionTrue,
			}},
		},
	}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod).Build()
	r := &Reconciler{client: cli}

	origClient := telemetryHTTPClient
	defer func() { telemetryHTTPClient = origClient }()
	telemetryHTTPClient = &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusInternalServerError, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}

	telemetry, err := r.collectNodeTelemetry(context.Background(), "node1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(telemetry.byUUID) != 0 {
		t.Fatalf("expected empty telemetry on non-200 response")
	}
}

func TestCollectNodeTelemetryParseErrorExtra(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: meta.WorkloadsNamespace,
			Labels:    map[string]string{"app": meta.AppName(meta.ComponentDCGMExporter)},
		},
		Spec: corev1.PodSpec{NodeName: "node1"},
		Status: corev1.PodStatus{
			Phase:      corev1.PodRunning,
			PodIP:      "127.0.0.1",
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
		},
	}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod).Build()
	r := &Reconciler{client: cli}

	origClient := telemetryHTTPClient
	defer func() { telemetryHTTPClient = origClient }()
	telemetryHTTPClient = &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(errorReader{err: errors.New("read fail")}),
		}, nil
	})}

	telemetry, err := r.collectNodeTelemetry(context.Background(), "node1")
	if err != nil {
		t.Fatalf("expected nil error despite parse failure, got %v", err)
	}
	if len(telemetry.byUUID) != 0 {
		t.Fatalf("expected empty telemetry on parse error")
	}
}

func TestCollectNodeTelemetryPodNotReadyExtra(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: meta.WorkloadsNamespace,
			Labels:    map[string]string{"app": meta.AppName(meta.ComponentDCGMExporter)},
		},
		Spec: corev1.PodSpec{NodeName: "node1"},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			PodIP: "127.0.0.1",
		},
	}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod).Build()
	r := &Reconciler{client: cli}
	telemetry, err := r.collectNodeTelemetry(context.Background(), "node1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(telemetry.byUUID) != 0 || len(telemetry.byIndex) != 0 {
		t.Fatalf("expected no telemetry for not ready pod")
	}
}

func TestCollectNodeTelemetrySuccessExtra(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: meta.WorkloadsNamespace,
			Labels:    map[string]string{"app": meta.AppName(meta.ComponentDCGMExporter)},
		},
		Spec: corev1.PodSpec{
			NodeName: "node1",
			Containers: []corev1.Container{{
				Name:  "dcgm-exporter",
				Ports: []corev1.ContainerPort{{ContainerPort: 9400}},
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
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod).Build()
	r := &Reconciler{client: cli}

	origClient := telemetryHTTPClient
	defer func() { telemetryHTTPClient = origClient }()

	metrics := `
dcgm_exporter_last_update_time_seconds 1
DCGM_FI_DEV_GPU_TEMP{uuid="GPU-AAA",gpu="0"} 55
`
	telemetryHTTPClient = &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(metrics)),
		}, nil
	})}

	telemetry, err := r.collectNodeTelemetry(context.Background(), "node1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(telemetry.byUUID) != 1 {
		t.Fatalf("expected telemetry populated, got %+v", telemetry)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

type errorReader struct {
	err error
}

func (r errorReader) Read([]byte) (int, error) {
	return 0, r.err
}
