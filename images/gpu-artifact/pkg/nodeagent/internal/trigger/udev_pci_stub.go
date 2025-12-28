//go:build !linux
// +build !linux

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

package trigger

import (
	"context"

	"github.com/deckhouse/deckhouse/pkg/log"
)

// UdevPCI is a stub for non-linux builds.
type UdevPCI struct {
	log *log.Logger
}

// NewUdevPCI constructs a no-op watcher for non-linux environments.
func NewUdevPCI(log *log.Logger) *UdevPCI {
	return &UdevPCI{log: log}
}

// Run blocks until context is done and does not emit events.
func (u *UdevPCI) Run(ctx context.Context, _ NotifyFunc) error {
	if u.log != nil {
		u.log.Info("udev pci watcher is disabled on non-linux")
	}
	<-ctx.Done()
	return nil
}
