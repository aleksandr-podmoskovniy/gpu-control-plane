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

package service

import (
	"context"
	"errors"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client"

	invstate "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory/internal/state"
)

func TestDeviceServiceReconcileFetchError(t *testing.T) {
	scheme := newTestScheme(t)
	node := newTestNode("node-fetch-error")
	base := newTestClient(t, scheme, node)

	boom := errors.New("get failed")
	cl := &hookClient{
		Client: base,
		get: func(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
			return boom
		},
	}

	svc := NewDeviceService(cl, scheme, nil, nil)
	if _, _, err := svc.Reconcile(context.Background(), node, newTestSnapshot(), nil, true, invstate.DeviceApprovalPolicy{}, nil); !errors.Is(err, boom) {
		t.Fatalf("expected error %v, got %v", boom, err)
	}
}
