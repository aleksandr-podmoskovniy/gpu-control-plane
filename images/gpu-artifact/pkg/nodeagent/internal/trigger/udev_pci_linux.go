//go:build linux
// +build linux

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
	"fmt"

	"golang.org/x/sys/unix"

	"github.com/deckhouse/deckhouse/pkg/log"
)

// UdevPCI listens for PCI udev events and triggers sync.
type UdevPCI struct {
	log *log.Logger
}

// NewUdevPCI constructs a PCI udev watcher.
func NewUdevPCI(log *log.Logger) *UdevPCI {
	return &UdevPCI{log: log}
}

// Run starts the udev event loop.
func (u *UdevPCI) Run(ctx context.Context, notify NotifyFunc) error {
	fd, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_RAW, unix.NETLINK_KOBJECT_UEVENT)
	if err != nil {
		return fmt.Errorf("open uevent socket: %w", err)
	}
	defer func() {
		_ = unix.Close(fd)
	}()

	if err := unix.Bind(fd, &unix.SockaddrNetlink{Family: unix.AF_NETLINK, Groups: 1}); err != nil {
		return fmt.Errorf("bind uevent socket: %w", err)
	}

	closeCh := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = unix.Close(fd)
		case <-closeCh:
		}
	}()
	defer close(closeCh)

	buf := make([]byte, 64*1024)
	for {
		n, _, err := unix.Recvfrom(fd, buf, 0)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("read uevent: %w", err)
		}

		env := parseUEvent(buf[:n])
		if env["SUBSYSTEM"] != "pci" {
			continue
		}
		notify()
	}
}
