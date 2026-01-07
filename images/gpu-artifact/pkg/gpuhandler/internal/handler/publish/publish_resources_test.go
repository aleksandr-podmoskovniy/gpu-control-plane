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

package publish

import (
	"context"
	"errors"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/dynamic-resource-allocation/resourceslice"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/state"
)

type fakeBuilder struct {
	resources resourceslice.DriverResources
	err       error
	got       []gpuv1alpha1.PhysicalGPU
}

func (b *fakeBuilder) Build(_ context.Context, _ string, devices []gpuv1alpha1.PhysicalGPU) (resourceslice.DriverResources, error) {
	b.got = append([]gpuv1alpha1.PhysicalGPU{}, devices...)
	return b.resources, b.err
}

type fakePublisher struct {
	err error
}

func (p fakePublisher) PublishResources(_ context.Context, _ resourceslice.DriverResources) error {
	return p.err
}

func TestPublishResourcesUsesReadyList(t *testing.T) {
	healthy := sampleGPU("pgpu-healthy")
	unhealthy := sampleGPU("pgpu-bad")
	st := state.New("node-1")
	st.SetReady([]gpuv1alpha1.PhysicalGPU{healthy, unhealthy})

	builder := &fakeBuilder{}
	h := NewPublishResourcesHandler(builder, fakePublisher{}, nil, nil)
	if err := h.Handle(context.Background(), st); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(builder.got) != 2 {
		t.Fatalf("unexpected ready list size: %d", len(builder.got))
	}
	if builder.got[0].Name != "pgpu-healthy" || builder.got[1].Name != "pgpu-bad" {
		t.Fatalf("unexpected ready list: %+v", builder.got)
	}
}

func TestPublishResourcesJoinsErrors(t *testing.T) {
	st := state.New("node-1")
	st.SetReady([]gpuv1alpha1.PhysicalGPU{sampleGPU("pgpu-1")})

	buildErr := errors.New("build failed")
	publishErr := errors.New("publish failed")
	h := NewPublishResourcesHandler(&fakeBuilder{err: buildErr}, fakePublisher{err: publishErr}, nil, nil)

	err := h.Handle(context.Background(), st)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, buildErr) || !errors.Is(err, publishErr) {
		t.Fatalf("expected joined errors, got %v", err)
	}
}

func TestPublishResourcesNilDeps(t *testing.T) {
	st := state.New("node-1")
	st.SetReady([]gpuv1alpha1.PhysicalGPU{sampleGPU("pgpu-1")})

	h := &PublishResourcesHandler{}
	if err := h.Handle(context.Background(), st); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func sampleGPU(name string) gpuv1alpha1.PhysicalGPU {
	return gpuv1alpha1.PhysicalGPU{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}
}
