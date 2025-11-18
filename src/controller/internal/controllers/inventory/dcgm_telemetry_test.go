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
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
)

func TestParseExporterMetricsAndApplyTelemetry(t *testing.T) {
	input := `
DCGM_FI_DEV_GPU_TEMP{gpu="0",uuid="GPU-AAA"} 55
DCGM_FI_DEV_ECC_DBE_AGG_TOTAL{gpu="0"} 3
dcgm_exporter_last_update_time_seconds 1700000000
`
	telemetry, err := parseExporterMetrics(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parseExporterMetrics returned error: %v", err)
	}

	snapshot := deviceSnapshot{Index: "0", UUID: "GPU-AAA"}
	tp, ok := telemetry.find(snapshot)
	if !ok {
		t.Fatalf("expected telemetry entry for snapshot %+v", snapshot)
	}
	if tp.temperatureC == nil || *tp.temperatureC != 55 {
		t.Fatalf("unexpected temperature: %+v", tp.temperatureC)
	}
	if tp.eccTotal == nil || *tp.eccTotal != 3 {
		t.Fatalf("unexpected ecc total: %+v", tp.eccTotal)
	}
	expectedTS := time.Unix(1700000000, 0).UTC()
	if !tp.lastUpdated.Equal(expectedTS) {
		t.Fatalf("unexpected timestamp: %v", tp.lastUpdated)
	}

	device := &v1alpha1.GPUDevice{}
	applyTelemetry(device, snapshot, telemetry)
	if device.Status.Health.TemperatureC != 55 {
		t.Fatalf("device temperature not applied: %d", device.Status.Health.TemperatureC)
	}
	if device.Status.Health.ECCErrorsTotal != 3 {
		t.Fatalf("device ecc not applied: %d", device.Status.Health.ECCErrorsTotal)
	}
	if device.Status.Health.LastUpdatedTime == nil || !device.Status.Health.LastUpdatedTime.Time.Equal(expectedTS) {
		t.Fatalf("device timestamp not applied: %v", device.Status.Health.LastUpdatedTime)
	}

	telemetry = nodeTelemetry{byIndex: map[string]telemetryPoint{}, byUUID: map[string]telemetryPoint{}}
	storeTelemetry(telemetry, map[string]string{"gpu": "1"}, func(tp *telemetryPoint) { v := int32(10); tp.temperatureC = &v })
	if _, ok := telemetry.byIndex["1"]; !ok {
		t.Fatalf("expected telemetry stored by gpu index")
	}
}

func TestParseMetricLineInvalid(t *testing.T) {
	if _, _, _, ok := parseMetricLine("dcgm_exporter_last_update_time_seconds abc"); ok {
		t.Fatalf("expected parseMetricLine to fail on invalid number")
	}
	if _, _, _, ok := parseMetricLine("# comment line"); ok {
		t.Fatalf("expected comment lines to be skipped")
	}
	if _, _, _, ok := parseMetricLine("dcgm_exporter_last_update_time_seconds NaN"); ok {
		t.Fatalf("expected parseMetricLine to fail on NaN")
	}
	if _, _, _, ok := parseMetricLine("metric_without_value"); ok {
		t.Fatalf("expected parseMetricLine to fail on missing value")
	}
	if _, _, _, ok := parseMetricLine("dcgm_exporter_last_update_time_seconds +Inf"); ok {
		t.Fatalf("expected parseMetricLine to fail on Inf")
	}
	name, labels, value, ok := parseMetricLine("DCGM_FI_DEV_GPU_TEMP{gpu=\"0\",uuid=\"UUID\"} 12")
	if !ok || name != "DCGM_FI_DEV_GPU_TEMP" || labels["gpu"] != "0" || labels["uuid"] != "UUID" || value != 12 {
		t.Fatalf("expected parsed metric with labels, got %s %v %f ok=%v", name, labels, value, ok)
	}
	if name, labels, value, ok := parseMetricLine("metric_without_labels 5"); !ok || name != "metric_without_labels" || len(labels) != 0 || value != 5 {
		t.Fatalf("expected metric without labels to parse, got %s %v %f ok=%v", name, labels, value, ok)
	}
}

func TestStoreTelemetryNilLabels(t *testing.T) {
	telemetry := nodeTelemetry{byIndex: map[string]telemetryPoint{}, byUUID: map[string]telemetryPoint{}}
	storeTelemetry(telemetry, nil, func(tp *telemetryPoint) { v := int32(1); tp.temperatureC = &v })
	if len(telemetry.byIndex) != 0 || len(telemetry.byUUID) != 0 {
		t.Fatalf("expected no telemetry stored for nil labels")
	}
}

func TestTelemetryFindFalse(t *testing.T) {
	telemetry := nodeTelemetry{byIndex: map[string]telemetryPoint{}, byUUID: map[string]telemetryPoint{}}
	if _, ok := telemetry.find(deviceSnapshot{Index: "5"}); ok {
		t.Fatalf("expected find to return false for empty telemetry")
	}

	telemetry.byIndex["0"] = telemetryPoint{temperatureC: ptrInt32(1)}
	telemetry.byUUID["UUID"] = telemetryPoint{eccTotal: ptrInt64(2)}
	tp, ok := telemetry.find(deviceSnapshot{Index: "0", UUID: "UUID"})
	if !ok || tp.temperatureC == nil || tp.eccTotal == nil {
		t.Fatalf("expected merged telemetry on uuid/index match, got %+v", tp)
	}
}

func TestScrapeExporterMetricsErrorPaths(t *testing.T) {
	pod := &corev1.Pod{}
	if _, err := scrapeExporterMetrics(context.Background(), pod); err == nil {
		t.Fatalf("expected error when pod IP is empty")
	}

	badURL := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "dcgm-exporter", Ports: []corev1.ContainerPort{{ContainerPort: 9400}}}},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			PodIP: "bad host",
			Conditions: []corev1.PodCondition{{
				Type:   corev1.PodReady,
				Status: corev1.ConditionTrue,
			}},
		},
	}
	if _, err := scrapeExporterMetrics(context.Background(), badURL); err == nil {
		t.Fatalf("expected error for invalid request URL")
	}
}

func TestCollectNodeTelemetryExporterMissing(t *testing.T) {
	scheme := newTestScheme(t)
	client := newTestClient(scheme)
	rec := &Reconciler{client: client}

	if _, err := rec.collectNodeTelemetry(context.Background(), "node-1"); err == nil {
		t.Fatalf("expected error when exporter pod is missing")
	}
}

func TestParseMetricLineHandlesMalformedLabels(t *testing.T) {
	name, labels, value, ok := parseMetricLine("metric{broken 5")
	if !ok || name != "metric{broken" || len(labels) != 0 || value != 5 {
		t.Fatalf("expected parser to ignore malformed label section, got %s %v %v ok=%v", name, labels, value, ok)
	}

	name, labels, value, ok = parseMetricLine("metric{gpu=\"0\",bad} 7")
	if !ok || name != "metric" || labels["gpu"] != "0" || value != 7 {
		t.Fatalf("expected malformed label entry to be skipped, got %s %v %v ok=%v", name, labels, value, ok)
	}
}

func ptrInt32(v int32) *int32 { return &v }
func ptrInt64(v int64) *int64 { return &v }
