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
	"time"
)

const defaultTimeout = time.Second

var collect = queryNVML

// Option configures a Client.
type Option func(c *Client)

// Client calls NVML to query GPUs.
type Client struct {
	Timeout time.Duration
}

// NewClient constructs a Client.
func NewClient(opts ...Option) *Client {
	c := &Client{Timeout: defaultTimeout}
	for _, apply := range opts {
		apply(c)
	}
	return c
}

// WithTimeout overrides the NVML call timeout.
func WithTimeout(timeout time.Duration) Option {
	return func(c *Client) {
		if timeout > 0 {
			c.Timeout = timeout
		}
	}
}

// Init initializes NVML bindings.
func (c *Client) Init() error { return initNVML() }

// Close releases NVML resources.
func (c *Client) Close() error { return shutdownNVML() }

// DetectGPU returns the current set of GPUs or propagates context cancellation.
func (c *Client) DetectGPU(ctx context.Context) ([]Info, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var cancel context.CancelFunc
	if c.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, c.Timeout)
		defer cancel()
	}

	resultCh := make(chan []Info, 1)
	errCh := make(chan error, 1)
	go func() {
		infos, err := collect()
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- infos
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-errCh:
		return nil, err
	case infos := <-resultCh:
		return infos, nil
	}
}

// queryNVML, initNVML and shutdownNVML are implemented in platform-specific files.
