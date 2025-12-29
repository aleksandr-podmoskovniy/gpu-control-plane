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

package publisher

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	resourcev1 "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/common/resource_builder"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/ports"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/logger"
)

const (
	driverName     = "gpu.deckhouse.io"
	defaultPool    = "gpus"
	finalizerName  = "gpu.deckhouse.io/resourceslice-cleanup"
	annotationKey  = "gpu.deckhouse.io/driver"
	unknownNodeKey = "unknown"
)

// Service publishes ResourceSlices based on inventory snapshots.
type Service struct {
	inventory ports.InventoryProvider
	writer    ports.ResourceSliceWriter
}

// NewService creates a new publisher service.
func NewService(inventory ports.InventoryProvider, writer ports.ResourceSliceWriter) *Service {
	return &Service{
		inventory: inventory,
		writer:    writer,
	}
}

// PublishOnce builds and publishes a ResourceSlice.
func (s *Service) PublishOnce(ctx context.Context, resourceExists bool) error {
	snapshot, err := s.inventory.Snapshot(ctx)
	if err != nil {
		return err
	}

	slice := s.buildSlice(ctx, snapshot, resourceExists)
	return s.writer.Publish(ctx, slice)
}

func (s *Service) buildSlice(ctx context.Context, snapshot domain.InventorySnapshot, resourceExists bool) *resourcev1.ResourceSlice {
	nodeName := snapshot.NodeName
	if nodeName == "" {
		nodeName = unknownNodeKey
	}

	slice := &resourcev1.ResourceSlice{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "resource.k8s.io/v1",
			Kind:       "ResourceSlice",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("gpu-%s", nodeName),
		},
		Spec: resourcev1.ResourceSliceSpec{
			Driver: driverName,
			Pool: resourcev1.ResourcePool{
				Name:               defaultPool,
				Generation:         1,
				ResourceSliceCount: 1,
			},
			NodeName: func() *string {
				if snapshot.NodeName == "" {
					return nil
				}
				return &snapshot.NodeName
			}(),
			AllNodes: func() *bool {
				if snapshot.NodeName != "" {
					return nil
				}
				return boolPtr(true)
			}(),
		},
	}

	builder := resource_builder.NewResourceBuilder(slice, resource_builder.ResourceBuilderOptions{ResourceExists: resourceExists})
	builder.AddAnnotation(annotationKey, driverName)
	builder.AddFinalizer(finalizerName)
	if snapshot.NodeUID != "" && snapshot.NodeName != "" {
		builder.SetOwnerRef(&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: snapshot.NodeName,
				UID:  snapshot.NodeUID,
			},
		}, corev1.SchemeGroupVersion.WithKind("Node"))
	}

	action := "create"
	if builder.IsResourceExists() {
		action = "update"
	}
	logger.FromContext(ctx).Debug("resource slice prepared", "action", action, "name", slice.Name)

	return builder.GetResource()
}

func boolPtr(v bool) *bool {
	return &v
}
