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

package state

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"

	nfdv1alpha1 "sigs.k8s.io/node-feature-discovery/api/nfd/v1alpha1"
)

func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add gpu scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	if err := nfdv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add nfd scheme: %v", err)
	}
	scheme.AddKnownTypes(
		nfdv1alpha1.SchemeGroupVersion,
		&nfdv1alpha1.NodeFeatureList{},
		&nfdv1alpha1.NodeFeatureRuleList{},
		&nfdv1alpha1.NodeFeatureGroupList{},
	)
	return scheme
}

func newTestClient(scheme *runtime.Scheme, objs ...client.Object) client.Client {
	builder := clientfake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUDevice{}, &v1alpha1.GPUNodeState{}).
		WithObjects(objs...)

	builder = builder.WithIndex(&v1alpha1.GPUDevice{}, DeviceNodeIndexKey, func(obj client.Object) []string {
		device, ok := obj.(*v1alpha1.GPUDevice)
		if !ok || device.Status.NodeName == "" {
			return nil
		}
		return []string{device.Status.NodeName}
	})

	return builder.Build()
}

type delegatingClient struct {
	client.Client
	get  func(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error
	list func(context.Context, client.ObjectList, ...client.ListOption) error
}

func (d *delegatingClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if d.get != nil {
		return d.get(ctx, key, obj, opts...)
	}
	return d.Client.Get(ctx, key, obj, opts...)
}

func (d *delegatingClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if d.list != nil {
		return d.list(ctx, list, opts...)
	}
	return d.Client.List(ctx, list, opts...)
}
