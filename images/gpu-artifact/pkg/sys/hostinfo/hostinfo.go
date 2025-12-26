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

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
)

const (
	defaultHostSysRoot     = "/host-sys"
	defaultHostOSRelease   = "/host-etc/os-release"
	defaultOSRelease       = "/etc/os-release"
	defaultKernelRelease   = "/proc/sys/kernel/osrelease"
	defaultCPUInfo         = "/proc/cpuinfo"
	defaultDMIRelativePath = "devices/virtual/dmi/id"
)

var virtualizationMarkers = []string{
	"kvm",
	"qemu",
	"vmware",
	"virtualbox",
	"xen",
	"hv",
	"hyper-v",
	"microsoft",
	"openstack",
	"amazon",
	"ec2",
	"google",
	"compute engine",
	"digitalocean",
	"parallels",
	"bhyve",
	"bochs",
}

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

func discoverOS(osReleasePath string) *gpuv1alpha1.OSInfo {
	path := osReleasePath
	if path == "" {
		path = defaultHostOSRelease
	}
	if _, err := os.Stat(path); err != nil {
		path = defaultOSRelease
	}

	release, err := parseOSRelease(path)
	if err != nil {
		return nil
	}

	return &gpuv1alpha1.OSInfo{
		ID:      release["ID"],
		Version: release["VERSION_ID"],
		Name:    release["NAME"],
	}
}

func parseOSRelease(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	re := regexp.MustCompile(`^(?P<key>\w+)=(?P<value>.+)`)
	release := map[string]string{}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if m := re.FindStringSubmatch(line); m != nil {
			release[m[1]] = strings.Trim(m[2], `"'`)
		}
	}

	return release, nil
}

func discoverKernelRelease() string {
	data, err := os.ReadFile(defaultKernelRelease)
	if err != nil {
		return ""
	}
	raw := strings.TrimSpace(string(data))
	if raw == "" {
		return ""
	}

	return raw
}

func detectBareMetal(sysRoot string) *bool {
	root := sysRoot
	if root == "" {
		root = defaultHostSysRoot
	}

	dmiDir := filepath.Join(root, defaultDMIRelativePath)
	if _, err := os.Stat(dmiDir); err != nil {
		dmiDir = filepath.Join("/sys", defaultDMIRelativePath)
	}

	sysVendor, _ := readTrim(filepath.Join(dmiDir, "sys_vendor"))
	productName, _ := readTrim(filepath.Join(dmiDir, "product_name"))

	if isVirtualizedCPU() || isVirtualizedDMI(sysVendor, productName) {
		value := false
		return &value
	}

	if sysVendor == "" && productName == "" {
		return nil
	}

	value := true
	return &value
}

func isVirtualizedCPU() bool {
	data, err := os.ReadFile(defaultCPUInfo)
	if err != nil {
		return false
	}
	return regexp.MustCompile(`\bhypervisor\b`).Match(data)
}

func isVirtualizedDMI(sysVendor, productName string) bool {
	combined := strings.ToLower(strings.TrimSpace(sysVendor + " " + productName))
	if combined == "" {
		return false
	}
	for _, marker := range virtualizationMarkers {
		if strings.Contains(combined, marker) {
			return true
		}
	}
	return false
}

func readTrim(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}
