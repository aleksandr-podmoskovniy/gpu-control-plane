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

package configapi

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VfioDeviceConfig signals that a device should be prepared for VFIO.
type VfioDeviceConfig struct {
	metav1.TypeMeta `json:",inline"`
}

// Normalize updates the configuration with implied defaults.
func (c *VfioDeviceConfig) Normalize() error {
	return nil
}

// Validate ensures the configuration is valid.
func (c *VfioDeviceConfig) Validate() error {
	return nil
}

// DeepCopyObject returns a runtime.Object copy.
func (c *VfioDeviceConfig) DeepCopyObject() runtime.Object {
	if c == nil {
		return nil
	}
	out := *c
	return &out
}
