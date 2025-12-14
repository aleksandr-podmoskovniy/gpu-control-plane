// Copyright 2025 Flant JSC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gpupool

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/indexer"
)

// listErrorClient injects a list error for specific list types (keyed by fmt.Sprintf("%T", list)).
type listErrorClient struct {
	client.Client
	errs map[string]error
}

func (c *listErrorClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if err, ok := c.errs[fmt.Sprintf("%T", list)]; ok {
		return err
	}
	return c.Client.List(ctx, list, opts...)
}

// statusErrorClient injects an error for status operations.
type statusErrorClient struct {
	client.Client
	err error
}

func (c *statusErrorClient) Status() client.StatusWriter {
	return &statusErrorWriter{StatusWriter: c.Client.Status(), err: c.err}
}

type statusErrorWriter struct {
	client.StatusWriter
	err error
}

func (w *statusErrorWriter) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	return w.err
}

func (w *statusErrorWriter) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	return w.err
}

// selectiveStatusErrorClient injects status errors only for specific object names.
type selectiveStatusErrorClient struct {
	client.Client
	errs map[string]error
}

func (c *selectiveStatusErrorClient) Status() client.StatusWriter {
	return &selectiveStatusWriter{StatusWriter: c.Client.Status(), errs: c.errs}
}

type selectiveStatusWriter struct {
	client.StatusWriter
	errs map[string]error
}

func (w *selectiveStatusWriter) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	if err, ok := w.errs[obj.GetName()]; ok {
		return err
	}
	return w.StatusWriter.Patch(ctx, obj, patch, opts...)
}

func (w *selectiveStatusWriter) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	if err, ok := w.errs[obj.GetName()]; ok {
		return err
	}
	return w.StatusWriter.Update(ctx, obj, opts...)
}

func withPoolDeviceIndexes(builder *fake.ClientBuilder) *fake.ClientBuilder {
	if builder == nil {
		return nil
	}

	builder = builder.WithIndex(&v1alpha1.GPUDevice{}, indexer.GPUDevicePoolRefNameField, func(obj client.Object) []string {
		dev, ok := obj.(*v1alpha1.GPUDevice)
		if !ok || dev.Status.PoolRef == nil || dev.Status.PoolRef.Name == "" {
			return nil
		}
		return []string{dev.Status.PoolRef.Name}
	})

	builder = builder.WithIndex(&v1alpha1.GPUDevice{}, indexer.GPUDeviceNamespacedAssignmentField, func(obj client.Object) []string {
		dev, ok := obj.(*v1alpha1.GPUDevice)
		if !ok || dev.Annotations == nil {
			return nil
		}
		if value := dev.Annotations[namespacedAssignmentAnnotation]; value != "" {
			return []string{value}
		}
		return nil
	})

	builder = builder.WithIndex(&v1alpha1.GPUDevice{}, indexer.GPUDeviceClusterAssignmentField, func(obj client.Object) []string {
		dev, ok := obj.(*v1alpha1.GPUDevice)
		if !ok || dev.Annotations == nil {
			return nil
		}
		if value := dev.Annotations[clusterAssignmentAnnotation]; value != "" {
			return []string{value}
		}
		return nil
	})

	return builder
}
