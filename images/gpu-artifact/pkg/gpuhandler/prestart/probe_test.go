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
	"path/filepath"
	"testing"
)

func TestProbeFindsBinariesAndContents(t *testing.T) {
	t.Parallel()

	driverRootMount, parentMount := newTempRoots(t)
	writeFile(t, filepath.Join(driverRootMount, "usr/bin/nvidia-smi"))
	writeFile(t, filepath.Join(driverRootMount, "usr/lib/x86_64-linux-gnu/libnvidia-ml.so.1"))

	runner := NewRunner(Options{
		DriverRoot:            "/",
		DriverRootMount:       driverRootMount,
		DriverRootParentMount: parentMount,
	})

	result := runner.probe()
	if result.NvidiaSMIPath != filepath.Join(driverRootMount, "usr/bin/nvidia-smi") {
		t.Fatalf("unexpected nvidia-smi path: %q", result.NvidiaSMIPath)
	}
	if result.NVMLLibPath != filepath.Join(driverRootMount, "usr/lib/x86_64-linux-gnu/libnvidia-ml.so.1") {
		t.Fatalf("unexpected nvml path: %q", result.NVMLLibPath)
	}
	if result.DriverRootEmpty {
		t.Fatalf("expected driver root to be non-empty")
	}
	if result.DriverRootContents != "usr" {
		t.Fatalf("unexpected driver root contents: %q", result.DriverRootContents)
	}
}

func TestProbeDetectsOperatorSMI(t *testing.T) {
	t.Parallel()

	driverRootMount, parentMount := newTempRoots(t)
	writeFile(t, filepath.Join(driverRootMount, "run/nvidia/driver/usr/bin/nvidia-smi"))

	runner := NewRunner(Options{
		DriverRoot:            "/",
		DriverRootMount:       driverRootMount,
		DriverRootParentMount: parentMount,
	})

	result := runner.probe()
	if result.OperatorSMIDetected == false {
		t.Fatalf("expected operator nvidia-smi detection")
	}
	if result.DriverRootContents != "run" {
		t.Fatalf("unexpected driver root contents: %q", result.DriverRootContents)
	}
}

func TestProbeEmptyRoot(t *testing.T) {
	t.Parallel()

	driverRootMount, parentMount := newTempRoots(t)
	runner := NewRunner(Options{
		DriverRoot:            "/",
		DriverRootMount:       driverRootMount,
		DriverRootParentMount: parentMount,
	})

	result := runner.probe()
	if !result.DriverRootEmpty {
		t.Fatalf("expected driver root to be empty")
	}
	if result.DriverRootContents != "" {
		t.Fatalf("expected empty contents, got %q", result.DriverRootContents)
	}
}
