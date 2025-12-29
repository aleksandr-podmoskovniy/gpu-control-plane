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

package hostinfo

import gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"

const (
	defaultHostSysRoot     = "/host-sys"
	defaultHostOSRelease   = "/host-etc/os-release"
	defaultOSRelease       = "/etc/os-release"
	defaultKernelRelease   = "/proc/sys/kernel/osrelease"
	defaultCPUInfo         = "/proc/cpuinfo"
	defaultDMIRelativePath = "devices/virtual/dmi/id"
)

// Discover returns basic OS/kernel/bare-metal information.
func Discover(osReleasePath, sysRoot string) *gpuv1alpha1.NodeInfo {
	osInfo := discoverOS(osReleasePath)
	kernelRelease := discoverKernelRelease()
	bareMetal := detectBareMetal(sysRoot)

	if osInfo == nil && kernelRelease == "" && bareMetal == nil {
		return nil
	}

	return &gpuv1alpha1.NodeInfo{
		OS:            osInfo,
		KernelRelease: kernelRelease,
		BareMetal:     bareMetal,
	}
}
