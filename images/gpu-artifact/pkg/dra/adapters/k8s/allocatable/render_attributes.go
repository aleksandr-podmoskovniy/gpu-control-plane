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

package allocatable

import (
	resourceapi "k8s.io/api/resource/v1"

	domain "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
)

// RenderAttributes converts domain attributes into API attributes.
func RenderAttributes(attrs map[string]domain.AttributeValue) map[resourceapi.QualifiedName]resourceapi.DeviceAttribute {
	if len(attrs) == 0 {
		return nil
	}
	out := make(map[resourceapi.QualifiedName]resourceapi.DeviceAttribute, len(attrs))
	for key, val := range attrs {
		attr := resourceapi.DeviceAttribute{
			StringValue: val.String,
			IntValue:    val.Int,
			BoolValue:   val.Bool,
		}
		out[resourceapi.QualifiedName(key)] = attr
	}
	return out
}
