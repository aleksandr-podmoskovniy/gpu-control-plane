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

package step

import (
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
)

func applyMpsAttributes(dev *domain.PrepareDevice, state domain.PreparedMpsState) {
	if dev == nil {
		return
	}
	if dev.Attributes == nil {
		dev.Attributes = map[string]allocatable.AttributeValue{}
	}
	dev.Attributes[allocatable.AttrMpsPipeDir] = allocatable.AttributeValue{String: &state.PipeDir}
	dev.Attributes[allocatable.AttrMpsShmDir] = allocatable.AttributeValue{String: &state.ShmDir}
	dev.Attributes[allocatable.AttrMpsLogDir] = allocatable.AttributeValue{String: &state.LogDir}
}

func applySharingState(state *domain.PreparedDeviceState, sharing domain.PreparedSharing) {
	if state == nil {
		return
	}
	state.Sharing = &sharing
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
