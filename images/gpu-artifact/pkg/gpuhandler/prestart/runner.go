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

package prestart

import (
	"io"
	"time"
)

const (
	defaultDriverRootMount       = "/driver-root"
	defaultDriverRootParentMount = "/driver-root-parent"
	defaultWaitInterval          = 10 * time.Second
	defaultHintEvery             = 6
)

// Options configures the prestart runner.
type Options struct {
	DriverRoot            string
	DriverRootMount       string
	DriverRootParentMount string
	WaitInterval          time.Duration
	HintEvery             int
	Now                   func() time.Time
	Out                   io.Writer
	Err                   io.Writer
	FS                    FS
	Exec                  ExecFunc
}

// Runner executes driver readiness checks until they pass.
type Runner struct {
	driverRoot            string
	driverRootMount       string
	driverRootParentMount string
	waitInterval          time.Duration
	hintEvery             int
	now                   func() time.Time
	out                   io.Writer
	err                   io.Writer
	fs                    FS
	exec                  ExecFunc
}

// NewRunner returns a runner configured with sensible defaults.
func NewRunner(opts Options) *Runner {
	driverRoot := opts.DriverRoot
	if driverRoot == "" {
		driverRoot = "/"
	}
	driverRootMount := opts.DriverRootMount
	if driverRootMount == "" {
		driverRootMount = defaultDriverRootMount
	}
	driverRootParentMount := opts.DriverRootParentMount
	if driverRootParentMount == "" {
		driverRootParentMount = defaultDriverRootParentMount
	}
	waitInterval := opts.WaitInterval
	if waitInterval == 0 {
		waitInterval = defaultWaitInterval
	}
	hintEvery := opts.HintEvery
	if hintEvery == 0 {
		hintEvery = defaultHintEvery
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	out := opts.Out
	if out == nil {
		out = io.Discard
	}
	err := opts.Err
	if err == nil {
		err = out
	}
	fs := opts.FS
	if fs == nil {
		fs = osFS{}
	}
	exec := opts.Exec
	if exec == nil {
		exec = execCommand
	}

	return &Runner{
		driverRoot:            driverRoot,
		driverRootMount:       driverRootMount,
		driverRootParentMount: driverRootParentMount,
		waitInterval:          waitInterval,
		hintEvery:             hintEvery,
		now:                   now,
		out:                   out,
		err:                   err,
		fs:                    fs,
		exec:                  exec,
	}
}
