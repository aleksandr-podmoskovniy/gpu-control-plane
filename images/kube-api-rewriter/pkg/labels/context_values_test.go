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

package labels

import (
	"context"
	"testing"
)

func TestContextWithCommonAndAccessors(t *testing.T) {
	ctx := ContextWithCommon(context.Background(),
		"gpu", "devices", "GET", "watch",
		"send", "receive")
	ctx = ContextWithDecision(ctx, "rewrite")
	ctx = ContextWithStatus(ctx, 200)

	if got := NameFromContext(ctx); got != "gpu" {
		t.Fatalf("unexpected name: %q", got)
	}
	if got := ResourceFromContext(ctx); got != "devices" {
		t.Fatalf("unexpected resource: %q", got)
	}
	if got := MethodFromContext(ctx); got != "GET" {
		t.Fatalf("unexpected method: %q", got)
	}
	if got := WatchFromContext(ctx); got != "watch" {
		t.Fatalf("unexpected watch label: %q", got)
	}
	if got := ToTargetActionFromContext(ctx); got != "send" {
		t.Fatalf("unexpected toTargetAction: %q", got)
	}
	if got := FromTargetActionFromContext(ctx); got != "receive" {
		t.Fatalf("unexpected fromTargetAction: %q", got)
	}
	if got := DecisionFromContext(ctx); got != "rewrite" {
		t.Fatalf("unexpected decision: %q", got)
	}
	if got := StatusFromContext(ctx); got != "200" {
		t.Fatalf("unexpected status: %q", got)
	}
}

func TestContextAccessorsWithEmptyContext(t *testing.T) {
	ctx := context.Background()

	if NameFromContext(ctx) != "" {
		t.Fatalf("expected empty name")
	}
	if ResourceFromContext(ctx) != "" {
		t.Fatalf("expected empty resource")
	}
	if MethodFromContext(ctx) != "" {
		t.Fatalf("expected empty method")
	}
	if WatchFromContext(ctx) != "" {
		t.Fatalf("expected empty watch label")
	}
	if ToTargetActionFromContext(ctx) != "" {
		t.Fatalf("expected empty toTarget action")
	}
	if FromTargetActionFromContext(ctx) != "" {
		t.Fatalf("expected empty fromTarget action")
	}
	if DecisionFromContext(ctx) != "" {
		t.Fatalf("expected empty decision")
	}
	if StatusFromContext(ctx) != "" {
		t.Fatalf("expected empty status")
	}
}
