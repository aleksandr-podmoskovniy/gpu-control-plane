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

package driver

import (
	"context"

	"k8s.io/client-go/kubernetes"
)

const defaultDriverName = "gpu.deckhouse.io"
const defaultCDIRoot = "/etc/cdi"

// Config defines settings for the DRA kubelet plugin.
type Config struct {
	NodeName            string
	DriverName          string
	KubeClient          kubernetes.Interface
	RegistrarDir        string
	PluginDataRoot      string
	DriverRoot          string
	HostDriverRoot      string
	CDIRoot             string
	NvidiaCDIHookPath   string
	SerializeGRPCCalls  bool
	EnableDebugResponse bool
	ErrorHandler        func(ctx context.Context, err error, msg string)
}
