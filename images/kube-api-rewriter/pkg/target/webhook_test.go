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
	"os"
	"path/filepath"
	"testing"
)

func TestNewWebhookTargetDefaults(t *testing.T) {
	t.Setenv(WebhookAddressVar, "")
	t.Setenv(WebhookServerNameVar, "")
	t.Setenv(WebhookCertFileVar, "")
	t.Setenv(WebhookKeyFileVar, "")

	target, err := NewWebhookTarget()
	if err != nil {
		t.Fatalf("NewWebhookTarget: %v", err)
	}
	if target.URL.String() != defaultWebhookAddress {
		t.Fatalf("unexpected URL: %s", target.URL)
	}
	if target.Client == nil {
		t.Fatalf("expected http client")
	}
	if target.CertManager != nil {
		t.Fatalf("expected no cert manager with default settings")
	}
}

func TestNewWebhookTargetCertificateErrors(t *testing.T) {
	t.Setenv(WebhookAddressVar, "https://127.0.0.1:9443")
	t.Setenv(WebhookServerNameVar, "")
	t.Setenv(WebhookCertFileVar, "cert.pem")
	t.Setenv(WebhookKeyFileVar, "")

	if _, err := NewWebhookTarget(); err == nil {
		t.Fatalf("expected error when only cert file provided")
	}

	t.Setenv(WebhookCertFileVar, "")
	t.Setenv(WebhookKeyFileVar, "key.pem")
	if _, err := NewWebhookTarget(); err == nil {
		t.Fatalf("expected error when only key file provided")
	}
}

func TestNewWebhookTargetWithTLS(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "tls.crt")
	keyPath := filepath.Join(dir, "tls.key")
	if err := os.WriteFile(certPath, []byte("dummy"), 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, []byte("dummy"), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	t.Setenv(WebhookCertFileVar, certPath)
	t.Setenv(WebhookKeyFileVar, keyPath)
	t.Setenv(WebhookAddressVar, "https://10.0.0.1:8443")
	t.Setenv(WebhookServerNameVar, "gpu-webhook")

	target, err := NewWebhookTarget()
	if err != nil {
		t.Fatalf("NewWebhookTarget: %v", err)
	}
	if target.CertManager == nil {
		t.Fatalf("expected cert manager to be configured")
	}
	if target.Client.Timeout != defaultWebhookTimeout {
		t.Fatalf("unexpected timeout: %s", target.Client.Timeout)
	}
}
