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

package inventory

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type failingDeleteClient struct {
	client.Client
	err error
}

func (f *failingDeleteClient) Delete(context.Context, client.Object, ...client.DeleteOption) error {
	return f.err
}

func TestCleanupServiceRemoveOrphansBranches(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node"}}
	cl := newTestClient(scheme, node)

	cleanup := newCleanupService(cl, nil)
	if err := cleanup.RemoveOrphans(context.Background(), node, nil); err != nil {
		t.Fatalf("expected nil for empty orphan list, got %v", err)
	}

	bad := &failingDeleteClient{Client: cl, err: apierrors.NewBadRequest("boom")}
	cleanup = newCleanupService(bad, nil)
	orphan := map[string]struct{}{"dev": {}}
	if err := cleanup.RemoveOrphans(context.Background(), node, orphan); err == nil || !apierrors.IsBadRequest(err) {
		t.Fatalf("expected delete error, got %v", err)
	}

	// Ensure NotFound is ignored (and recorder nil doesn't crash).
	notFound := &failingDeleteClient{Client: cl, err: apierrors.NewNotFound(schema.GroupResource{Group: "gpu.deckhouse.io", Resource: "gpudevices"}, "dev")}
	cleanup = newCleanupService(notFound, nil)
	if err := cleanup.RemoveOrphans(context.Background(), node, orphan); err != nil {
		t.Fatalf("expected NotFound to be ignored: %v", err)
	}
}
