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
	"reflect"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
	if device.Status.Health.LastError != "" {
		t.Fatalf("did not expect initial health error, got %s", device.Status.Health.LastError)
	}

	// Next sample increases ECC counter and should trigger a fault.
	input = `
DCGM_FI_DEV_ECC_DBE_AGG_TOTAL{gpu="0"} 5
dcgm_exporter_last_update_time_seconds 1700000010
`
	tlm, err := parseExporterMetrics(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parseExporterMetrics returned error: %v", err)
	}
	applyTelemetry(device, snapshot, tlm)
	if device.Status.Health.LastErrorReason != "ECCDoubleBitError" {
		t.Fatalf("expected ECC fault, got reason=%s message=%s", device.Status.Health.LastErrorReason, device.Status.Health.LastError)
	}
	if device.Status.Health.ConsecutiveHealthy != 0 {
		t.Fatalf("expected healthy counter reset on fault, got %d", device.Status.Health.ConsecutiveHealthy)
	}

	// Provide healthy samples to clear the fault.
	for i := 0; i < deviceHealthRecoveryThreshold; i++ {
		input = fmt.Sprintf("DCGM_FI_DEV_ECC_DBE_AGG_TOTAL{gpu=\"0\"} 5\ndcgm_exporter_last_update_time_seconds %d\n", 1700000020+i)
		healthy, err := parseExporterMetrics(strings.NewReader(input))
		if err != nil {
			t.Fatalf("parseExporterMetrics returned error: %v", err)
		}
		applyTelemetry(device, snapshot, healthy)
	}
	if device.Status.Health.LastError != "" {
		t.Fatalf("expected health error cleared after stable period, got %s", device.Status.Health.LastError)
	}

	// XID errors should immediately trigger a new fault.
	input = `
DCGM_FI_DEV_XID_ERRORS{uuid="GPU-AAA"} 31
dcgm_exporter_last_update_time_seconds 1700000100
`
	xidTelemetry, err := parseExporterMetrics(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parseExporterMetrics returned error: %v", err)
	}
	applyTelemetry(device, snapshot, xidTelemetry)
	if device.Status.Health.LastErrorReason != "XIDError" {
		t.Fatalf("expected XID fault, got reason=%s message=%s", device.Status.Health.LastErrorReason, device.Status.Health.LastError)
	}

	// Power violation should also trigger a fault via monotonic metric.
	// Use a fresh device to assess power violation fault via monotonic metric.
	powerDevice := &v1alpha1.GPUDevice{}
	powerDevice.Status.Health.LastUpdatedTime = &metav1.Time{Time: time.Unix(1700000105, 0)}
	setHealthMetricInt(&powerDevice.Status.Health, metricKeyPowerViolations, 0)

	input = `
DCGM_FI_DEV_POWER_VIOLATION{gpu="0"} 1
dcgm_exporter_last_update_time_seconds 1700000110
`
	powerTlm, err := parseExporterMetrics(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parseExporterMetrics returned error: %v", err)
	}
	powerTP, ok := powerTlm.find(snapshot)
	if !ok || powerTP.powerViolations == nil || *powerTP.powerViolations != 1 {
		t.Fatalf("expected power violation telemetry stored, got %+v", powerTP)
	}
	applyTelemetry(powerDevice, snapshot, powerTlm)
	if powerDevice.Status.Health.LastErrorReason != "PowerViolation" {
		t.Fatalf("expected power violation fault, got %s", powerDevice.Status.Health.LastErrorReason)
	}

	// If there is no telemetry or DCGM disabled, health stays as-is.
	prev := device.Status.Health
	applyTelemetry(device, deviceSnapshot{Index: "missing"}, nodeTelemetry{})
	if device.Status.Health.LastErrorReason != prev.LastErrorReason ||
		device.Status.Health.LastError != prev.LastError ||
		device.Status.Health.ConsecutiveHealthy != prev.ConsecutiveHealthy ||
		!reflect.DeepEqual(device.Status.Health.Metrics, prev.Metrics) {
		t.Fatalf("expected health untouched without telemetry, got %+v", device.Status.Health)
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

func TestUpdateMonotonicMetricAndHealthHelpers(t *testing.T) {
	health := &v1alpha1.GPUDeviceHealth{}
	val := int64(5)
	if updateMonotonicMetric(health, "metric", &val, true) {
		t.Fatalf("initial sample should not trigger violation")
	}
	if got := health.Metrics["metric"]; got != "5" {
		t.Fatalf("metric should be stored on initial sample, got %s", got)
	}

	higher := int64(7)
	if !updateMonotonicMetric(health, "metric", &higher, false) {
		t.Fatalf("increase should trigger violation")
	}
	if getHealthMetricInt(health, "metric") != 7 {
		t.Fatalf("metric should be updated to 7, got %d", getHealthMetricInt(health, "metric"))
	}

	lower := int64(6)
	if updateMonotonicMetric(health, "metric", &lower, false) {
		t.Fatalf("lower value should not trigger violation")
	}
	health.Metrics["bad"] = "NaN"
	if getHealthMetricInt(health, "bad") != 0 {
		t.Fatalf("non-numeric metric should be parsed as 0")
	}

	if updateMonotonicMetric(health, "metric", nil, false) {
		t.Fatalf("nil metric should not trigger violation")
	}
	if _, ok := health.Metrics["metric"]; ok {
		t.Fatalf("metric key should be removed when value is nil")
	}
}

func TestApplyTelemetryThermalAndReliabilityViolations(t *testing.T) {
	now := time.Unix(1700000200, 0)
	snapshot := deviceSnapshot{Index: "0"}

	thermalOnly := nodeTelemetry{
		byIndex: map[string]telemetryPoint{
			"0": {thermalViolations: ptrInt64(1), lastUpdated: now},
		},
		byUUID: map[string]telemetryPoint{},
	}
	device := &v1alpha1.GPUDevice{}
	device.Status.Health.LastUpdatedTime = &metav1.Time{Time: now.Add(-time.Minute)}
	applyTelemetry(device, snapshot, thermalOnly)
	if device.Status.Health.LastErrorReason != "ThermalViolation" {
		t.Fatalf("expected thermal violation fault, got %s", device.Status.Health.LastErrorReason)
	}

	reliabilityOnly := nodeTelemetry{
		byIndex: map[string]telemetryPoint{
			"0": {reliabilityViolations: ptrInt64(2), lastUpdated: now},
		},
	}
	reliabilityDevice := &v1alpha1.GPUDevice{}
	reliabilityDevice.Status.Health.LastUpdatedTime = &metav1.Time{Time: now.Add(-time.Minute)}
	applyTelemetry(reliabilityDevice, snapshot, reliabilityOnly)
	if reliabilityDevice.Status.Health.LastErrorReason != "ReliabilityViolation" {
		t.Fatalf("expected reliability violation fault, got %s", reliabilityDevice.Status.Health.LastErrorReason)
	}

	trackDeviceFault(&reliabilityDevice.Status.Health, "Manual", "manual fault", time.Time{})
	if reliabilityDevice.Status.Health.LastErrorTime != nil {
		t.Fatalf("expected LastErrorTime nil when timestamp is zero")
	}
}

func TestApplyTelemetryClearsMetricsOnHealthySample(t *testing.T) {
	now := time.Unix(1700000300, 0)
	device := &v1alpha1.GPUDevice{
		Status: v1alpha1.GPUDeviceStatus{
			Health: v1alpha1.GPUDeviceHealth{
				LastError:          "fault",
				LastErrorReason:    "XIDError",
				LastErrorTime:      &metav1.Time{Time: now.Add(-time.Minute)},
				LastUpdatedTime:    &metav1.Time{Time: now.Add(-2 * time.Minute)},
				ConsecutiveHealthy: deviceHealthRecoveryThreshold - 1,
				Metrics: map[string]string{
					metricKeyXIDCode:         "31",
					metricKeyPowerViolations: "2",
				},
			},
		},
	}
	telemetry := nodeTelemetry{
		byIndex: map[string]telemetryPoint{
			"0": {lastUpdated: now},
		},
	}
	applyTelemetry(device, deviceSnapshot{Index: "0"}, telemetry)
	if device.Status.Health.LastError != "" || device.Status.Health.LastErrorReason != "" {
		t.Fatalf("expected health error cleared on healthy sample, got %s %s", device.Status.Health.LastErrorReason, device.Status.Health.LastError)
	}
	if _, ok := device.Status.Health.Metrics[metricKeyPowerViolations]; ok {
		t.Fatalf("expected power violation metric cleared")
	}
	if device.Status.Health.LastUpdatedTime == nil || device.Status.Health.LastUpdatedTime.Time.IsZero() {
		t.Fatalf("expected last updated timestamp set")
	}
}

func TestApplyTelemetryClearsXIDMetricWhenZero(t *testing.T) {
	now := time.Unix(1700000400, 0)
	device := &v1alpha1.GPUDevice{
		Status: v1alpha1.GPUDeviceStatus{
			Health: v1alpha1.GPUDeviceHealth{
				Metrics: map[string]string{metricKeyXIDCode: "30"},
			},
		},
	}
	telemetry := nodeTelemetry{
		byUUID: map[string]telemetryPoint{
			"GPU-AAA": {xidCode: ptrInt64(0), lastUpdated: now},
		},
	}
	applyTelemetry(device, deviceSnapshot{UUID: "GPU-AAA"}, telemetry)
	if _, ok := device.Status.Health.Metrics[metricKeyXIDCode]; ok {
		t.Fatalf("expected xid metric cleared when value is zero")
	}
	if device.Status.Health.LastUpdatedTime == nil || device.Status.Health.LastUpdatedTime.Time.IsZero() {
		t.Fatalf("expected updated timestamp after xid clear")
	}

	// And when XID value changes, we pass through previous metric branch.
	telemetryChange := nodeTelemetry{
		byUUID: map[string]telemetryPoint{
			"GPU-AAA": {xidCode: ptrInt64(31), lastUpdated: now.Add(time.Second)},
		},
	}
	applyTelemetry(device, deviceSnapshot{UUID: "GPU-AAA"}, telemetryChange)
	if device.Status.Health.LastErrorReason != "XIDError" {
		t.Fatalf("expected xid fault on code change, got %s", device.Status.Health.LastErrorReason)
	}
}

func TestParseExporterMetricsCoversAllCounters(t *testing.T) {
	input := `
DCGM_FI_DEV_THERMAL_VIOLATION{uuid="GPU-BBB"} 2
DCGM_FI_DEV_RELIABILITY_VIOLATION{gpu="1"} 3
`
	telemetry, err := parseExporterMetrics(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parseExporterMetrics returned error: %v", err)
	}
	if tp, ok := telemetry.byUUID["GPU-BBB"]; !ok || tp.thermalViolations == nil || *tp.thermalViolations != 2 || tp.lastUpdated.IsZero() {
		t.Fatalf("expected thermal violation stored with timestamp, got %+v", tp)
	}
	if tp, ok := telemetry.byIndex["1"]; !ok || tp.reliabilityViolations == nil || *tp.reliabilityViolations != 3 || tp.lastUpdated.IsZero() {
		t.Fatalf("expected reliability violation stored with timestamp, got %+v", tp)
	}
}

func ptrInt32(v int32) *int32 { return &v }
func ptrInt64(v int64) *int64 { return &v }
