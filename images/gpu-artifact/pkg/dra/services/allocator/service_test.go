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

package allocator

import (
	"context"
	"testing"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
)

func TestAllocateExactCount(t *testing.T) {
	t.Parallel()

	svc := NewService()
	alloc, err := svc.Allocate(context.Background(), Input{
		Requests: []Request{{
			Name:      "gpu",
			Count:     1,
			Selectors: []Selector{allowAllSelector{}},
		}},
		Candidates: []CandidateDevice{{
			Key:      DeviceKey{Driver: DefaultDriverName, Pool: "pool-a", Device: "dev-1"},
			Driver:   DefaultDriverName,
			Pool:     "pool-a",
			NodeName: "node-1",
			Spec: allocatable.DeviceSpec{
				Name: "dev-1",
			},
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if alloc == nil {
		t.Fatalf("expected allocation result")
	}
	if len(alloc.Devices) != 1 {
		t.Fatalf("expected 1 device result, got %d", len(alloc.Devices))
	}
	got := alloc.Devices[0]
	if got.Driver != DefaultDriverName || got.Pool != "pool-a" || got.Device != "dev-1" {
		t.Fatalf("unexpected allocation result: %#v", got)
	}
	if alloc.NodeName != "node-1" {
		t.Fatalf("expected node-1, got %q", alloc.NodeName)
	}
}

func TestAllocateSkipsWhenNoMatch(t *testing.T) {
	t.Parallel()

	svc := NewService()
	alloc, err := svc.Allocate(context.Background(), Input{
		Requests: []Request{{
			Name:      "gpu",
			Count:     1,
			Selectors: []Selector{rejectAllSelector{}},
		}},
		Candidates: []CandidateDevice{{
			Key:      DeviceKey{Driver: DefaultDriverName, Pool: "pool-a", Device: "dev-1"},
			Driver:   DefaultDriverName,
			Pool:     "pool-a",
			NodeName: "node-1",
			Spec: allocatable.DeviceSpec{
				Name: "dev-1",
			},
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if alloc != nil {
		t.Fatalf("expected no allocation when selector rejects devices")
	}
}

type allowAllSelector struct{}

func (allowAllSelector) Match(context.Context, string, allocatable.DeviceSpec) (bool, error) {
	return true, nil
}

type rejectAllSelector struct{}

func (rejectAllSelector) Match(context.Context, string, allocatable.DeviceSpec) (bool, error) {
	return false, nil
}
