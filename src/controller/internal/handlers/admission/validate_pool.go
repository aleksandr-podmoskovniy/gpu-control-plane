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

package admission

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
)

const (
	defaultProvider = "Nvidia"
	defaultBackend  = "DevicePlugin"
)

// PoolValidationHandler validates backend/provider/slicing constraints and applies safe defaults.
type PoolValidationHandler struct {
	log logr.Logger
}

func NewPoolValidationHandler(log logr.Logger) *PoolValidationHandler {
	return &PoolValidationHandler{log: log.WithName("pool-validation")}
}

func (h *PoolValidationHandler) Name() string {
	return "pool-validation"
}

func (h *PoolValidationHandler) SyncPool(_ context.Context, pool *v1alpha1.GPUPool) (contracts.Result, error) {
	if strings.TrimSpace(pool.Name) == "" {
		return contracts.Result{}, fmt.Errorf("metadata.name must be set")
	}
	applyDefaults(&pool.Spec)

	validators := []func(*v1alpha1.GPUPoolSpec) error{
		func(spec *v1alpha1.GPUPoolSpec) error { return h.validateProvider(spec.Provider) },
		h.validateResource,
		h.validateSelectors,
		h.validateScheduling,
	}
	if err := runValidations(validators, &pool.Spec); err != nil {
		return contracts.Result{}, err
	}

	return contracts.Result{}, nil
}

func (h *PoolValidationHandler) validateProvider(provider string) error {
	if provider != defaultProvider {
		return fmt.Errorf("unsupported provider %q", provider)
	}
	return nil
}

func (h *PoolValidationHandler) validateResource(spec *v1alpha1.GPUPoolSpec) error {
	if spec.Resource.Unit == "" {
		return fmt.Errorf("resource.unit must be set")
	}
	switch spec.Resource.Unit {
	case "Card":
		if spec.Resource.MIGProfile != "" {
			return fmt.Errorf("resource.migProfile is not allowed when unit=Card")
		}
	case "MIG":
		if spec.Resource.MIGProfile == "" {
			return fmt.Errorf("resource.migProfile is required when unit=MIG")
		}
		if !isValidMIGProfile(spec.Resource.MIGProfile) {
			return fmt.Errorf("resource.migProfile %q has invalid format", spec.Resource.MIGProfile)
		}
	default:
		return fmt.Errorf("unsupported resource.unit %q", spec.Resource.Unit)
	}

	if spec.Resource.SlicesPerUnit < 1 {
		return fmt.Errorf("resource.slicesPerUnit must be >= 1")
	}
	if spec.Resource.SlicesPerUnit > 64 {
		return fmt.Errorf("resource.slicesPerUnit must be <= 64")
	}

	if spec.Backend == "DRA" {
		if spec.Resource.Unit != "Card" {
			return fmt.Errorf("backend=DRA currently supports only unit=Card")
		}
		if spec.Resource.SlicesPerUnit > 1 {
			return fmt.Errorf("backend=DRA does not support slicesPerUnit>1")
		}
	}
	return nil
}

func (h *PoolValidationHandler) validateSelectors(spec *v1alpha1.GPUPoolSpec) error {
	if spec.DeviceSelector == nil {
		spec.DeviceSelector = &v1alpha1.GPUPoolDeviceSelector{}
	}

	spec.DeviceSelector.Include.InventoryIDs = dedupStrings(spec.DeviceSelector.Include.InventoryIDs)
	spec.DeviceSelector.Include.Products = dedupStrings(spec.DeviceSelector.Include.Products)
	spec.DeviceSelector.Include.PCIVendors = dedupStrings(spec.DeviceSelector.Include.PCIVendors)
	spec.DeviceSelector.Include.PCIDevices = dedupStrings(spec.DeviceSelector.Include.PCIDevices)
	spec.DeviceSelector.Include.MIGProfiles = dedupStrings(spec.DeviceSelector.Include.MIGProfiles)

	spec.DeviceSelector.Exclude.InventoryIDs = dedupStrings(spec.DeviceSelector.Exclude.InventoryIDs)
	spec.DeviceSelector.Exclude.Products = dedupStrings(spec.DeviceSelector.Exclude.Products)
	spec.DeviceSelector.Exclude.PCIVendors = dedupStrings(spec.DeviceSelector.Exclude.PCIVendors)
	spec.DeviceSelector.Exclude.PCIDevices = dedupStrings(spec.DeviceSelector.Exclude.PCIDevices)
	spec.DeviceSelector.Exclude.MIGProfiles = dedupStrings(spec.DeviceSelector.Exclude.MIGProfiles)

	for _, vendor := range append(spec.DeviceSelector.Include.PCIVendors, spec.DeviceSelector.Exclude.PCIVendors...) {
		if !isHex4(vendor) {
			return fmt.Errorf("pci vendor %q must be 4-digit hex without 0x", vendor)
		}
	}
	for _, dev := range append(spec.DeviceSelector.Include.PCIDevices, spec.DeviceSelector.Exclude.PCIDevices...) {
		if !isHex4(dev) {
			return fmt.Errorf("pci device %q must be 4-digit hex without 0x", dev)
		}
	}
	for _, mp := range append(spec.DeviceSelector.Include.MIGProfiles, spec.DeviceSelector.Exclude.MIGProfiles...) {
		if !isValidMIGProfile(mp) {
			return fmt.Errorf("migProfile %q has invalid format", mp)
		}
	}

	if spec.NodeSelector != nil {
		if _, err := metav1.LabelSelectorAsSelector(spec.NodeSelector); err != nil {
			return fmt.Errorf("invalid nodeSelector: %w", err)
		}
	}
	if sel := spec.DeviceAssignment.AutoApproveSelector; sel != nil {
		if _, err := metav1.LabelSelectorAsSelector(sel); err != nil {
			return fmt.Errorf("invalid deviceAssignment.autoApproveSelector: %w", err)
		}
	}
	return nil
}

func (h *PoolValidationHandler) validateScheduling(spec *v1alpha1.GPUPoolSpec) error {
	switch spec.Scheduling.Strategy {
	case "", v1alpha1.GPUPoolSchedulingBinPack, v1alpha1.GPUPoolSchedulingSpread:
	default:
		return fmt.Errorf("unsupported scheduling.strategy %q", spec.Scheduling.Strategy)
	}
	if spec.Scheduling.Strategy == v1alpha1.GPUPoolSchedulingSpread && spec.Scheduling.TopologyKey == "" {
		return fmt.Errorf("scheduling.topologyKey is required when strategy=Spread")
	}
	if spec.Scheduling.TaintsEnabled == nil {
		spec.Scheduling.TaintsEnabled = ptr.To(true)
	}

	for i, t := range spec.Scheduling.Taints {
		if strings.TrimSpace(t.Key) == "" {
			return fmt.Errorf("taints[%d].key must be set", i)
		}
		spec.Scheduling.Taints[i].Key = strings.TrimSpace(t.Key)
		spec.Scheduling.Taints[i].Value = strings.TrimSpace(t.Value)
		spec.Scheduling.Taints[i].Effect = strings.TrimSpace(t.Effect)
	}
	return nil
}

var migProfileRE = regexp.MustCompile(`^[0-9]+g\.[0-9]+gb$`)

func isValidMIGProfile(s string) bool {
	return migProfileRE.MatchString(strings.ToLower(s))
}

func isHex4(s string) bool {
	if len(s) != 4 {
		return false
	}
	for _, r := range s {
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return false
		}
	}
	return true
}

func applyDefaults(spec *v1alpha1.GPUPoolSpec) {
	if spec.Provider == "" {
		spec.Provider = defaultProvider
	}
	if spec.Backend == "" {
		spec.Backend = defaultBackend
	}
	if spec.Resource.SlicesPerUnit == 0 {
		spec.Resource.SlicesPerUnit = 1
	}
	if spec.Scheduling.Strategy == "" {
		spec.Scheduling.Strategy = v1alpha1.GPUPoolSchedulingSpread
	}
	if spec.Scheduling.TopologyKey == "" && spec.Scheduling.Strategy == v1alpha1.GPUPoolSchedulingSpread {
		spec.Scheduling.TopologyKey = "topology.kubernetes.io/zone"
	}
}

func dedupStrings(items []string) []string {
	out := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, v := range items {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func runValidations(validators []func(*v1alpha1.GPUPoolSpec) error, spec *v1alpha1.GPUPoolSpec) error {
	for _, v := range validators {
		if err := v(spec); err != nil {
			return err
		}
	}
	return nil
}
