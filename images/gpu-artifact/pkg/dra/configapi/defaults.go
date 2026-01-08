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

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// DefaultGpuConfig provides the default GPU configuration.
func DefaultGpuConfig() *GpuConfig {
	return &GpuConfig{
		TypeMeta: metav1.TypeMeta{
			APIVersion: GroupName + "/" + Version,
			Kind:       GpuConfigKind,
		},
	}
}

// DefaultMigDeviceConfig provides the default MIG device configuration.
func DefaultMigDeviceConfig() *MigDeviceConfig {
	return &MigDeviceConfig{
		TypeMeta: metav1.TypeMeta{
			APIVersion: GroupName + "/" + Version,
			Kind:       MigDeviceConfigKind,
		},
	}
}

// DefaultVfioDeviceConfig provides the default VFIO device configuration.
func DefaultVfioDeviceConfig() *VfioDeviceConfig {
	return &VfioDeviceConfig{
		TypeMeta: metav1.TypeMeta{
			APIVersion: GroupName + "/" + Version,
			Kind:       VfioDeviceConfigKind,
		},
	}
}
