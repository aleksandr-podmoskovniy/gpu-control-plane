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

package proxy

import "testing"

func TestHandlerInit_Defaults(t *testing.T) {
	h := &Handler{
		Rewriter: newEmptyRewriter(),
	}
	h.Init()
	if h.MetricsProvider == nil {
		t.Fatalf("expected MetricsProvider to be initialized")
	}
	if h.streamHandler == nil {
		t.Fatalf("expected streamHandler to be initialized")
	}
}

func TestHandlerInit_KeepProvidedMetricsProvider(t *testing.T) {
	provider := NewMetricsProvider()
	h := &Handler{
		Rewriter:        newEmptyRewriter(),
		MetricsProvider: provider,
	}
	h.Init()
	if h.MetricsProvider != provider {
		t.Fatalf("expected MetricsProvider to be preserved")
	}
}
