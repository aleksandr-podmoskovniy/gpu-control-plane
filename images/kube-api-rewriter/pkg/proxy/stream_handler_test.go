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

package proxy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"k8s.io/apimachinery/pkg/watch"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/images/kube-api-rewriter/pkg/gpu"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/images/kube-api-rewriter/pkg/labels"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/images/kube-api-rewriter/pkg/rewriter"
)

func TestStreamHandlerHandleSuccess(t *testing.T) {
	rules := *gpu.GPURewriteRules
	rules.Init()

	provider := newStubMetricsProvider()
	handler := &StreamHandler{
		Rewriter:        &rewriter.RuleBasedRewriter{Rules: &rules},
		MetricsProvider: provider,
	}

	internalKind := rules.RenameKind("GPUDevice")
	internalAPIVersion := rules.RenameApiVersion("gpu.deckhouse.io/v1alpha1")
	stream := []byte(fmt.Sprintf(`{"type":"%s","object":{"apiVersion":"%s","kind":"%s","metadata":{"name":"device-1"}}}`,
		watch.Added, internalAPIVersion, internalKind))
	resp := &http.Response{
		Body:       io.NopCloser(bytes.NewReader(stream)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		StatusCode: http.StatusOK,
	}

	req, err := http.NewRequest(http.MethodGet, "https://kubernetes/apis/gpu.deckhouse.io/v1alpha1/gpudevices?watch=1", nil)
	if err != nil {
		t.Fatalf("create http request: %v", err)
	}
	targetReq := rewriter.NewTargetRequest(handler.Rewriter, req)
	if targetReq == nil {
		t.Fatal("expected non-nil target request")
	}

	ctx := labels.ContextWithCommon(context.Background(),
		"gpu-api", "gpudevices", http.MethodGet, WatchLabel(true), "rewrite", "restore")
	ctx = labels.ContextWithDecision(ctx, "rewrite")
	ctx = labels.ContextWithStatus(ctx, http.StatusOK)

	recorder := httptest.NewRecorder()
	if err := handler.Handle(ctx, recorder, resp, targetReq); err != nil {
		t.Fatalf("handle returned error: %v", err)
	}

	if recorder.Body.Len() == 0 {
		t.Fatalf("expected body to contain forwarded event")
	}

	if provider.counters["rewrites"].incCalls == 0 {
		t.Fatalf("expected rewrites counter to be incremented")
	}
	if provider.counters["handled"].incCalls == 0 {
		t.Fatalf("expected handled counter to be incremented")
	}
}

func TestStreamHandlerHandleInitError(t *testing.T) {
	handler := &StreamHandler{
		Rewriter:        &rewriter.RuleBasedRewriter{Rules: &rewriter.RewriteRules{}},
		MetricsProvider: newStubMetricsProvider(),
	}

	resp := &http.Response{
		Body:       io.NopCloser(bytes.NewReader(nil)),
		Header:     http.Header{"Content-Type": []string{"text/plain"}},
		StatusCode: http.StatusOK,
	}

	req, _ := http.NewRequest(http.MethodGet, "https://kubernetes/apis/gpu.deckhouse.io/v1alpha1/gpudevices?watch=1", nil)
	targetReq := rewriter.NewTargetRequest(handler.Rewriter, req)
	ctx := context.Background()
	recorder := httptest.NewRecorder()

	err := handler.Handle(ctx, recorder, resp, targetReq)
	if err == nil {
		t.Fatalf("expected initialization error for unsupported content type")
	}
}

func TestCreateWatchDecoderInvalid(t *testing.T) {
	_, err := createWatchDecoder(io.NopCloser(bytes.NewReader(nil)), "text/plain")
	if err == nil {
		t.Fatalf("expected error for invalid media type")
	}
}
