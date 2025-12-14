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

package target

import (
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"k8s.io/client-go/rest"
)

const kubeconfigContent = `
apiVersion: v1
kind: Config
clusters:
- cluster:
    server: http://127.0.0.1:9443
  name: test
users:
- name: test
  user:
    token: fake
contexts:
- context:
    cluster: test
    user: test
  name: test
current-context: test
`

func TestLoadConfigFromKubeconfig(t *testing.T) {
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBERNETES_SERVICE_PORT", "")

	dir := t.TempDir()
	kubeconfigPath := filepath.Join(dir, "config")
	if err := os.WriteFile(kubeconfigPath, []byte(kubeconfigContent), 0o644); err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}
	t.Setenv("KUBECONFIG", kubeconfigPath)

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.Host != "http://127.0.0.1:9443" {
		t.Fatalf("unexpected host: %s", cfg.Host)
	}

	target, err := NewKubernetesTarget()
	if err != nil {
		t.Fatalf("NewKubernetesTarget: %v", err)
	}
	if target.APIServerURL.String() != "http://127.0.0.1:9443" {
		t.Fatalf("unexpected URL: %s", target.APIServerURL)
	}
	if target.Client == nil {
		t.Fatalf("expected HTTP client")
	}
}

func TestLoadConfigError(t *testing.T) {
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBERNETES_SERVICE_PORT", "")
	t.Setenv("KUBECONFIG", "")
	t.Setenv("HOME", t.TempDir())

	if _, err := loadConfig(); err == nil {
		t.Fatalf("expected error when KUBECONFIG is empty")
	}
}

func TestLoadConfigFromInCluster(t *testing.T) {
	orig := inClusterConfig
	inClusterConfig = func() (*rest.Config, error) {
		return &rest.Config{Host: "https://in-cluster"}, nil
	}
	t.Cleanup(func() { inClusterConfig = orig })

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.Host != "https://in-cluster" {
		t.Fatalf("unexpected host: %s", cfg.Host)
	}
}

func TestLoadConfigFailsWhenHomeDirFails(t *testing.T) {
	origCluster := inClusterConfig
	origHome := userHomeDir
	t.Cleanup(func() {
		inClusterConfig = origCluster
		userHomeDir = origHome
	})

	inClusterConfig = func() (*rest.Config, error) { return nil, errors.New("no cluster") }
	userHomeDir = func() (string, error) { return "", errors.New("no home") }

	t.Setenv("KUBECONFIG", "")

	if _, err := loadConfig(); err == nil {
		t.Fatalf("expected error when home dir and KUBECONFIG are unavailable")
	}
}

func TestNewKubernetesTargetHTTPClientError(t *testing.T) {
	origCluster := inClusterConfig
	origClientFor := httpClientFor
	t.Cleanup(func() {
		inClusterConfig = origCluster
		httpClientFor = origClientFor
	})

	inClusterConfig = func() (*rest.Config, error) { return &rest.Config{Host: "https://127.0.0.1"}, nil }
	httpClientFor = func(*rest.Config) (*http.Client, error) { return nil, errors.New("boom") }

	if _, err := NewKubernetesTarget(); err == nil {
		t.Fatalf("expected HTTP client config error")
	}
}

func TestNewKubernetesTargetURLParseError(t *testing.T) {
	origCluster := inClusterConfig
	origClientFor := httpClientFor
	origParse := parseURL
	t.Cleanup(func() {
		inClusterConfig = origCluster
		httpClientFor = origClientFor
		parseURL = origParse
	})

	inClusterConfig = func() (*rest.Config, error) { return &rest.Config{Host: "https://127.0.0.1"}, nil }
	httpClientFor = func(*rest.Config) (*http.Client, error) { return &http.Client{}, nil }
	parseURL = func(string) (*url.URL, error) { return nil, errors.New("parse fail") }

	if _, err := NewKubernetesTarget(); err == nil {
		t.Fatalf("expected URL parse error")
	}
}

func TestNewKubernetesTargetLoadConfigError(t *testing.T) {
	origCluster := inClusterConfig
	origHome := userHomeDir
	t.Cleanup(func() {
		inClusterConfig = origCluster
		userHomeDir = origHome
	})

	inClusterConfig = func() (*rest.Config, error) { return nil, errors.New("no cluster") }
	userHomeDir = func() (string, error) { return "", errors.New("no home") }
	t.Setenv("KUBECONFIG", "")

	if _, err := NewKubernetesTarget(); err == nil {
		t.Fatalf("expected config load error")
	}
}
