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

package server

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

type waitCertManager struct {
	started chan struct{}
	stopped atomic.Bool
}

func (w *waitCertManager) Start()                    { close(w.started) }
func (w *waitCertManager) Stop()                     { w.stopped.Store(true) }
func (w *waitCertManager) Current() *tls.Certificate { return nil }

func TestHTTPServerInitReturnsFalseWhenStopped(t *testing.T) {
	srv := &HTTPServer{stopped: true}
	if srv.init() {
		t.Fatalf("expected init to return false when stopped")
	}
}

func TestHTTPServerInitListenErrorSetsErr(t *testing.T) {
	origListen := netListen
	netListen = func(string, string) (net.Listener, error) {
		return nil, errors.New("listen fail")
	}
	t.Cleanup(func() { netListen = origListen })

	srv := &HTTPServer{InstanceDesc: "test", ListenAddr: "127.0.0.1:0"}
	if srv.init() {
		t.Fatalf("expected init to fail")
	}
	if srv.Err == nil {
		t.Fatalf("expected Err to be set")
	}
}

func TestHTTPServerStartReturnsWhenInitFails(t *testing.T) {
	origListen := netListen
	origServe := serverServe
	t.Cleanup(func() {
		netListen = origListen
		serverServe = origServe
	})

	netListen = func(string, string) (net.Listener, error) {
		return nil, errors.New("listen fail")
	}

	var served atomic.Bool
	serverServe = func(*http.Server, net.Listener) error {
		served.Store(true)
		return nil
	}

	srv := &HTTPServer{InstanceDesc: "test", ListenAddr: "127.0.0.1:0", RootHandler: http.NewServeMux()}
	srv.Start()

	if served.Load() {
		t.Fatalf("expected Start to return before Serve is called")
	}
	if srv.Err == nil {
		t.Fatalf("expected Err to be set from init failure")
	}
}

func TestHTTPServerStartSetsErrOnServeFailure(t *testing.T) {
	origServe := serverServe
	t.Cleanup(func() { serverServe = origServe })

	serveErr := errors.New("serve fail")
	serverServe = func(_ *http.Server, ln net.Listener) error {
		_ = ln.Close()
		return serveErr
	}

	srv := &HTTPServer{InstanceDesc: "test", ListenAddr: "127.0.0.1:0", RootHandler: http.NewServeMux()}
	srv.Start()
	if !errors.Is(srv.Err, serveErr) {
		t.Fatalf("expected Err to be %v, got %v", serveErr, srv.Err)
	}
}

func TestHTTPServerStartTLSPathSetsErrOnServeTLSFailure(t *testing.T) {
	origServeTLS := serverServeTLS
	t.Cleanup(func() { serverServeTLS = origServeTLS })

	serveErr := errors.New("serveTLS fail")
	serverServeTLS = func(_ *http.Server, ln net.Listener, _, _ string) error {
		_ = ln.Close()
		return serveErr
	}

	cm := &waitCertManager{started: make(chan struct{})}
	srv := &HTTPServer{
		InstanceDesc: "tls",
		ListenAddr:   "127.0.0.1:0",
		RootHandler:  http.NewServeMux(),
		CertManager:  cm,
	}

	srv.Start()

	select {
	case <-cm.started:
	case <-time.After(time.Second):
		t.Fatalf("expected CertManager.Start to be called")
	}

	if !errors.Is(srv.Err, serveErr) {
		t.Fatalf("expected Err to be %v, got %v", serveErr, srv.Err)
	}

	srv.Stop()
	if !cm.stopped.Load() {
		t.Fatalf("expected CertManager.Stop to be called")
	}
}

func TestDefaultServerServeTLSWrapper_IsCovered(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	if err := serverServeTLS(&http.Server{}, ln, "", ""); err == nil {
		t.Fatalf("expected error from ServeTLS without certificates")
	}
}

func TestHTTPServerStopIdempotent(t *testing.T) {
	srv := &HTTPServer{}
	srv.Stop()
	srv.Stop()
}

func TestHTTPServerStopSetsErrOnShutdownError(t *testing.T) {
	origShutdown := serverShutdown
	t.Cleanup(func() { serverShutdown = origShutdown })

	shutdownErr := errors.New("shutdown fail")
	serverShutdown = func(*http.Server, context.Context) error { return shutdownErr }

	srv := &HTTPServer{InstanceDesc: "test", instance: &http.Server{}}
	srv.Stop()
	if !errors.Is(srv.Err, shutdownErr) {
		t.Fatalf("expected Err to be %v, got %v", shutdownErr, srv.Err)
	}
}

func TestHTTPServerStopKeepsExistingErrOnShutdownError(t *testing.T) {
	origShutdown := serverShutdown
	t.Cleanup(func() { serverShutdown = origShutdown })

	shutdownErr := errors.New("shutdown fail")
	serverShutdown = func(*http.Server, context.Context) error { return shutdownErr }

	existing := errors.New("existing err")
	srv := &HTTPServer{InstanceDesc: "test", instance: &http.Server{}, Err: existing}
	srv.Stop()
	if !errors.Is(srv.Err, existing) {
		t.Fatalf("expected Err to stay %v, got %v", existing, srv.Err)
	}
}
