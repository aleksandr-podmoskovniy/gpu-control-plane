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

package detect

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDetectGPUTimeout(t *testing.T) {
	t.Cleanup(func() { collect = queryNVML })
	collect = func() ([]Info, error) {
		time.Sleep(200 * time.Millisecond)
		return nil, nil
	}

	client := NewClient(WithTimeout(50 * time.Millisecond))
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	if _, err := client.DetectGPU(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}

func TestDetectGPUSuccess(t *testing.T) {
	t.Cleanup(func() { collect = queryNVML })
	expected := []Info{{Index: 1, UUID: "gpu-1"}}
	collect = func() ([]Info, error) {
		return expected, nil
	}

	client := NewClient(WithTimeout(time.Second))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	infos, err := client.DetectGPU(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(infos) != 1 || infos[0].UUID != "gpu-1" {
		t.Fatalf("unexpected results: %+v", infos)
	}
}

func TestDetectGPUCollectError(t *testing.T) {
	t.Cleanup(func() { collect = queryNVML })
	collect = func() ([]Info, error) {
		return nil, errors.New("boom")
	}
	client := NewClient()
	if _, err := client.DetectGPU(context.Background()); err == nil {
		t.Fatalf("expected error, got nil")
	}
}

func TestDetectGPUNilContext(t *testing.T) {
	t.Cleanup(func() { collect = queryNVML })
	collect = func() ([]Info, error) {
		return []Info{{Index: 0}}, nil
	}
	client := NewClient()
	result, err := client.DetectGPU(nil)
	if err != nil || len(result) != 1 {
		t.Fatalf("expected success with nil context")
	}
}

func TestClientInitClose(t *testing.T) {
	c := NewClient()
	// On non-linux build tags initNVML returns error; we only assert that Close never errors.
	_ = c.Init()
	if err := c.Close(); err != nil {
		t.Fatalf("expected close to be noop, got %v", err)
	}
}

func TestNewClientDefaultTimeout(t *testing.T) {
	c := NewClient()
	if c.Timeout != defaultTimeout {
		t.Fatalf("expected default timeout %s, got %s", defaultTimeout, c.Timeout)
	}
	c = NewClient(WithTimeout(-time.Second))
	if c.Timeout != defaultTimeout {
		t.Fatalf("negative timeout should keep default, got %s", c.Timeout)
	}
}

func TestQueryNVMLStub(t *testing.T) {
	if _, err := queryNVML(); err == nil {
		t.Fatalf("expected stub to fail on this platform")
	}
}
