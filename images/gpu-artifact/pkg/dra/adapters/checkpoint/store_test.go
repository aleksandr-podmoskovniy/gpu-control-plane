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

package checkpoint

import (
	"context"
	"testing"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain"
)

func TestStoreNoop(t *testing.T) {
	s := NewStore()
	if s == nil {
		t.Fatalf("expected store to be constructed")
	}
	if err := s.Save(context.Background(), domain.PrepareRequest{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
