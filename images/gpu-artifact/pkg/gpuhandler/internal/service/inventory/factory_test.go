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

package inventory

import (
	"errors"
	"testing"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
)

func TestDefaultFactoryNoMig(t *testing.T) {
	factory := NewDefaultFactory(nil)
	plan := factory.Build([]gpuv1alpha1.PhysicalGPU{newPGPU(nil)})

	if len(plan.Errs) != 0 {
		t.Fatalf("expected no errors, got %v", plan.Errs)
	}
	if len(plan.Builders) != 1 {
		t.Fatalf("expected 1 builder, got %d", len(plan.Builders))
	}
	if _, ok := plan.Builders[0].(*PhysicalDeviceBuilder); !ok {
		t.Fatalf("expected PhysicalDeviceBuilder, got %T", plan.Builders[0])
	}
	if plan.Context.MigSession != nil {
		t.Fatalf("expected no MIG session, got %v", plan.Context.MigSession)
	}
	if plan.Close != nil {
		t.Fatalf("expected no Close callback")
	}
}

func TestDefaultFactoryMigSupportedNoPlacements(t *testing.T) {
	mig := true
	factory := NewDefaultFactory(nil)
	plan := factory.Build([]gpuv1alpha1.PhysicalGPU{newPGPU(&mig)})

	if len(plan.Errs) == 0 {
		t.Fatalf("expected errors when MIG placements reader is missing")
	}
	if len(plan.Builders) != 1 {
		t.Fatalf("expected 1 builder, got %d", len(plan.Builders))
	}
	if _, ok := plan.Builders[0].(*PhysicalDeviceBuilder); !ok {
		t.Fatalf("expected PhysicalDeviceBuilder, got %T", plan.Builders[0])
	}
}

func TestDefaultFactoryMigSupportedWithPlacements(t *testing.T) {
	mig := true
	session := &fakeMigSession{}
	factory := NewDefaultFactory(&fakeMigReader{session: session})
	plan := factory.Build([]gpuv1alpha1.PhysicalGPU{newPGPU(&mig)})

	if len(plan.Errs) != 0 {
		t.Fatalf("expected no errors, got %v", plan.Errs)
	}
	if len(plan.Builders) != 2 {
		t.Fatalf("expected 2 builders, got %d", len(plan.Builders))
	}
	if _, ok := plan.Builders[0].(*PhysicalDeviceBuilder); !ok {
		t.Fatalf("expected PhysicalDeviceBuilder, got %T", plan.Builders[0])
	}
	if _, ok := plan.Builders[1].(*MigDeviceBuilder); !ok {
		t.Fatalf("expected MigDeviceBuilder, got %T", plan.Builders[1])
	}
	if plan.Context.MigSession == nil {
		t.Fatalf("expected MIG session to be set")
	}
	if plan.Close == nil {
		t.Fatalf("expected Close callback to be set")
	}

	plan.Close()
	if !session.closed {
		t.Fatalf("expected Close to close the session")
	}
}

type fakeMigReader struct {
	session MigPlacementSession
	err     error
}

func (f *fakeMigReader) Open() (MigPlacementSession, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.session == nil {
		return nil, errors.New("no session")
	}
	return f.session, nil
}

type fakeMigSession struct {
	closed bool
}

func (s *fakeMigSession) Close() {
	s.closed = true
}

func (s *fakeMigSession) ReadPlacements(_ string, _ []int32) (map[int32][]MigPlacement, error) {
	return map[int32][]MigPlacement{}, nil
}

func newPGPU(migSupported *bool) gpuv1alpha1.PhysicalGPU {
	return gpuv1alpha1.PhysicalGPU{
		Status: gpuv1alpha1.PhysicalGPUStatus{
			Capabilities: &gpuv1alpha1.GPUCapabilities{
				Vendor: gpuv1alpha1.VendorNvidia,
				Nvidia: &gpuv1alpha1.NvidiaCapabilities{
					MIGSupported: migSupported,
				},
			},
		},
	}
}
