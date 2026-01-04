/*
Copyright 2025 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package allocator

import (
	"context"
	"sort"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
)

func groupByNode(devices []CandidateDevice) map[string][]CandidateDevice {
	nodes := map[string][]CandidateDevice{}
	for _, dev := range devices {
		if dev.NodeName == "" {
			continue
		}
		nodes[dev.NodeName] = append(nodes[dev.NodeName], dev)
	}

	for nodeName := range nodes {
		sort.Slice(nodes[nodeName], func(i, j int) bool {
			return nodes[nodeName][i].Spec.Name < nodes[nodeName][j].Spec.Name
		})
	}

	return nodes
}

func allocateOnNode(ctx context.Context, nodeName string, devices []CandidateDevice, requests []Request) (*domain.AllocationResult, bool, error) {
	if len(devices) == 0 {
		return nil, false, nil
	}

	used := map[DeviceKey]struct{}{}
	results := make([]domain.AllocatedDevice, 0)

	for _, req := range requests {
		var allocatedCount int64
		for _, dev := range devices {
			if _, ok := used[dev.Key]; ok {
				continue
			}
			match, err := matchesDevice(ctx, dev.Driver, dev.Spec, req.Selectors)
			if err != nil {
				return nil, false, err
			}
			if !match {
				continue
			}
			results = append(results, domain.AllocatedDevice{
				Request: req.Name,
				Driver:  dev.Driver,
				Pool:    dev.Pool,
				Device:  dev.Spec.Name,
			})
			used[dev.Key] = struct{}{}
			allocatedCount++
			if allocatedCount >= req.Count {
				break
			}
		}
		if allocatedCount < req.Count {
			return nil, false, nil
		}
	}

	return &domain.AllocationResult{
		NodeName: nodeName,
		Devices:  results,
		NodeSelector: &domain.NodeSelector{
			NodeName: nodeName,
		},
	}, true, nil
}

func matchesDevice(ctx context.Context, driver string, spec allocatable.DeviceSpec, selectors []Selector) (bool, error) {
	if len(selectors) == 0 {
		return true, nil
	}

	for _, sel := range selectors {
		ok, err := sel.Match(ctx, driver, spec)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}
