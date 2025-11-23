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
	"encoding/json"
	"fmt"
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

var detectHTTPClient = &http.Client{Timeout: 2 * time.Second}

const detectGPUPath = "/api/v1/detect/gpu"

type detectGPUMemory struct {
	Total uint64 `json:"Total"`
	Free  uint64 `json:"Free"`
	Used  uint64 `json:"Used"`
}

type detectGPUUtilization struct {
	GPU    uint32 `json:"Gpu"`
	Memory uint32 `json:"Memory"`
}

type detectGPUPCI struct {
	Address   string `json:"address"`
	Vendor    string `json:"vendor"`
	Device    string `json:"device"`
	Class     string `json:"class"`
	Subsystem string `json:"subsystem"`
}

type detectGPUPCIELink struct {
	Generation *int32 `json:"generation"`
	Width      *int32 `json:"width"`
}

type detectGPUMIG struct {
	Capable           bool     `json:"capable"`
	Mode              string   `json:"mode"`
	ProfilesSupported []string `json:"profilesSupported"`
}

type detectGPUEntry struct {
	Index                       int                  `json:"index"`
	UUID                        string               `json:"uuid"`
	Product                     string               `json:"product"`
	MemoryInfo                  detectGPUMemory      `json:"memoryInfo"`
	PowerUsage                  uint32               `json:"powerUsage"`
	PowerManagementDefaultLimit uint32               `json:"powerManagementDefaultLimit"`
	Utilization                 detectGPUUtilization `json:"utilization"`
	PowerState                  uint32               `json:"powerState"`
	TemperatureC                int32                `json:"temperatureC"`
	MemoryMiB                   int32                `json:"memoryMiB"`
	ComputeMajor                int32                `json:"computeMajor"`
	ComputeMinor                int32                `json:"computeMinor"`
	NUMANode                    *int32               `json:"numaNode"`
	SMCount                     *int32               `json:"smCount"`
	MemoryBandwidthMiB          *int32               `json:"memoryBandwidthMiB"`
	PCI                         detectGPUPCI         `json:"pci"`
	PCIE                        detectGPUPCIELink    `json:"pcie"`
	Board                       string               `json:"board"`
	Family                      string               `json:"family"`
	Serial                      string               `json:"serial"`
	DisplayMode                 string               `json:"displayMode"`
	Precision                   []string             `json:"precision"`
	MIG                         detectGPUMIG         `json:"mig"`
}

type nodeDetection struct {
	byUUID  map[string]detectGPUEntry
	byIndex map[string]detectGPUEntry
}

func (r *Reconciler) collectNodeDetections(ctx context.Context, node string) (nodeDetection, error) {
	result := nodeDetection{
		byUUID:  make(map[string]detectGPUEntry),
		byIndex: make(map[string]detectGPUEntry),
	}

	pods := &corev1.PodList{}
	if err := r.client.List(ctx, pods,
		client.InNamespace(meta.WorkloadsNamespace),
		client.MatchingLabels{"app": meta.AppName(meta.ComponentGPUFeatureDiscovery)}); err != nil {
		return result, err
	}

	var targetPod *corev1.Pod
	var port int32
	for i := range pods.Items {
		pod := &pods.Items[i]
		if pod.Spec.NodeName != node {
			continue
		}
		if pod.Status.PodIP == "" || !isPodReady(pod) {
			continue
		}
		p := detectGPUPort(pod)
		if p == 0 {
			continue
		}
		targetPod = pod
		port = p
		break
	}

	if targetPod == nil {
		return result, fmt.Errorf("gfd-extender pod for node %s not found", node)
	}

	url := fmt.Sprintf("http://%s:%d%s", targetPod.Status.PodIP, port, detectGPUPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return result, err
	}

	resp, err := detectHTTPClient.Do(req)
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return result, fmt.Errorf("unexpected status %s", resp.Status)
	}

	var entries []detectGPUEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return result, err
	}

	for _, entry := range entries {
		if entry.UUID != "" {
			result.byUUID[entry.UUID] = entry
		}
		indexKey := strconv.Itoa(entry.Index)
		result.byIndex[indexKey] = entry
	}

	return result, nil
}

func detectGPUPort(pod *corev1.Pod) int32 {
	for _, container := range pod.Spec.Containers {
		if container.Name != "gfd-extender" {
			continue
		}
		for _, port := range container.Ports {
			if port.ContainerPort > 0 {
				return port.ContainerPort
			}
		}
	}
	return 0
}

func (n nodeDetection) find(snapshot deviceSnapshot) (detectGPUEntry, bool) {
	if snapshot.UUID != "" {
		if entry, ok := n.byUUID[snapshot.UUID]; ok {
			return entry, true
		}
	}
	if entry, ok := n.byIndex[snapshot.Index]; ok {
		return entry, true
	}
	return detectGPUEntry{}, false
}

func applyDetection(device *v1alpha1.GPUDevice, snapshot deviceSnapshot, detections nodeDetection) {
	entry, ok := detections.find(snapshot)
	if !ok {
		return
	}
	health := &device.Status.Health
	if health.Metrics == nil {
		health.Metrics = make(map[string]string)
	}
	health.Metrics["detect.powerUsageMilliwatt"] = fmt.Sprintf("%d", entry.PowerUsage)
	health.Metrics["detect.powerLimitMilliwatt"] = fmt.Sprintf("%d", entry.PowerManagementDefaultLimit)
	health.Metrics["detect.memory.totalBytes"] = fmt.Sprintf("%d", entry.MemoryInfo.Total)
	health.Metrics["detect.memory.freeBytes"] = fmt.Sprintf("%d", entry.MemoryInfo.Free)
	health.Metrics["detect.memory.usedBytes"] = fmt.Sprintf("%d", entry.MemoryInfo.Used)
	health.Metrics["detect.utilization.gpuPercent"] = fmt.Sprintf("%d", entry.Utilization.GPU)
	health.Metrics["detect.utilization.memoryPercent"] = fmt.Sprintf("%d", entry.Utilization.Memory)
	if entry.TemperatureC != 0 {
		health.Metrics["detect.temperature.celsius"] = fmt.Sprintf("%d", entry.TemperatureC)
		if device.Status.Health.TemperatureC == 0 {
			device.Status.Health.TemperatureC = entry.TemperatureC
		}
	}

	now := metav1.NewTime(time.Now().UTC())
	health.LastUpdatedTime = &now
	health.LastHealthyTime = &now

	applyDetectionHardware(device, entry)
}

func isPodReady(pod *corev1.Pod) bool {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}

func applyDetectionHardware(device *v1alpha1.GPUDevice, entry detectGPUEntry) {
	hw := &device.Status.Hardware

	if entry.Product != "" {
		hw.Product = entry.Product
	}
	if entry.UUID != "" {
		hw.UUID = entry.UUID
	}
	if entry.MemoryMiB > 0 {
		hw.MemoryMiB = entry.MemoryMiB
	}
	if entry.ComputeMajor > 0 {
		capability := &v1alpha1.GPUComputeCapability{Major: entry.ComputeMajor, Minor: entry.ComputeMinor}
		if !computeCapabilityEqual(hw.ComputeCapability, capability) {
			hw.ComputeCapability = capability
		}
	}
	if entry.PCI.Vendor != "" {
		hw.PCI.Vendor = strings.ToLower(entry.PCI.Vendor)
	}
	if entry.PCI.Device != "" {
		hw.PCI.Device = strings.ToLower(entry.PCI.Device)
	}
	if entry.PCI.Class != "" {
		hw.PCI.Class = strings.ToLower(entry.PCI.Class)
	}
	if entry.PCI.Address != "" {
		hw.PCI.Address = strings.ToLower(entry.PCI.Address)
	}
	if entry.NUMANode != nil && !int32PtrEqual(hw.NUMANode, entry.NUMANode) {
		hw.NUMANode = entry.NUMANode
	}
	if entry.PowerManagementDefaultLimit > 0 {
		hw.PowerLimitMilliWatt = detectionInt32Ptr(int32(entry.PowerManagementDefaultLimit))
	}
	if entry.SMCount != nil {
		hw.SMCount = entry.SMCount
	}
	if entry.MemoryBandwidthMiB != nil {
		hw.MemoryBandwidthMiB = entry.MemoryBandwidthMiB
	}
	desiredPCIE := v1alpha1.PCIELink{
		Generation: entry.PCIE.Generation,
		Width:      entry.PCIE.Width,
	}
	if !pcieEqual(hw.PCIE, desiredPCIE) {
		hw.PCIE = desiredPCIE
	}
	if entry.Board != "" {
		hw.Board = entry.Board
	}
	if entry.Family != "" {
		hw.Family = entry.Family
	}
	if entry.Serial != "" {
		hw.Serial = entry.Serial
	}
	if entry.DisplayMode != "" {
		hw.DisplayMode = entry.DisplayMode
	}
	hw.PState = fmt.Sprintf("P%d", entry.PowerState)
	if entry.MIG.Capable {
		hw.MIG.Capable = true
	}
	if len(entry.MIG.ProfilesSupported) > 0 {
		hw.MIG.ProfilesSupported = append([]string(nil), entry.MIG.ProfilesSupported...)
	}
	if len(entry.Precision) > 0 && len(hw.Precision.Supported) == 0 {
		hw.Precision.Supported = append([]string(nil), entry.Precision...)
	}
}

func detectionInt32Ptr(value int32) *int32 {
	val := value
	return &val
}
