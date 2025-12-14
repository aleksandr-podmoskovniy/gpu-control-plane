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

package filesystem

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

type fakeWatcher struct {
	eventsCh chan fsnotify.Event
	errorsCh chan error

	addErr error

	closeEvents bool
	closeErrors bool

	closeOnce sync.Once
}

func newFakeWatcher(closeEvents, closeErrors bool) *fakeWatcher {
	return &fakeWatcher{
		eventsCh:    make(chan fsnotify.Event, 16),
		errorsCh:    make(chan error, 16),
		closeEvents: closeEvents,
		closeErrors: closeErrors,
	}
}

func (f *fakeWatcher) Add(string) error { return f.addErr }

func (f *fakeWatcher) Close() error {
	f.closeOnce.Do(func() {
		if f.closeEvents {
			close(f.eventsCh)
		}
		if f.closeErrors {
			close(f.errorsCh)
		}
	})
	return nil
}

func (f *fakeWatcher) Events() <-chan fsnotify.Event { return f.eventsCh }
func (f *fakeWatcher) Errors() <-chan error          { return f.errorsCh }

func TestFileCertificateManagerStartReturnsOnWatcherError(t *testing.T) {
	orig := newWatcher
	newWatcher = func() (watcher, error) {
		return nil, errors.New("boom")
	}
	t.Cleanup(func() { newWatcher = orig })

	manager := NewFileCertificateManager("cert", "key")
	manager.Start()
}

func TestNewWatcherPropagatesFSNotifyError(t *testing.T) {
	orig := fsnotifyNewWatcher
	fsnotifyNewWatcher = func() (*fsnotify.Watcher, error) {
		return nil, errors.New("boom")
	}
	t.Cleanup(func() { fsnotifyNewWatcher = orig })

	if _, err := newWatcher(); err == nil {
		t.Fatalf("expected newWatcher to return error")
	}
}

func TestFileCertificateManagerStartRetryRequeuesObjectUpdated(t *testing.T) {
	origWatcher := newWatcher
	origSleep := sleep
	t.Cleanup(func() {
		newWatcher = origWatcher
		sleep = origSleep
	})

	fw := newFakeWatcher(true, false) // close Events to deterministically hit Events ok=false branch
	fw.addErr = errors.New("add fail")
	newWatcher = func() (watcher, error) { return fw, nil }
	sleep = func(time.Duration) {}

	certDir := t.TempDir()
	keyDir := t.TempDir()

	manager := NewFileCertificateManager(filepath.Join(certDir, "tls.crt"), filepath.Join(keyDir, "tls.key"))

	var rotateCalls atomic.Int32
	secondCall := make(chan struct{})
	manager.rotateCertsFn = func() error {
		n := rotateCalls.Add(1)
		if n == 1 {
			return errors.New("rotate fail")
		}
		close(secondCall)
		manager.Stop()
		return nil
	}

	done := make(chan struct{})
	go func() {
		manager.Start()
		close(done)
	}()

	select {
	case <-secondCall:
	case <-time.After(2 * time.Second):
		t.Fatalf("expected rotate to be called twice")
	}
	<-done
}

func TestFileCertificateManagerStartDropsRedundantWakeups(t *testing.T) {
	origWatcher := newWatcher
	origSleep := sleep
	t.Cleanup(func() {
		newWatcher = origWatcher
		sleep = origSleep
	})

	fw := newFakeWatcher(true, false)
	fw.eventsCh = make(chan fsnotify.Event)
	newWatcher = func() (watcher, error) { return fw, nil }

	origRetryDrop := onRetryDrop
	t.Cleanup(func() { onRetryDrop = origRetryDrop })
	retryDropped := make(chan struct{})
	var dropOnce sync.Once
	onRetryDrop = func() {
		dropOnce.Do(func() {
			close(retryDropped)
		})
	}

	sleepRelease := make(chan struct{})
	retryReached := make(chan struct{})
	retryContinue := make(chan struct{})
	sleep = func(time.Duration) {
		<-sleepRelease
		close(retryReached)
		<-retryContinue
	}

	manager := NewFileCertificateManager(filepath.Join(t.TempDir(), "tls.crt"), filepath.Join(t.TempDir(), "tls.key"))

	var call atomic.Int32
	rotate1Done := make(chan struct{})
	rotate2Started := make(chan struct{})
	rotate2Release := make(chan struct{})

	manager.rotateCertsFn = func() error {
		switch call.Add(1) {
		case 1:
			close(rotate1Done)
			return errors.New("rotate fail")
		case 2:
			close(rotate2Started)
			<-rotate2Release
			manager.Stop()
			return nil
		default:
			return nil
		}
	}

	done := make(chan struct{})
	go func() {
		manager.Start()
		close(done)
	}()

	// Trigger second rotation and keep the main loop blocked in rotate2.
	<-rotate1Done
	fw.eventsCh <- fsnotify.Event{}
	<-rotate2Started

	// Fill objectUpdated and then send redundant events to hit the drop branch.
	fw.eventsCh <- fsnotify.Event{}
	fw.eventsCh <- fsnotify.Event{}

	close(sleepRelease)
	<-retryReached
	close(retryContinue) // let retry goroutine attempt a redundant send
	select {
	case <-retryDropped:
	case <-time.After(2 * time.Second):
		t.Fatalf("expected retry drop path to be taken")
	}
	close(rotate2Release)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("expected Start to stop")
	}
}

func TestFileCertificateManagerStartHandlesWatcherErrors(t *testing.T) {
	origWatcher := newWatcher
	t.Cleanup(func() { newWatcher = origWatcher })

	fw := newFakeWatcher(false, true) // close Errors to deterministically hit Errors ok=false branch
	newWatcher = func() (watcher, error) { return fw, nil }

	manager := NewFileCertificateManager(filepath.Join(t.TempDir(), "tls.crt"), filepath.Join(t.TempDir(), "tls.key"))
	manager.rotateCertsFn = func() error { return nil }

	done := make(chan struct{})
	go func() {
		manager.Start()
		close(done)
	}()

	fw.errorsCh <- errors.New("watcher error")
	manager.Stop()
	<-done
}

func TestFileCertificateManagerRotateCertsReturnsErrorWhenLoadFails(t *testing.T) {
	manager := NewFileCertificateManager(filepath.Join(t.TempDir(), "missing.crt"), filepath.Join(t.TempDir(), "missing.key"))
	if err := manager.rotateCerts(); err == nil {
		t.Fatalf("expected rotateCerts error")
	}
}

func TestFileCertificateManagerLoadCertificatesErrors(t *testing.T) {
	t.Run("read cert error", func(t *testing.T) {
		manager := NewFileCertificateManager(filepath.Join(t.TempDir(), "missing.crt"), filepath.Join(t.TempDir(), "missing.key"))
		if _, err := manager.loadCertificates(); err == nil {
			t.Fatalf("expected cert read error")
		}
	})

	t.Run("read key error", func(t *testing.T) {
		dir := t.TempDir()
		certPath := filepath.Join(dir, "tls.crt")
		if err := os.WriteFile(certPath, []byte("cert"), 0o600); err != nil {
			t.Fatalf("write cert: %v", err)
		}

		manager := NewFileCertificateManager(certPath, filepath.Join(dir, "missing.key"))
		if _, err := manager.loadCertificates(); err == nil {
			t.Fatalf("expected key read error")
		}
	})

	t.Run("key pair error", func(t *testing.T) {
		dir := t.TempDir()
		certPath := filepath.Join(dir, "tls.crt")
		keyPath := filepath.Join(dir, "tls.key")
		if err := os.WriteFile(certPath, []byte("invalid"), 0o600); err != nil {
			t.Fatalf("write cert: %v", err)
		}
		if err := os.WriteFile(keyPath, []byte("invalid"), 0o600); err != nil {
			t.Fatalf("write key: %v", err)
		}

		manager := NewFileCertificateManager(certPath, keyPath)
		if _, err := manager.loadCertificates(); err == nil {
			t.Fatalf("expected key pair error")
		}
	})

	t.Run("leaf parse error", func(t *testing.T) {
		dir := t.TempDir()
		certPath := filepath.Join(dir, "tls.crt")
		keyPath := filepath.Join(dir, "tls.key")
		writeSelfSignedCertWithHeaders(t, certPath, keyPath, "test")

		manager := NewFileCertificateManager(certPath, keyPath)
		if _, err := manager.loadCertificates(); err == nil {
			t.Fatalf("expected leaf parse error")
		}
	})
}

func writeSelfSignedCertWithHeaders(t *testing.T, certPath, keyPath, cn string) {
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

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER, Headers: map[string]string{"x": "y"}})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})

	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
}
