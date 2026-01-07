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

package fslock

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

const defaultRetryDelay = 100 * time.Millisecond

// Locker serializes prepare/unprepare operations.
type Locker struct {
	path       string
	retryDelay time.Duration
}

// New constructs a file-based locker.
func New(path string) *Locker {
	return &Locker{path: path, retryDelay: defaultRetryDelay}
}

// Lock acquires an exclusive lock and returns a release function.
func (l *Locker) Lock(ctx context.Context) (func() error, error) {
	if l == nil || l.path == "" {
		return nil, errors.New("lock path is not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	file, err := os.OpenFile(l.path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}

	for {
		if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err == nil {
			break
		} else if !errors.Is(err, unix.EWOULDBLOCK) {
			_ = file.Close()
			return nil, fmt.Errorf("acquire lock: %w", err)
		}

		select {
		case <-ctx.Done():
			_ = file.Close()
			return nil, ctx.Err()
		case <-time.After(l.retryDelay):
		}
	}

	release := func() error {
		lockErr := unix.Flock(int(file.Fd()), unix.LOCK_UN)
		closeErr := file.Close()
		if lockErr != nil {
			return fmt.Errorf("release lock: %w", lockErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close lock file: %w", closeErr)
		}
		return nil
	}

	return release, nil
}
