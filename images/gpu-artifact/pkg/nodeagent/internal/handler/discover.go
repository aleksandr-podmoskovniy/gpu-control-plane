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

package handler

import (
	"context"
	"errors"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/nodeagent/internal/service"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/nodeagent/internal/state"
)

const discoverHandlerName = "Discover"

// DiscoverHandler gathers devices and host info.
type DiscoverHandler struct {
	pci      service.PCIProvider
	hostInfo service.HostInfoProvider
}

// NewDiscoverHandler constructs a discovery handler.
func NewDiscoverHandler(pci service.PCIProvider, hostInfo service.HostInfoProvider) *DiscoverHandler {
	return &DiscoverHandler{pci: pci, hostInfo: hostInfo}
}

// Name returns the handler name.
func (h *DiscoverHandler) Name() string {
	return discoverHandlerName
}

// Handle scans for PCI devices and populates the state.
func (h *DiscoverHandler) Handle(ctx context.Context, st state.State) error {
	if st.NodeName() == "" {
		return StopHandlerChain(errors.New("node name is empty"))
	}

	devices, err := h.pci.Scan(ctx)
	if err != nil {
		return StopHandlerChain(err)
	}

	st.SetDevices(devices)
	st.SetNodeInfo(h.hostInfo.NodeInfo(ctx))

	return nil
}
