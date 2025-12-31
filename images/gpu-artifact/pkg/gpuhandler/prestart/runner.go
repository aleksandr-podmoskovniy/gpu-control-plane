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
	"context"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"strings"
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

// Run checks driver readiness until it succeeds or the context is canceled.
func (r *Runner) Run(ctx context.Context) error {
	target := r.driverRootSymlinkTarget()
	r.logf("create symlink: %s -> %s\n", r.driverRootMount, target)
	if err := r.fs.Symlink(target, r.driverRootMount); err != nil {
		r.errf("ln: %s: %v\n", r.driverRootMount, err)
	}

	attempt := 0
	for {
		if r.checkOnce(ctx, attempt) {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(r.waitInterval):
		}
		attempt++
	}
}

func (r *Runner) checkOnce(ctx context.Context, attempt int) bool {
	timestamp := r.now().UTC().Format("2006-01-02T15:04:05Z")
	r.logf("%s  %s (%s on host): ", timestamp, r.driverRootMount, r.driverRoot)

	result := r.probe()

	if result.NvidiaSMIPath == "" {
		r.logf("nvidia-smi: not found, ")
	} else {
		r.logf("nvidia-smi: '%s', ", result.NvidiaSMIPath)
	}

	if result.NVMLLibPath == "" {
		r.logf("libnvidia-ml.so.1: not found, ")
	} else {
		r.logf("libnvidia-ml.so.1: '%s', ", result.NVMLLibPath)
	}

	r.logf("current contents: [%s].\n", result.DriverRootContents)

	if result.CanInvoke() {
		r.logf("invoke: env -i LD_PRELOAD=%s %s\n", result.NVMLLibPath, result.NvidiaSMIPath)
		code, err := r.exec(ctx, result.NvidiaSMIPath, []string{fmt.Sprintf("LD_PRELOAD=%s", result.NVMLLibPath)}, r.out, r.err)
		if err != nil && code == 0 {
			code = 1
		}
		if code == 0 {
			r.logf("nvidia-smi returned with code 0: success, leave\n")
			return true
		}
		r.logf("exit code: %d\n", code)
	}

	r.emitHints(result, attempt)
	return false
}

func (r *Runner) driverRootSymlinkTarget() string {
	rootTrim := strings.TrimSuffix(r.driverRoot, "/")
	base := path.Base(rootTrim)
	return filepath.Join(r.driverRootParentMount, base)
}

func (r *Runner) logf(format string, args ...any) {
	fmt.Fprintf(r.out, format, args...)
}

func (r *Runner) errf(format string, args ...any) {
	fmt.Fprintf(r.err, format, args...)
}
