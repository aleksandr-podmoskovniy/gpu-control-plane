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

package featuregates

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/util/version"
	"k8s.io/client-go/kubernetes"
)

type deviceStatusMode string

const (
	deviceStatusAuto     deviceStatusMode = "auto"
	deviceStatusEnabled  deviceStatusMode = "enabled"
	deviceStatusDisabled deviceStatusMode = "disabled"
)

const deviceStatusMinVersion = "v1.32.0"

// ResolveDeviceStatus reports whether device status features are enabled.
func ResolveDeviceStatus(kubeClient kubernetes.Interface, rawMode string) (bool, string, string, error) {
	mode, err := parseDeviceStatusMode(rawMode)
	if err != nil {
		return false, "auto", "", err
	}
	switch mode {
	case deviceStatusEnabled:
		return true, "config", "", nil
	case deviceStatusDisabled:
		return false, "config", "", nil
	default:
	}

	if kubeClient == nil {
		return false, "auto", "", fmt.Errorf("kube client is nil")
	}
	serverVersion, err := kubeClient.Discovery().ServerVersion()
	if err != nil {
		return false, "auto", "", fmt.Errorf("detect server version: %w", err)
	}
	parsed, err := version.ParseGeneric(serverVersion.GitVersion)
	if err != nil {
		return false, "auto", serverVersion.GitVersion, fmt.Errorf("parse server version %q: %w", serverVersion.GitVersion, err)
	}
	minVersion := version.MustParseGeneric(deviceStatusMinVersion)
	return parsed.AtLeast(minVersion), "auto", serverVersion.GitVersion, nil
}

func parseDeviceStatusMode(raw string) (deviceStatusMode, error) {
	value := strings.TrimSpace(strings.ToLower(raw))
	switch value {
	case "", "auto":
		return deviceStatusAuto, nil
	case "true", "enabled", "enable", "on", "1", "yes":
		return deviceStatusEnabled, nil
	case "false", "disabled", "disable", "off", "0", "no":
		return deviceStatusDisabled, nil
	default:
		return deviceStatusAuto, fmt.Errorf("unsupported device status mode %q", raw)
	}
}
