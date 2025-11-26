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

package handlers

import (
	"testing"

	"github.com/go-logr/logr/testr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestRegisterDefaultsWithClient(t *testing.T) {
	scheme := fake.NewClientBuilder().Build().Scheme()
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()

	deps := &Handlers{Client: cl}
	RegisterDefaults(testr.New(t), deps)

	if len(deps.Pool.List()) == 0 {
		t.Fatalf("expected pool handlers registered")
	}
	if len(deps.Admission.List()) == 0 {
		t.Fatalf("expected admission handlers registered")
	}
}
