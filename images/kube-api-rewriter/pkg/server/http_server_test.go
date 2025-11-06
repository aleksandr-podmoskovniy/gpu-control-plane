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

package server

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"
)

func waitForListener(t *testing.T, srv *HTTPServer) string {
	t.Helper()
	for i := 0; i < 50; i++ {
		if srv.listener != nil {
			return srv.listener.Addr().String()
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("listener not initialized")
	return ""
}

func TestHTTPServerStartStop(t *testing.T) {
	srv := &HTTPServer{
		InstanceDesc: "test",
		ListenAddr:   "127.0.0.1:0",
		RootHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "ok")
		}),
	}

	done := make(chan struct{})
	go func() {
		srv.Start()
		close(done)
	}()

	addr := waitForListener(t, srv)

	resp, err := http.Get("http://" + addr)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok" {
		t.Fatalf("unexpected body: %q", string(body))
	}

	srv.Stop()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("server did not stop in time")
	}
}

type dummyRunnable struct {
	started bool
	stopped bool
}

func (d *dummyRunnable) Start() { d.started = true }
func (d *dummyRunnable) Stop()  { d.stopped = true }

func TestRunnableGroup(t *testing.T) {
	r1 := &dummyRunnable{}
	r2 := &dummyRunnable{}

	group := NewRunnableGroup()
	group.Add(r1)
	group.Add(r2)

	done := make(chan struct{})
	go func() {
		group.Start()
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	if !r1.started || !r2.started {
		t.Fatalf("expected all runnables to start")
	}

	r1.Stop()
	r2.Stop()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("group did not stop in time")
	}
}

type stubCertManager struct {
	current *tls.Certificate
	started bool
	stopped bool
}

func (s *stubCertManager) Start() { s.started = true }
func (s *stubCertManager) Stop()  { s.stopped = true }
func (s *stubCertManager) Current() *tls.Certificate {
	return s.current
}

func TestSetupTLS(t *testing.T) {
	srv := &HTTPServer{
		InstanceDesc: "tls",
		ListenAddr:   "127.0.0.1:0",
		RootHandler:  http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
		CertManager:  &stubCertManager{},
	}
	if !srv.init() {
		t.Fatalf("init failed")
	}
	srv.setupTLS()
	if srv.instance.TLSConfig == nil {
		t.Fatalf("expected TLS config")
	}
	if _, err := srv.instance.TLSConfig.GetCertificate(nil); err == nil {
		t.Fatalf("expected error when certificate is absent")
	}

	stub := srv.CertManager.(*stubCertManager)
	stub.current = &tls.Certificate{}
	if cert, err := srv.instance.TLSConfig.GetCertificate(nil); err != nil || cert == nil {
		t.Fatalf("expected certificate after manager update")
	}
}

func TestConstructListenAddr(t *testing.T) {
	addr := ConstructListenAddr("", "", "127.0.0.1", "8080")
	if addr != "127.0.0.1:8080" {
		t.Fatalf("unexpected addr: %s", addr)
	}

	addr = ConstructListenAddr("0.0.0.0", "9090", "127.0.0.1", "8080")
	if addr != "0.0.0.0:9090" {
		t.Fatalf("unexpected addr override: %s", addr)
	}
}
