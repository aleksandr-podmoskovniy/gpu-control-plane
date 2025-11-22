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

package main

import (
	"context"
	"errors"
	"testing"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/src/gfd-extender/pkg/detect"
)

type closableDetectorStub struct {
	detectErr error
	closeErr  error
	closed    bool
}

func (c *closableDetectorStub) DetectGPU(context.Context) ([]detect.Info, error) {
	return nil, c.detectErr
}

func (c *closableDetectorStub) Close() error {
	c.closed = true
	return c.closeErr
}

func TestSwappableDetectorDetectBeforeInit(t *testing.T) {
	s := newSwappableDetector()
	if _, err := s.DetectGPU(context.Background()); err == nil {
		t.Fatalf("expected error before init")
	}
}

func TestSwappableDetectorSetAndDetect(t *testing.T) {
	s := newSwappableDetector()
	det := &closableDetectorStub{}
	s.set(det)
	if _, err := s.DetectGPU(context.Background()); err != nil {
		t.Fatalf("unexpected detect error: %v", err)
	}
}

func TestSwappableDetectorSetError(t *testing.T) {
	s := newSwappableDetector()
	prev := &closableDetectorStub{}
	s.set(prev)

	s.setError(errors.New("boom"))

	if !prev.closed {
		t.Fatalf("previous detector must be closed on error")
	}
	if _, err := s.DetectGPU(context.Background()); err == nil {
		t.Fatalf("expected error after setError")
	}
}

func TestSwappableDetectorClose(t *testing.T) {
	s := newSwappableDetector()
	det := &closableDetectorStub{}
	s.set(det)
	if err := s.Close(); err != nil {
		t.Fatalf("unexpected close error: %v", err)
	}
	if !det.closed {
		t.Fatalf("detector should be closed")
	}
	// Closing twice must be safe.
	if err := s.Close(); err != nil {
		t.Fatalf("second close should be noop, got %v", err)
	}
}
