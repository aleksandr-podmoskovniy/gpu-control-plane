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

package main

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/images/kube-api-rewriter/pkg/monitoring/metrics"
)

func TestMainNoRewritersReturns(t *testing.T) {
	metrics.Registry = prometheus.NewRegistry()

	t.Setenv("CLIENT_PROXY", "no")
	t.Setenv("WEBHOOK_ADDRESS", "")

	var called atomic.Bool
	origExit := exitFunc
	exitFunc = func(int) { called.Store(true) }
	t.Cleanup(func() { exitFunc = origExit })

	main()

	if called.Load() {
		t.Fatalf("expected main to return without exit when no rewriters configured")
	}
}

func TestMainRulesPathErrorExits(t *testing.T) {
	metrics.Registry = prometheus.NewRegistry()

	t.Setenv("RULES_PATH", filepath.Join(t.TempDir(), "missing.yaml"))

	var (
		called atomic.Bool
		code   atomic.Int32
	)
	origExit := exitFunc
	exitFunc = func(c int) {
		called.Store(true)
		code.Store(int32(c))
	}
	t.Cleanup(func() { exitFunc = origExit })

	main()

	if !called.Load() || code.Load() != 1 {
		t.Fatalf("expected exit code 1, got called=%v code=%d", called.Load(), code.Load())
	}
}

func TestMainClientProxyTargetErrorExits(t *testing.T) {
	metrics.Registry = prometheus.NewRegistry()

	t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "missing"))
	t.Setenv("WEBHOOK_ADDRESS", "")

	var code atomic.Int32
	origExit := exitFunc
	exitFunc = func(c int) { code.Store(int32(c)) }
	t.Cleanup(func() { exitFunc = origExit })

	main()

	if code.Load() != 1 {
		t.Fatalf("expected exit code 1, got %d", code.Load())
	}
}

func TestMainWebhookTargetErrorExits(t *testing.T) {
	metrics.Registry = prometheus.NewRegistry()

	t.Setenv("CLIENT_PROXY", "no")
	t.Setenv("WEBHOOK_ADDRESS", "https://127.0.0.1:9443")
	t.Setenv("WEBHOOK_CERT_FILE", "cert.pem")
	t.Setenv("WEBHOOK_KEY_FILE", "")

	var code atomic.Int32
	origExit := exitFunc
	exitFunc = func(c int) { code.Store(int32(c)) }
	t.Cleanup(func() { exitFunc = origExit })

	main()

	if code.Load() != 1 {
		t.Fatalf("expected exit code 1, got %d", code.Load())
	}
}

func TestMainStartsServersAndExits(t *testing.T) {
	metrics.Registry = prometheus.NewRegistry()

	kubeconfig := writeTempKubeconfig(t)
	t.Setenv("KUBECONFIG", kubeconfig)

	rulesPath := filepath.Join(t.TempDir(), "rules.yaml")
	if err := os.WriteFile(rulesPath, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write rules file: %v", err)
	}
	t.Setenv("RULES_PATH", rulesPath)

	t.Setenv("CLIENT_PROXY_PORT", "invalid")
	t.Setenv("WEBHOOK_ADDRESS", "https://127.0.0.1:9443")
	t.Setenv("WEBHOOK_PROXY_PORT", "invalid")

	// Enable pprof branch.
	t.Setenv("PPROF_BIND_ADDRESS", "127.0.0.1:invalid")

	var code atomic.Int32
	origExit := exitFunc
	exitFunc = func(c int) { code.Store(int32(c)) }
	t.Cleanup(func() { exitFunc = origExit })

	main()

	if code.Load() != 1 {
		t.Fatalf("expected exit code 1 for server start errors, got %d", code.Load())
	}
}

func writeTempKubeconfig(t *testing.T) string {
	t.Helper()

	content := `
apiVersion: v1
clusters:
- cluster:
    server: https://127.0.0.1
  name: fake
contexts:
- context:
    cluster: fake
    user: fake
  name: fake
current-context: fake
users:
- name: fake
  user:
    token: fake
`

	path := filepath.Join(t.TempDir(), "kubeconfig")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}
	return path
}
