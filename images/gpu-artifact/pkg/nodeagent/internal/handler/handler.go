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

package handler

import (
	"context"
	"errors"
	"fmt"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/nodeagent/internal/state"
)

// Handler performs a single action against the node-agent state.
type Handler interface {
	Name() string
	Handle(ctx context.Context, st state.State) error
}

// ErrStopHandlerChain stops handler execution immediately.
var ErrStopHandlerChain = errors.New("stop handler chain")

// StopHandlerChain wraps an error to stop the handler chain.
func StopHandlerChain(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrStopHandlerChain) {
		return err
	}
	return fmt.Errorf("%w: %v", ErrStopHandlerChain, err)
}
