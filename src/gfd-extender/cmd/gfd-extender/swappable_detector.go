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
	"sync"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/src/gfd-extender/internal/server"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/src/gfd-extender/pkg/detect"
)

type swappableDetector struct {
	mu       sync.RWMutex
	current  server.Detector
	closable closableDetector
	initErr  error
}

func newSwappableDetector() *swappableDetector {
	return &swappableDetector{}
}

func (s *swappableDetector) DetectGPU(ctx context.Context) ([]detect.Info, error) {
	s.mu.RLock()
	cur := s.current
	err := s.initErr
	s.mu.RUnlock()

	if cur != nil {
		return cur.DetectGPU(ctx)
	}
	if err != nil {
		return nil, err
	}
	return nil, errors.New("detector not initialized")
}

func (s *swappableDetector) set(det closableDetector) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closable != nil {
		_ = s.closable.Close()
	}
	s.closable = det
	s.current = det
	s.initErr = nil
}

func (s *swappableDetector) setError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closable != nil {
		_ = s.closable.Close()
	}
	s.closable = nil
	s.current = nil
	s.initErr = err
}

func (s *swappableDetector) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closable != nil {
		err := s.closable.Close()
		s.closable = nil
		s.current = nil
		return err
	}
	return nil
}
