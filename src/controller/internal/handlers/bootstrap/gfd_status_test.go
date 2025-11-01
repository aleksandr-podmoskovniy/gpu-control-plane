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

package bootstrap

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/go-logr/logr/testr"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
)

func TestGFDStatusHandler(t *testing.T) {
	h := NewGFDStatusHandler(testr.New(t))
	if h.Name() == "" {
		t.Fatal("expected non-empty handler name")
	}

	inv := &gpuv1alpha1.GPUNodeInventory{}

	res, err := h.HandleNode(context.Background(), inv)
	if err != nil || !res.Requeue || res.RequeueAfter != 10*time.Second {
		t.Fatalf("expected requeue when condition missing, got %+v err=%v", res, err)
	}

	inv.Status.Conditions = []metav1.Condition{{Type: conditionGFDReady, Status: metav1.ConditionTrue}}
	res, err = h.HandleNode(context.Background(), inv)
	if err != nil || res.Requeue {
		t.Fatalf("expected success without requeue, got %+v err=%v", res, err)
	}
}
