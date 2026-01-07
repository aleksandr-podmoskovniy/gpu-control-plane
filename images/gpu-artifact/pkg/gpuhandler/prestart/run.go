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
	"path"
	"path/filepath"
	"strings"
	"time"
)

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
