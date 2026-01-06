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

package resourceslice

import (
	"sort"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
)

type slicePlan struct {
	groups     map[string]*sliceGroup
	groupKeys  []string
	standalone allocatable.DeviceList
}

type sliceGroup struct {
	counterSets          []allocatable.CounterSet
	devicesWithCounters  allocatable.DeviceList
	devicesNoConsumption allocatable.DeviceList
}

func buildSlicePlan(inv allocatable.Inventory) slicePlan {
	plan := slicePlan{
		groups: map[string]*sliceGroup{},
	}

	for _, set := range inv.CounterSets {
		group := plan.group(set.Name)
		group.counterSets = append(group.counterSets, set)
	}

	for _, dev := range inv.Devices {
		spec := dev.Spec()
		if key, ok := counterSetKeyFromConsumes(spec.Consumes); ok {
			group := plan.group(key)
			group.devicesWithCounters = append(group.devicesWithCounters, dev)
			continue
		}

		if key := groupKeyFromAttrs(spec.Attributes, plan.groups); key != "" {
			plan.groups[key].devicesNoConsumption = append(plan.groups[key].devicesNoConsumption, dev)
			continue
		}

		plan.standalone = append(plan.standalone, dev)
	}

	plan.groupKeys = make([]string, 0, len(plan.groups))
	for key := range plan.groups {
		plan.groupKeys = append(plan.groupKeys, key)
	}
	sort.Strings(plan.groupKeys)

	return plan
}

func (p *slicePlan) group(key string) *sliceGroup {
	group := p.groups[key]
	if group == nil {
		group = &sliceGroup{}
		p.groups[key] = group
	}
	return group
}

func counterSetKeyFromConsumes(consumes []allocatable.CounterConsumption) (string, bool) {
	if len(consumes) == 0 {
		return "", false
	}
	if consumes[0].CounterSet == "" {
		return "", false
	}
	return consumes[0].CounterSet, true
}

func groupKeyFromAttrs(attrs map[string]allocatable.AttributeValue, groups map[string]*sliceGroup) string {
	if len(attrs) == 0 {
		return ""
	}
	attr, ok := attrs[allocatable.AttrPCIAddress]
	if !ok || attr.String == nil {
		return ""
	}
	key := allocatable.CounterSetNameForPCI(*attr.String)
	if key == "" {
		return ""
	}
	if _, ok := groups[key]; !ok {
		return ""
	}
	return key
}
