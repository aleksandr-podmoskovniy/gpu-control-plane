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

	"k8s.io/dynamic-resource-allocation/resourceslice"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/state"
)

const publishResourcesHandlerName = "publish-resources"

// ResourcesPublisher publishes DRA ResourceSlices for a driver.
type ResourcesPublisher interface {
	PublishResources(ctx context.Context, resources resourceslice.DriverResources) error
}

// ResourceSliceBuilder builds driver resources for a node.
type ResourceSliceBuilder interface {
	Build(ctx context.Context, nodeName string, devices []gpuv1alpha1.PhysicalGPU) (resourceslice.DriverResources, error)
}

// PublishResourcesHandler publishes ResourceSlices based on PhysicalGPU status.
type PublishResourcesHandler struct {
	builder   ResourceSliceBuilder
	publisher ResourcesPublisher
}

// NewPublishResourcesHandler constructs a publish handler.
func NewPublishResourcesHandler(builder ResourceSliceBuilder, publisher ResourcesPublisher) *PublishResourcesHandler {
	return &PublishResourcesHandler{
		builder:   builder,
		publisher: publisher,
	}
}

// Name returns the handler name.
func (h *PublishResourcesHandler) Name() string {
	return publishResourcesHandlerName
}

// Handle publishes ResourceSlice inventory for ready GPUs.
func (h *PublishResourcesHandler) Handle(ctx context.Context, st state.State) error {
	if h.builder == nil || h.publisher == nil {
		return nil
	}

	resources, buildErr := h.builder.Build(ctx, st.NodeName(), st.Ready())
	publishErr := h.publisher.PublishResources(ctx, resources)
	return errors.Join(buildErr, publishErr)
}
