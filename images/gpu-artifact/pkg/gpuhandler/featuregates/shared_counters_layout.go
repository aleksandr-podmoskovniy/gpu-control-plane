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

	"k8s.io/apimachinery/pkg/util/version"
	"k8s.io/client-go/kubernetes"

	k8sresourceslice "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/adapters/k8s/resourceslice"
)

const sharedCountersSeparateMinVersion = "v1.34.0"

func resolveSharedCountersLayout(kubeClient kubernetes.Interface) (k8sresourceslice.SharedCountersLayout, string, string, error) {
	if kubeClient == nil {
		return k8sresourceslice.SharedCountersInline, "auto", "", fmt.Errorf("kube client is nil")
	}

	serverVersion, err := kubeClient.Discovery().ServerVersion()
	if err != nil {
		return k8sresourceslice.SharedCountersInline, "auto", "", fmt.Errorf("detect server version: %w", err)
	}

	parsed, err := version.ParseGeneric(serverVersion.GitVersion)
	if err != nil {
		return k8sresourceslice.SharedCountersInline, "auto", serverVersion.GitVersion, fmt.Errorf("parse server version %q: %w", serverVersion.GitVersion, err)
	}

	minVersion := version.MustParseGeneric(sharedCountersSeparateMinVersion)
	if parsed.AtLeast(minVersion) {
		return k8sresourceslice.SharedCountersSeparate, "auto", serverVersion.GitVersion, nil
	}
	return k8sresourceslice.SharedCountersInline, "auto", serverVersion.GitVersion, nil
}
