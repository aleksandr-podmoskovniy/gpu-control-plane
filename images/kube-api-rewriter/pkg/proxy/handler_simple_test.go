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
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/images/kube-api-rewriter/pkg/rewriter"
)

func newEmptyRewriter() *rewriter.RuleBasedRewriter {
	return &rewriter.RuleBasedRewriter{
		Rules: &rewriter.RewriteRules{},
	}
}

func TestHandlerServeHTTPSuccess(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	t.Cleanup(upstream.Close)

	u, _ := url.Parse(upstream.URL)

	h := &Handler{
		Name:            "test",
		TargetClient:    upstream.Client(),
		TargetURL:       u,
		ProxyMode:       ToOriginal,
		Rewriter:        newEmptyRewriter(),
		MetricsProvider: NewMetricsProvider(),
	}
	h.Init()

	req := httptest.NewRequest(http.MethodPost, "http://example/api", nil)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", rr.Code)
	}
	if rr.Body.String() != `{"status":"ok"}` {
		t.Fatalf("unexpected body: %q", rr.Body.String())
	}
}

func TestHandlerServeHTTPUpstreamError(t *testing.T) {
	h := &Handler{
		Name:            "test",
		TargetClient:    &http.Client{},
		TargetURL:       &url.URL{Scheme: "http", Host: "127.0.0.1:0"},
		ProxyMode:       ToOriginal,
		Rewriter:        newEmptyRewriter(),
		MetricsProvider: NewMetricsProvider(),
	}
	h.Init()

	req := httptest.NewRequest(http.MethodGet, "http://example/api", nil)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}
