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

//go:build !linux

package detect

import "testing"

func TestStubInitFails(t *testing.T) {
	if err := initNVML(); err == nil {
		t.Fatalf("expected initNVML to fail on stub")
	}
	if _, err := queryNVML(); err == nil {
		t.Fatalf("expected queryNVML to fail on stub")
	}
	if err := shutdownNVML(); err != nil {
		t.Fatalf("shutdown should be noop, got %v", err)
	}
}
