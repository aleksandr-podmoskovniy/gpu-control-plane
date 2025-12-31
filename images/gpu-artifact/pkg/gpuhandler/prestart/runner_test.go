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
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCheckOnceMissingBinariesHintsEverySix(t *testing.T) {
	t.Parallel()

	driverRootMount, parentMount := newTempRoots(t)
	out := &bytes.Buffer{}

	runner := NewRunner(Options{
		DriverRoot:            "/run/nvidia/driver",
		DriverRootMount:       driverRootMount,
		DriverRootParentMount: parentMount,
		Now:                   fixedNow,
		Out:                   out,
		Err:                   out,
		Exec:                  stubExec(0, nil),
	})

	if runner.checkOnce(context.Background(), 0) {
		t.Fatalf("expected check to fail")
	}

	logs := out.String()
	assertContains(t, logs, "nvidia-smi: not found")
	assertContains(t, logs, "libnvidia-ml.so.1: not found")
	assertContains(t, logs, "Check failed.")
	assertContains(t, logs, "Hint: Directory /run/nvidia/driver on the host is empty")
	assertContains(t, logs, "NVIDIA_DRIVER_ROOT is set to '/run/nvidia/driver'")

	out.Reset()
	if runner.checkOnce(context.Background(), 1) {
		t.Fatalf("expected check to fail")
	}
	if strings.Contains(out.String(), "Check failed.") {
		t.Fatalf("unexpected hint output on attempt 1")
	}
}

func TestCheckOnceSuccess(t *testing.T) {
	t.Parallel()

	driverRootMount, parentMount := newTempRoots(t)
	out := &bytes.Buffer{}

	writeFile(t, filepath.Join(driverRootMount, "usr/bin/nvidia-smi"))
	writeFile(t, filepath.Join(driverRootMount, "usr/lib/x86_64-linux-gnu/libnvidia-ml.so.1"))

	runner := NewRunner(Options{
		DriverRoot:            "/",
		DriverRootMount:       driverRootMount,
		DriverRootParentMount: parentMount,
		Now:                   fixedNow,
		Out:                   out,
		Err:                   out,
		Exec:                  stubExec(0, nil),
	})

	if !runner.checkOnce(context.Background(), 0) {
		t.Fatalf("expected check to succeed")
	}

	logs := out.String()
	assertContains(t, logs, "invoke: env -i LD_PRELOAD=")
	assertContains(t, logs, "nvidia-smi returned with code 0: success, leave")
}

func TestCheckOnceNonZeroExit(t *testing.T) {
	t.Parallel()

	driverRootMount, parentMount := newTempRoots(t)
	out := &bytes.Buffer{}

	writeFile(t, filepath.Join(driverRootMount, "usr/bin/nvidia-smi"))
	writeFile(t, filepath.Join(driverRootMount, "usr/lib/x86_64-linux-gnu/libnvidia-ml.so.1"))

	runner := NewRunner(Options{
		DriverRoot:            "/",
		DriverRootMount:       driverRootMount,
		DriverRootParentMount: parentMount,
		Now:                   fixedNow,
		Out:                   out,
		Err:                   out,
		Exec:                  stubExec(7, nil),
	})

	if runner.checkOnce(context.Background(), 0) {
		t.Fatalf("expected check to fail")
	}

	assertContains(t, out.String(), "exit code: 7")
}

func TestCheckOnceHintForOperatorDriverRoot(t *testing.T) {
	t.Parallel()

	driverRootMount, parentMount := newTempRoots(t)
	out := &bytes.Buffer{}

	writeFile(t, filepath.Join(driverRootMount, "run/nvidia/driver/usr/bin/nvidia-smi"))

	runner := NewRunner(Options{
		DriverRoot:            "/",
		DriverRootMount:       driverRootMount,
		DriverRootParentMount: parentMount,
		Now:                   fixedNow,
		Out:                   out,
		Err:                   out,
		Exec:                  stubExec(0, nil),
	})

	if runner.checkOnce(context.Background(), 0) {
		t.Fatalf("expected check to fail")
	}

	assertContains(t, out.String(), "may want to re-install the DRA driver Helm chart")
}

func newTempRoots(t *testing.T) (string, string) {
	t.Helper()

	tmp := t.TempDir()
	driverRootMount := filepath.Join(tmp, "driver-root")
	parentMount := filepath.Join(tmp, "driver-root-parent")

	if err := os.MkdirAll(driverRootMount, 0o755); err != nil {
		t.Fatalf("mkdir driver-root: %v", err)
	}
	if err := os.MkdirAll(parentMount, 0o755); err != nil {
		t.Fatalf("mkdir driver-root-parent: %v", err)
	}

	return driverRootMount, parentMount
}

func writeFile(t *testing.T, path string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

func assertContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("expected logs to contain %q, got %q", needle, haystack)
	}
}

func stubExec(code int, err error) ExecFunc {
	return func(ctx context.Context, path string, env []string, stdout, stderr io.Writer) (int, error) {
		_ = ctx
		_ = path
		_ = env
		_ = stdout
		_ = stderr
		return code, err
	}
}

func fixedNow() time.Time {
	return time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
}
