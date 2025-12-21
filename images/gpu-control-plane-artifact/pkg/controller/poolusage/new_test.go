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

package poolusage

import (
	"testing"

	"github.com/go-logr/logr"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/config"
)

func TestNewGPUPoolUsage_DefaultWorkers(t *testing.T) {
	r := NewGPUPoolUsage(logr.Discard(), config.ControllerConfig{Workers: 0}, nil)
	if r.cfg.Workers != 1 {
		t.Fatalf("expected workers to default to 1, got %d", r.cfg.Workers)
	}

	r = NewGPUPoolUsage(logr.Discard(), config.ControllerConfig{Workers: 3}, nil)
	if r.cfg.Workers != 3 {
		t.Fatalf("expected workers to stay at 3, got %d", r.cfg.Workers)
	}
}

func TestNewClusterGPUPoolUsage_DefaultWorkers(t *testing.T) {
	r := NewClusterGPUPoolUsage(logr.Discard(), config.ControllerConfig{Workers: -1}, nil)
	if r.cfg.Workers != 1 {
		t.Fatalf("expected workers to default to 1, got %d", r.cfg.Workers)
	}

	r = NewClusterGPUPoolUsage(logr.Discard(), config.ControllerConfig{Workers: 2}, nil)
	if r.cfg.Workers != 2 {
		t.Fatalf("expected workers to stay at 2, got %d", r.cfg.Workers)
	}
}
