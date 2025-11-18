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
	temperatureC *int32
	eccTotal     *int64
	lastUpdated  time.Time
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
		return result, fmt.Errorf("dcgm exporter pod for node %s not found", node)
	}

	telemetry, err := scrapeExporterMetrics(ctx, exporter)
	if err != nil {
		return result, err
	}

	return telemetry, nil
}

func scrapeExporterMetrics(ctx context.Context, pod *corev1.Pod) (nodeTelemetry, error) {
	out := nodeTelemetry{
		byUUID:  make(map[string]telemetryPoint),
		byIndex: make(map[string]telemetryPoint),
	}

	if pod.Status.PodIP == "" {
		return out, fmt.Errorf("pod %s has no IP assigned", pod.Name)
	}

	port := exporterPort(pod)
	url := fmt.Sprintf("http://%s:%d/metrics", pod.Status.PodIP, port)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return out, err
	}

	resp, err := telemetryHTTPClient.Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return out, fmt.Errorf("unexpected status %s", resp.Status)
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
	if tp.temperatureC != nil {
		device.Status.Health.TemperatureC = *tp.temperatureC
	}
	if tp.eccTotal != nil {
		device.Status.Health.ECCErrorsTotal = *tp.eccTotal
	}
	if !tp.lastUpdated.IsZero() {
		ts := metav1.NewTime(tp.lastUpdated.UTC())
		device.Status.Health.LastUpdatedTime = &ts
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
