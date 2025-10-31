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

package manager

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
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

func generateSelfSignedCert(t *testing.T) ([]byte, []byte) {
	t.Helper()

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "example.local"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	return certPEM, keyPEM
}
