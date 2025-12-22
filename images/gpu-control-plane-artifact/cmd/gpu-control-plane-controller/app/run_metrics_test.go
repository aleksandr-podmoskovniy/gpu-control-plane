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

package app

import (
	"crypto/tls"
	"os"
	"path/filepath"
	"testing"
)

func TestMetricsOptionsFromEnvDefault(t *testing.T) {
	t.Setenv("TLS_CERT_FILE", "")
	t.Setenv("TLS_PRIVATE_KEY_FILE", "")
	t.Setenv("TLS_CA_FILE", "")

	opts, err := metricsOptionsFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.SecureServing {
		t.Fatalf("expected SecureServing to be disabled by default")
	}
}

func TestMetricsOptionsFromEnvTLS(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "tls.crt")
	keyPath := filepath.Join(dir, "tls.key")
	caPath := filepath.Join(dir, "ca.crt")

	certPEM, keyPEM := generateSelfSignedCert(t)
	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	if err := os.WriteFile(caPath, certPEM, 0o600); err != nil {
		t.Fatalf("write ca: %v", err)
	}

	t.Setenv("TLS_CERT_FILE", certPath)
	t.Setenv("TLS_PRIVATE_KEY_FILE", keyPath)
	t.Setenv("TLS_CA_FILE", caPath)

	opts, err := metricsOptionsFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !opts.SecureServing {
		t.Fatalf("expected SecureServing to be enabled")
	}
	if opts.CertDir != dir {
		t.Fatalf("unexpected cert dir: %s", opts.CertDir)
	}
	if opts.CertName != "tls.crt" {
		t.Fatalf("unexpected cert name: %s", opts.CertName)
	}
	if opts.KeyName != "tls.key" {
		t.Fatalf("unexpected key name: %s", opts.KeyName)
	}
	if len(opts.TLSOpts) == 0 {
		t.Fatalf("expected TLSOpts to include CA configuration")
	}
	cfg := &tls.Config{}
	opts.TLSOpts[0](cfg)
	if cfg.ClientCAs == nil {
		t.Fatalf("expected ClientCAs to be configured")
	}
	if cfg.ClientAuth != tls.VerifyClientCertIfGiven {
		t.Fatalf("unexpected client auth mode: %v", cfg.ClientAuth)
	}
}

func TestMetricsOptionsFromEnvInvalidCA(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "tls.crt")
	keyPath := filepath.Join(dir, "tls.key")
	caPath := filepath.Join(dir, "ca.crt")

	certPEM, keyPEM := generateSelfSignedCert(t)
	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	if err := os.WriteFile(caPath, []byte("not a certificate"), 0o600); err != nil {
		t.Fatalf("write ca: %v", err)
	}

	t.Setenv("TLS_CERT_FILE", certPath)
	t.Setenv("TLS_PRIVATE_KEY_FILE", keyPath)
	t.Setenv("TLS_CA_FILE", caPath)

	if _, err := metricsOptionsFromEnv(); err == nil {
		t.Fatalf("expected error for invalid CA data")
	}
}

func TestMetricsOptionsFromEnvMissingCert(t *testing.T) {
	t.Setenv("TLS_CERT_FILE", filepath.Join(t.TempDir(), "absent.crt"))
	t.Setenv("TLS_PRIVATE_KEY_FILE", filepath.Join(t.TempDir(), "absent.key"))

	if _, err := metricsOptionsFromEnv(); err == nil {
		t.Fatalf("expected error when TLS files are missing")
	}
}

func TestMetricsOptionsFromEnvMissingKey(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "tls.crt")
	certPEM, keyPEM := generateSelfSignedCert(t)
	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	// Ensure key file exists separately to avoid short-circuit on missing cert.
	keyPath := filepath.Join(dir, "tls.key")
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	t.Setenv("TLS_CERT_FILE", certPath)
	t.Setenv("TLS_PRIVATE_KEY_FILE", filepath.Join(dir, "missing.key"))

	if _, err := metricsOptionsFromEnv(); err == nil {
		t.Fatalf("expected error when private key is missing")
	}
}

func TestMetricsOptionsFromEnvReadCAError(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "tls.crt")
	keyPath := filepath.Join(dir, "tls.key")
	certPEM, keyPEM := generateSelfSignedCert(t)

	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	t.Setenv("TLS_CERT_FILE", certPath)
	t.Setenv("TLS_PRIVATE_KEY_FILE", keyPath)
	// Point to a missing CA file to trigger read error.
	t.Setenv("TLS_CA_FILE", filepath.Join(dir, "missing-ca.crt"))

	if _, err := metricsOptionsFromEnv(); err == nil {
		t.Fatalf("expected error when CA file cannot be read")
	}
}

func TestMetricsOptionsFromEnvCustomBindAddress(t *testing.T) {
	t.Setenv("METRICS_BIND_ADDRESS", "  :9444  ")
	t.Setenv("TLS_CERT_FILE", "")
	t.Setenv("TLS_PRIVATE_KEY_FILE", "")
	t.Setenv("TLS_CA_FILE", "")

	opts, err := metricsOptionsFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.BindAddress != ":9444" {
		t.Fatalf("expected bind address :9444, got %s", opts.BindAddress)
	}
}
