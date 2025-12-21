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
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/bootstrap/meta"
	invpci "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory/internal/pci"
	invstate "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory/internal/state"
)

// DetectionCollector fetches detections from gfd-extender for a node.
type DetectionCollector interface {
	Collect(ctx context.Context, node string) (NodeDetection, error)
}

type detectionCollector struct {
	client client.Client
}

func NewDetectionCollector(c client.Client) DetectionCollector {
	return &detectionCollector{client: c}
}

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

type NodeDetection struct {
	byUUID  map[string]detectGPUEntry
	byIndex map[string]detectGPUEntry
}

func (c *detectionCollector) Collect(ctx context.Context, node string) (NodeDetection, error) {
	result := NodeDetection{
		byUUID:  make(map[string]detectGPUEntry),
		byIndex: make(map[string]detectGPUEntry),
	}

	pods := &corev1.PodList{}
	if err := c.client.List(ctx, pods,
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
		// GFD DaemonSet ещё не готов — не считаем это ошибкой, просто пропускаем цикл.
		return result, nil
	}

	url := fmt.Sprintf("http://%s:%d%s", targetPod.Status.PodIP, port, detectGPUPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return result, nil
	}

	resp, err := detectHTTPClient.Do(req)
	if err != nil {
		// При старте pod может не слушать ещё; не шумим и не блокируем reconcile.
		return result, nil
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return result, nil
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

func (n NodeDetection) find(snapshot invstate.DeviceSnapshot) (detectGPUEntry, bool) {
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

func ApplyDetection(device *v1alpha1.GPUDevice, snapshot invstate.DeviceSnapshot, detections NodeDetection) {
	entry, ok := detections.find(snapshot)
	if !ok {
		return
	}

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
	if entry.PCI.Vendor != "" && hw.PCI.Vendor == "" {
		hw.PCI.Vendor = strings.ToLower(entry.PCI.Vendor)
	}
	if entry.PCI.Device != "" && hw.PCI.Device == "" {
		hw.PCI.Device = strings.ToLower(entry.PCI.Device)
	}
	if entry.PCI.Class != "" && hw.PCI.Class == "" {
		hw.PCI.Class = strings.ToLower(entry.PCI.Class)
	}
	if entry.PCI.Address != "" {
		if addr := invpci.CanonicalizePCIAddress(entry.PCI.Address); addr != "" && addr != hw.PCI.Address {
			hw.PCI.Address = addr
		}
	}
	if entry.MIG.Capable {
		hw.MIG.Capable = true
	}
	if mode := strings.TrimSpace(entry.MIG.Mode); mode != "" {
		switch strings.ToLower(mode) {
		case "single":
			hw.MIG.Strategy = v1alpha1.GPUMIGStrategySingle
		case "mixed":
			hw.MIG.Strategy = v1alpha1.GPUMIGStrategyMixed
		case "none":
			hw.MIG.Strategy = v1alpha1.GPUMIGStrategyNone
		default:
			// Unknown values may belong to a different semantic (e.g. "enabled"/"disabled") — do not override.
		}
	}
	if len(entry.MIG.ProfilesSupported) > 0 {
		seen := make(map[string]struct{}, len(entry.MIG.ProfilesSupported))
		profiles := make([]string, 0, len(entry.MIG.ProfilesSupported))
		for _, raw := range entry.MIG.ProfilesSupported {
			profile := strings.TrimSpace(raw)
			if profile == "" {
				continue
			}
			profile = strings.ToLower(profile)
			profile = strings.TrimPrefix(profile, "mig-")
			if _, ok := seen[profile]; ok {
				continue
			}
			seen[profile] = struct{}{}
			profiles = append(profiles, profile)
		}
		sort.Strings(profiles)
		hw.MIG.ProfilesSupported = profiles
	}
	if !hw.MIG.Capable && len(hw.MIG.ProfilesSupported) > 0 {
		hw.MIG.Capable = true
	}
}

