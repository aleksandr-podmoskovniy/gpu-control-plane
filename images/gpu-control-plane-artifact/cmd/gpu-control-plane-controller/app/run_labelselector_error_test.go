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

package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/rest"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/config"
)

func TestRunFailsOnPodCacheLabelSelectorError(t *testing.T) {
	orig := newLabelRequirement
	t.Cleanup(func() { newLabelRequirement = orig })

	newLabelRequirement = func(string, selection.Operator, []string, ...field.PathOption) (*labels.Requirement, error) {
		return nil, errors.New("requirement fail")
	}

	t.Setenv("TLS_CERT_FILE", "")
	t.Setenv("TLS_PRIVATE_KEY_FILE", "")
	t.Setenv("TLS_CA_FILE", "")

	err := Run(context.Background(), &rest.Config{}, config.DefaultSystem())
	if err == nil || !strings.Contains(err.Error(), "build pod cache label selector") {
		t.Fatalf("expected selector build error, got %v", err)
	}
}
