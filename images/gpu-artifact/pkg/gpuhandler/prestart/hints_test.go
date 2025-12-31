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
	"strings"
	"testing"
)

func TestEmitHintsOnlyOnInterval(t *testing.T) {
	t.Parallel()

	out := &bytes.Buffer{}
	runner := NewRunner(Options{
		DriverRoot: "/run/nvidia/driver",
		HintEvery:  6,
		Out:        out,
		Err:        out,
	})

	runner.emitHints(ProbeResult{}, 1)
	if out.Len() != 0 {
		t.Fatalf("expected no output, got %q", out.String())
	}
}

func TestEmitHintsDriverRootEmpty(t *testing.T) {
	t.Parallel()

	out := &bytes.Buffer{}
	runner := NewRunner(Options{
		DriverRoot: "/run/nvidia/driver",
		HintEvery:  6,
		Out:        out,
		Err:        out,
	})

	runner.emitHints(ProbeResult{DriverRootEmpty: true}, 6)
	logs := out.String()
	assertContains(t, logs, "Check failed.")
	assertContains(t, logs, "Hint: Directory /run/nvidia/driver on the host is empty")
	assertContains(t, logs, "NVIDIA_DRIVER_ROOT is set to '/run/nvidia/driver'")
}

func TestEmitHintsOperatorDriverRoot(t *testing.T) {
	t.Parallel()

	out := &bytes.Buffer{}
	runner := NewRunner(Options{
		DriverRoot: "/",
		HintEvery:  6,
		Out:        out,
		Err:        out,
	})

	runner.emitHints(ProbeResult{OperatorSMIDetected: true}, 6)
	if !strings.Contains(out.String(), "may want to re-install the DRA driver Helm chart") {
		t.Fatalf("expected operator hint, got %q", out.String())
	}
}
