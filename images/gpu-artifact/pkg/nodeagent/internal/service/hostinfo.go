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

package service

import (
	"context"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/sys/hostinfo"
)

// HostInfoProvider reports OS/kernel/bare-metal info for the node.
type HostInfoProvider interface {
	NodeInfo(ctx context.Context) *gpuv1alpha1.NodeInfo
}

// HostInfoCollector gathers node information from host files.
type HostInfoCollector struct {
	OSReleasePath string
	SysRoot       string
}

// NewHostInfoCollector creates a host info provider.
func NewHostInfoCollector(osReleasePath, sysRoot string) *HostInfoCollector {
	return &HostInfoCollector{
		OSReleasePath: osReleasePath,
		SysRoot:       sysRoot,
	}
}

// NodeInfo returns detected node information.
func (h *HostInfoCollector) NodeInfo(_ context.Context) *gpuv1alpha1.NodeInfo {
	return hostinfo.Discover(h.OSReleasePath, h.SysRoot)
}
