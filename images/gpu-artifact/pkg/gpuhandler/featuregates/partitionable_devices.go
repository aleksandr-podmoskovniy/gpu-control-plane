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
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/util/version"
	"k8s.io/client-go/kubernetes"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/logger"
)

const partitionableMinVersion = "v1.34.0"

// ConfigurePartitionableDevices disables partitionable devices when the apiserver is too old.
func (s *Service) ConfigurePartitionableDevices(kubeClient kubernetes.Interface) {
	supported, source, serverVersion, err := resolvePartitionableSupport(kubeClient)
	if err != nil && s.log != nil {
		s.log.Warn("failed to resolve partitionable devices support", "source", source, "apiserverVersion", serverVersion, logger.SlogErr(err))
	}

	if !supported && s.builder != nil {
		if s.builder.DisableFeatures([]string{featurePartitionable}) && s.log != nil {
			s.log.Warn("partitionable devices disabled", "source", source, "apiserverVersion", serverVersion)
		}
		if s.recorder != nil {
			poolName := "gpus/" + s.nodeName
			s.recordFeatureGateEvents(context.Background(), poolName, []string{featurePartitionable})
		}
	}

	if s.log != nil {
		s.log.Info("partitionable devices support resolved", "supported", supported, "source", source, "apiserverVersion", serverVersion)
	}
}

func resolvePartitionableSupport(kubeClient kubernetes.Interface) (bool, string, string, error) {
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

	minVersion := version.MustParseGeneric(partitionableMinVersion)
	return parsed.AtLeast(minVersion), "auto", serverVersion.GitVersion, nil
}
