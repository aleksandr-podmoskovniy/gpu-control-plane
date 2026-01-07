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

package prepare

import (
	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
)

func shareID(id *types.UID) string {
	if id == nil {
		return ""
	}
	return string(*id)
}

func consumedCapacity(input map[resourcev1.QualifiedName]resource.Quantity) map[string]resource.Quantity {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]resource.Quantity, len(input))
	for key, val := range input {
		out[string(key)] = val.DeepCopy()
	}
	return out
}

func attributesFromDevice(device resourcev1.Device) map[string]allocatable.AttributeValue {
	if len(device.Attributes) == 0 {
		return nil
	}
	out := make(map[string]allocatable.AttributeValue, len(device.Attributes))
	for key, attr := range device.Attributes {
		value := allocatable.AttributeValue{}
		if attr.StringValue != nil {
			v := *attr.StringValue
			value.String = &v
		}
		if attr.IntValue != nil {
			v := *attr.IntValue
			value.Int = &v
		}
		if attr.BoolValue != nil {
			v := *attr.BoolValue
			value.Bool = &v
		}
		out[string(key)] = value
	}
	return out
}
