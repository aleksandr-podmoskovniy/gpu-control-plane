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

package filesystem

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

func TestFileCertificateManagerForceReload(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "tls.crt")
	keyPath := filepath.Join(dir, "tls.key")

	writeSelfSignedCert(t, certPath, keyPath, "initial")

	manager := NewFileCertificateManager(certPath, keyPath)
	if err := manager.rotateCerts(); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if cert := manager.Current(); cert == nil || cert.Leaf.Subject.CommonName != "initial" {
		t.Fatalf("expected initial certificate")
	}

	writeSelfSignedCert(t, certPath, keyPath, "updated")
	if err := manager.rotateCerts(); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if manager.Current().Leaf.Subject.CommonName != "updated" {
		t.Fatalf("unexpected CN: %s", manager.Current().Leaf.Subject.CommonName)
	}
}

func TestFileCertificateManagerStartStop(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "tls.crt")
	keyPath := filepath.Join(dir, "tls.key")

	writeSelfSignedCert(t, certPath, keyPath, "initial")

	manager := NewFileCertificateManager(certPath, keyPath)
	manager.errorRetryInterval = 20 * time.Millisecond

	done := make(chan struct{})
	go func() {
		manager.Start()
		close(done)
	}()

	waitForCN := func(expected string) {
		t.Helper()
		for i := 0; i < 100; i++ {
			if cert := manager.Current(); cert != nil && cert.Leaf != nil && cert.Leaf.Subject.CommonName == expected {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("expected certificate CN %q", expected)
	}

	waitForCN("initial")

	writeSelfSignedCert(t, certPath, keyPath, "updated")
	waitForCN("updated")

	manager.Stop()
	<-done
	manager.Stop()
}

func writeSelfSignedCert(t *testing.T, certPath, keyPath, cn string) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject: pkix.Name{
			CommonName: cn,
		},
		NotBefore: time.Now().Add(-time.Minute),
		NotAfter:  time.Now().Add(time.Hour),
		KeyUsage:  x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})

	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
}
