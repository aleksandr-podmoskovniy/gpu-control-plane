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

package util

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"
)

func TestParseCertsPEM(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject: pkix.Name{
			CommonName: "test",
		},
		NotBefore: time.Now().Add(-time.Minute),
		NotAfter:  time.Now().Add(time.Hour),
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	certs, err := ParseCertsPEM(pemBytes)
	if err != nil {
		t.Fatalf("parse certs: %v", err)
	}
	if len(certs) != 1 || certs[0].Subject.CommonName != "test" {
		t.Fatalf("unexpected certs: %+v", certs)
	}
}

func TestParseCertsPEMInvalid(t *testing.T) {
	if _, err := ParseCertsPEM([]byte("invalid")); err == nil {
		t.Fatalf("expected failure for invalid PEM")
	}
}

func TestParseCertsPEMSkipsNonCertificateBlocks(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject: pkix.Name{
			CommonName: "test",
		},
		NotBefore: time.Now().Add(-time.Minute),
		NotAfter:  time.Now().Add(time.Hour),
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}

	ignored := pem.EncodeToMemory(&pem.Block{Type: "NOT-A-CERT", Bytes: []byte("ignored")})
	withHeaders := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER, Headers: map[string]string{"x": "y"}})
	valid := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	certs, err := ParseCertsPEM(append(append(ignored, withHeaders...), valid...))
	if err != nil {
		t.Fatalf("parse certs: %v", err)
	}
	if len(certs) != 1 || certs[0].Subject.CommonName != "test" {
		t.Fatalf("unexpected certs: %+v", certs)
	}
}

func TestParseCertsPEMInvalidCertificateBlockReturnsError(t *testing.T) {
	invalid := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("not-der")})
	if _, err := ParseCertsPEM(invalid); err == nil {
		t.Fatalf("expected certificate parse error")
	}
}
