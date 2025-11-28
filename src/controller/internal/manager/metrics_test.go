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
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"k8s.io/client-go/rest"
	certutil "k8s.io/client-go/util/cert"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

func TestMetricsEndpointServesPrometheus(t *testing.T) {
	t.Setenv("TLS_CERT_FILE", "")
	t.Setenv("TLS_PRIVATE_KEY_FILE", "")
	t.Setenv("TLS_CA_FILE", "")

	tmp := t.TempDir()
	certPath := filepath.Join(tmp, "tls.crt")
	keyPath := filepath.Join(tmp, "tls.key")
	caPath := filepath.Join(tmp, "ca.crt")

	cert, key, err := certutil.GenerateSelfSignedCertKey("localhost", nil, nil)
	if err != nil {
		t.Fatalf("generate self-signed certificate: %v", err)
	}
	if err := os.WriteFile(certPath, cert, 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, key, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	if err := os.WriteFile(caPath, cert, 0o600); err != nil {
		t.Fatalf("write ca: %v", err)
	}

	t.Setenv("TLS_CERT_FILE", certPath)
	t.Setenv("TLS_PRIVATE_KEY_FILE", keyPath)
	t.Setenv("TLS_CA_FILE", caPath)

	opts, err := metricsOptionsFromEnv()
	if err != nil {
		t.Fatalf("metrics options from env: %v", err)
	}
	opts.BindAddress = "127.0.0.1:0"

	srv, err := server.NewServer(opts, &rest.Config{}, &http.Client{})
	if err != nil {
		t.Fatalf("create metrics server: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		if err := srv.Start(ctx); err != nil && err != http.ErrServerClosed {
			t.Errorf("metrics server start: %v", err)
		}
	}()

	var bindAddr string
	for i := 0; i < 200; i++ {
		if getter, ok := srv.(interface{ GetBindAddr() string }); ok {
			bindAddr = getter.GetBindAddr()
			if bindAddr != "" {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if bindAddr == "" {
		t.Fatalf("metrics server bind address not available")
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, // self-signed certificate
			},
		},
		Timeout: 5 * time.Second,
	}

resp, err := client.Get(fmt.Sprintf("https://%s/metrics", bindAddr))
	if err != nil {
		t.Fatalf("request metrics endpoint: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status %d, body: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read metrics body: %v", err)
	}
	if len(body) == 0 {
		t.Fatalf("metrics body is empty")
	}
}
