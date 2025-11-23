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
	"bufio"
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/bootstrap/meta"
)

// nodeTelemetry keeps per-device telemetry parsed from dcgm-exporter.
type nodeTelemetry struct {
	byUUID  map[string]telemetryPoint
	byIndex map[string]telemetryPoint
}

type telemetryPoint struct {
	temperatureC          *int32
	eccTotal              *int64
	xidCode               *int64
	powerViolations       *int64
	thermalViolations     *int64
	reliabilityViolations *int64
	lastUpdated           time.Time
}

func (t nodeTelemetry) find(snapshot deviceSnapshot) (telemetryPoint, bool) {
	var (
		result    telemetryPoint
		foundUUID bool
		foundIdx  bool
	)

	if snapshot.UUID != "" {
		if tp, ok := t.byUUID[snapshot.UUID]; ok {
			result = tp
			foundUUID = true
		}
	}
	if tp, ok := t.byIndex[snapshot.Index]; ok {
		foundIdx = true
		if result.temperatureC == nil {
			result.temperatureC = tp.temperatureC
		}
		if result.eccTotal == nil {
			result.eccTotal = tp.eccTotal
		}
		if result.xidCode == nil {
			result.xidCode = tp.xidCode
		}
		if result.powerViolations == nil {
			result.powerViolations = tp.powerViolations
		}
		if result.thermalViolations == nil {
			result.thermalViolations = tp.thermalViolations
		}
		if result.reliabilityViolations == nil {
			result.reliabilityViolations = tp.reliabilityViolations
		}
		if result.lastUpdated.IsZero() || (!tp.lastUpdated.IsZero() && tp.lastUpdated.After(result.lastUpdated)) {
			result.lastUpdated = tp.lastUpdated
		}
	}

	if foundUUID || foundIdx {
		return result, true
	}
	return telemetryPoint{}, false
}

var telemetryHTTPClient = &http.Client{Timeout: 3 * time.Second}

const (
	deviceHealthRecoveryThreshold = 3

	metricKeyXIDCode               = "dcgm_xid_code"
	metricKeyPowerViolations       = "dcgm_power_violation"
	metricKeyThermalViolations     = "dcgm_thermal_violation"
	metricKeyReliabilityViolations = "dcgm_reliability_violation"
)

func (r *Reconciler) collectNodeTelemetry(ctx context.Context, node string) (nodeTelemetry, error) {
	result := nodeTelemetry{
		byUUID:  make(map[string]telemetryPoint),
		byIndex: make(map[string]telemetryPoint),
	}

	pods := &corev1.PodList{}
	if err := r.client.List(ctx, pods,
		client.InNamespace(meta.WorkloadsNamespace),
		client.MatchingLabels{"app": meta.AppName(meta.ComponentDCGMExporter)},
	); err != nil {
		return result, err
	}

	var exporter *corev1.Pod
	for i := range pods.Items {
		pod := &pods.Items[i]
		if pod.Spec.NodeName != node || pod.Status.PodIP == "" || !podReady(pod) {
			continue
		}
		exporter = pod
		break
	}
	if exporter == nil {
		return result, nil
	}

	telemetry, err := scrapeExporterMetrics(ctx, exporter)
	if err != nil {
		return result, nil
	}

	return telemetry, nil
}

func scrapeExporterMetrics(ctx context.Context, pod *corev1.Pod) (nodeTelemetry, error) {
	out := nodeTelemetry{
		byUUID:  make(map[string]telemetryPoint),
		byIndex: make(map[string]telemetryPoint),
	}

	if pod.Status.PodIP == "" {
		return out, nil
	}

	port := exporterPort(pod)
	url := fmt.Sprintf("http://%s:%d/metrics", pod.Status.PodIP, port)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return out, nil
	}

	resp, err := telemetryHTTPClient.Do(req)
	if err != nil {
		return out, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return out, nil
	}

	return parseExporterMetrics(resp.Body)
}

func parseExporterMetrics(reader io.Reader) (nodeTelemetry, error) {
	telemetry := nodeTelemetry{
		byUUID:  make(map[string]telemetryPoint),
		byIndex: make(map[string]telemetryPoint),
	}

	scanner := bufio.NewScanner(reader)
	now := time.Now().UTC()
	var heartbeat time.Time

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		metric, labels, value, ok := parseMetricLine(line)
		if !ok {
			continue
		}

		switch metric {
		case "dcgm_exporter_last_update_time_seconds":
			sec := int64(value)
			nsec := int64((value - float64(sec)) * float64(time.Second))
			heartbeat = time.Unix(sec, nsec).UTC()
		case "DCGM_FI_DEV_GPU_TEMP":
			v := int32(value)
			storeTelemetry(telemetry, labels, func(tp *telemetryPoint) {
				tp.temperatureC = &v
			})
		case "DCGM_FI_DEV_ECC_DBE_AGG_TOTAL":
			v := int64(value)
			storeTelemetry(telemetry, labels, func(tp *telemetryPoint) {
				tp.eccTotal = &v
			})
		case "DCGM_FI_DEV_XID_ERRORS":
			v := int64(value)
			storeTelemetry(telemetry, labels, func(tp *telemetryPoint) {
				tp.xidCode = &v
			})
		case "DCGM_FI_DEV_POWER_VIOLATION":
			v := int64(value)
			storeTelemetry(telemetry, labels, func(tp *telemetryPoint) {
				tp.powerViolations = &v
			})
		case "DCGM_FI_DEV_THERMAL_VIOLATION":
			v := int64(value)
			storeTelemetry(telemetry, labels, func(tp *telemetryPoint) {
				tp.thermalViolations = &v
			})
		case "DCGM_FI_DEV_RELIABILITY_VIOLATION":
			v := int64(value)
			storeTelemetry(telemetry, labels, func(tp *telemetryPoint) {
				tp.reliabilityViolations = &v
			})
		}
	}

	timestamp := heartbeat
	if timestamp.IsZero() {
		timestamp = now
	}
	for key, tp := range telemetry.byUUID {
		if tp.lastUpdated.IsZero() {
			tp.lastUpdated = timestamp
			telemetry.byUUID[key] = tp
		}
	}
	for key, tp := range telemetry.byIndex {
		if tp.lastUpdated.IsZero() {
			tp.lastUpdated = timestamp
			telemetry.byIndex[key] = tp
		}
	}

	return telemetry, scanner.Err()
}

func parseMetricLine(line string) (string, map[string]string, float64, bool) {
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return "", nil, 0, false
	}

	name := parts[0]
	labelStr := ""
	if idx := strings.Index(name, "{"); idx != -1 {
		if end := strings.Index(name[idx:], "}"); end != -1 {
			labelStr = name[idx+1 : idx+end]
			name = name[:idx]
		}
	}

	valueStr := parts[len(parts)-1]
	value, err := strconv.ParseFloat(valueStr, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return "", nil, 0, false
	}

	labels := map[string]string{}
	if labelStr != "" {
		for _, item := range strings.Split(labelStr, ",") {
			kv := strings.SplitN(item, "=", 2)
			if len(kv) != 2 {
				continue
			}
			labels[strings.TrimSpace(kv[0])] = strings.Trim(strings.TrimSpace(kv[1]), `"`)
		}
	}

	return name, labels, value, true
}

func storeTelemetry(t nodeTelemetry, labels map[string]string, update func(*telemetryPoint)) {
	if labels == nil {
		return
	}
	apply := func(m map[string]telemetryPoint, key string) {
		entry := m[key]
		update(&entry)
		m[key] = entry
	}

	if uuid := labels["uuid"]; uuid != "" {
		apply(t.byUUID, uuid)
	}
	if idx := labels["gpu"]; idx != "" {
		apply(t.byIndex, idx)
	}
}

func applyTelemetry(device *v1alpha1.GPUDevice, snapshot deviceSnapshot, telemetry nodeTelemetry) {
	tp, ok := telemetry.find(snapshot)
	if !ok {
		return
	}
	health := &device.Status.Health
	initialSample := health.LastUpdatedTime == nil
	if tp.temperatureC != nil {
		health.TemperatureC = *tp.temperatureC
	}

	var faultReason, faultMessage string
	faultTime := tp.lastUpdated

	if tp.xidCode != nil {
		if *tp.xidCode != 0 {
			code := strconv.FormatInt(*tp.xidCode, 10)
			prev := ""
			if health.Metrics != nil {
				prev = health.Metrics[metricKeyXIDCode]
			}
			if health.Metrics == nil {
				health.Metrics = make(map[string]string)
			}
			health.Metrics[metricKeyXIDCode] = code
			if prev != code {
				faultReason = "XIDError"
				faultMessage = fmt.Sprintf("DCGM reported XID error code %d", *tp.xidCode)
			}
		} else if health.Metrics != nil {
			delete(health.Metrics, metricKeyXIDCode)
		}
	}

	if faultReason == "" && tp.eccTotal != nil {
		if !initialSample && *tp.eccTotal > health.ECCErrorsTotal {
			faultReason = "ECCDoubleBitError"
			faultMessage = fmt.Sprintf("ECC double-bit errors increased to %d", *tp.eccTotal)
		}
		health.ECCErrorsTotal = *tp.eccTotal
	}

	if faultReason == "" && updateMonotonicMetric(health, metricKeyPowerViolations, tp.powerViolations, initialSample) {
		faultReason = "PowerViolation"
		faultMessage = "DCGM reported power limit violation"
	}
	if faultReason == "" && updateMonotonicMetric(health, metricKeyThermalViolations, tp.thermalViolations, initialSample) {
		faultReason = "ThermalViolation"
		faultMessage = "DCGM reported thermal violation"
	}
	if faultReason == "" && updateMonotonicMetric(health, metricKeyReliabilityViolations, tp.reliabilityViolations, initialSample) {
		faultReason = "ReliabilityViolation"
		faultMessage = "DCGM reported reliability violation"
	}

	if faultReason != "" {
		trackDeviceFault(health, faultReason, faultMessage, faultTime)
	} else {
		markDeviceHealthy(health, tp.lastUpdated)
	}

	if !tp.lastUpdated.IsZero() {
		ts := metav1.NewTime(tp.lastUpdated.UTC())
		health.LastUpdatedTime = &ts
	}
}

func updateMonotonicMetric(health *v1alpha1.GPUDeviceHealth, key string, value *int64, initial bool) bool {
	if value == nil {
		if health.Metrics != nil {
			delete(health.Metrics, key)
		}
		return false
	}
	if initial {
		setHealthMetricInt(health, key, *value)
		return false
	}
	prev := getHealthMetricInt(health, key)
	setHealthMetricInt(health, key, *value)
	return *value > prev
}

func getHealthMetricInt(health *v1alpha1.GPUDeviceHealth, key string) int64 {
	if health.Metrics == nil {
		return 0
	}
	if val, ok := health.Metrics[key]; ok {
		if parsed, err := strconv.ParseInt(val, 10, 64); err == nil {
			return parsed
		}
	}
	return 0
}

func setHealthMetricInt(health *v1alpha1.GPUDeviceHealth, key string, value int64) {
	if health.Metrics == nil {
		health.Metrics = make(map[string]string)
	}
	health.Metrics[key] = strconv.FormatInt(value, 10)
}

func trackDeviceFault(health *v1alpha1.GPUDeviceHealth, reason, message string, ts time.Time) {
	health.LastErrorReason = reason
	health.LastError = message
	health.ConsecutiveHealthy = 0
	if !ts.IsZero() {
		t := metav1.NewTime(ts.UTC())
		health.LastErrorTime = &t
	} else {
		health.LastErrorTime = nil
	}
}

func markDeviceHealthy(health *v1alpha1.GPUDeviceHealth, ts time.Time) {
	if health.ConsecutiveHealthy < deviceHealthRecoveryThreshold {
		health.ConsecutiveHealthy++
	}
	if !ts.IsZero() {
		t := metav1.NewTime(ts.UTC())
		health.LastHealthyTime = &t
	}
	if health.LastError != "" && health.ConsecutiveHealthy >= deviceHealthRecoveryThreshold {
		health.LastError = ""
		health.LastErrorReason = ""
		health.LastErrorTime = nil
	}
}

func podReady(pod *corev1.Pod) bool {
	if pod.Status.Phase != corev1.PodRunning {
		return false
	}
	if pod.Status.PodIP == "" {
		return false
	}
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}

func exporterPort(pod *corev1.Pod) int32 {
	for _, container := range pod.Spec.Containers {
		if container.Name != "dcgm-exporter" {
			continue
		}
		for _, port := range container.Ports {
			if port.ContainerPort > 0 {
				return port.ContainerPort
			}
		}
	}
	return 9400
}
