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

import (
	"testing"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/images/kube-api-rewriter/pkg/rewriter"
)

func TestProxyModeActions(t *testing.T) {
	if ToTargetAction(ToRenamed) != rewriter.Rename {
		t.Fatalf("expected ToTargetAction(ToRenamed) to return Rename")
	}
	if ToTargetAction(ToOriginal) != rewriter.Restore {
		t.Fatalf("expected ToTargetAction(ToOriginal) to return Restore")
	}

	if FromTargetAction(ToRenamed) != rewriter.Restore {
		t.Fatalf("expected FromTargetAction(ToRenamed) to return Restore")
	}
	if FromTargetAction(ToOriginal) != rewriter.Rename {
		t.Fatalf("expected FromTargetAction(ToOriginal) to return Rename")
	}

	// Unknown proxy mode should behave like ToOriginal.
	if ToTargetAction(ProxyMode("unknown")) != rewriter.Restore {
		t.Fatalf("expected ToTargetAction(unknown) to return Restore")
	}
	if FromTargetAction(ProxyMode("unknown")) != rewriter.Rename {
		t.Fatalf("expected FromTargetAction(unknown) to return Rename")
	}
}
