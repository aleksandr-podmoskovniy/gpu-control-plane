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
	"os"
	"path/filepath"
	"regexp"
	"strings"
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

func detectBareMetal(sysRoot string) *bool {
	sysVendor, productName := readDMI(sysRoot)
	cpuInfo := readCPUInfo()
	return detectBareMetalFromInfo(sysVendor, productName, cpuInfo)
}

func detectBareMetalFromInfo(sysVendor, productName, cpuInfo string) *bool {
	if isVirtualizedCPUInfo(cpuInfo) || isVirtualizedDMI(sysVendor, productName) {
		value := false
		return &value
	}

	if sysVendor == "" && productName == "" {
		return nil
	}

	value := true
	return &value
}

func readDMI(sysRoot string) (string, string) {
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

	return sysVendor, productName
}

func readCPUInfo() string {
	data, err := os.ReadFile(defaultCPUInfo)
	if err != nil {
		return ""
	}
	return string(data)
}

func isVirtualizedCPUInfo(cpuInfo string) bool {
	return regexp.MustCompile(`\bhypervisor\b`).MatchString(cpuInfo)
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
