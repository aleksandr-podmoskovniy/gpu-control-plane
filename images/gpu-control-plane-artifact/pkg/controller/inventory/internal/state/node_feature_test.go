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

package state

import (
	"context"
	"errors"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	nfdv1alpha1 "sigs.k8s.io/node-feature-discovery/api/nfd/v1alpha1"
)

func TestFindNodeFeaturePrefersExactMatch(t *testing.T) {
	scheme := newTestScheme(t)
	exact := &nfdv1alpha1.NodeFeature{ObjectMeta: metav1.ObjectMeta{Name: "worker-exact", ResourceVersion: "5"}}
	labeled := &nfdv1alpha1.NodeFeature{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "worker-exact-labeled",
			Namespace:       "gpu-operator",
			ResourceVersion: "7",
			Labels:          map[string]string{NodeFeatureNodeNameLabel: "worker-exact"},
		},
	}

	client := newTestClient(scheme, exact, labeled)
	feature, err := FindNodeFeature(context.Background(), client, "worker-exact")
	if err != nil {
		t.Fatalf("findNodeFeature returned error: %v", err)
	}
	if feature == nil || feature.GetName() != "worker-exact" {
		t.Fatalf("expected exact NodeFeature, got %+v", feature)
	}
}

func TestFindNodeFeatureSelectsNewestByResourceVersion(t *testing.T) {
	scheme := newTestScheme(t)
	older := &nfdv1alpha1.NodeFeature{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "nfd-old",
			Namespace:       "gpu-operator",
			ResourceVersion: "5",
			Labels:          map[string]string{NodeFeatureNodeNameLabel: "worker-rv"},
		},
	}
	newer := &nfdv1alpha1.NodeFeature{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "nfd-new",
			Namespace:       "gpu-operator",
			ResourceVersion: "8",
			Labels:          map[string]string{NodeFeatureNodeNameLabel: "worker-rv"},
		},
	}

	client := newTestClient(scheme, older, newer)
	feature, err := FindNodeFeature(context.Background(), client, "worker-rv")
	if err != nil {
		t.Fatalf("findNodeFeature returned error: %v", err)
	}
	if feature == nil || feature.GetName() != "nfd-new" {
		t.Fatalf("expected newest NodeFeature, got %+v", feature)
	}
}

func TestFindNodeFeatureReturnsNilWhenMissing(t *testing.T) {
	feature, err := FindNodeFeature(context.Background(), newTestClient(newTestScheme(t)), "absent")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if feature != nil {
		t.Fatalf("expected nil feature, got %+v", feature)
	}
}

func TestResourceVersionNewer(t *testing.T) {
	cases := []struct {
		name      string
		candidate string
		current   string
		expect    bool
	}{
		{"empty candidate", "", "10", false},
		{"empty current", "5", "", true},
		{"numeric greater", "6", "5", true},
		{"numeric smaller", "4", "5", false},
		{"candidate numeric current non-numeric", "5", "xyz", true},
		{"candidate non-numeric current numeric", "abc", "4", false},
		{"both non-numeric", "def", "abc", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resourceVersionNewer(tc.candidate, tc.current); got != tc.expect {
				t.Fatalf("resourceVersionNewer(%q,%q)=%v, want %v", tc.candidate, tc.current, got, tc.expect)
			}
		})
	}
}

func TestFindNodeFeatureGetError(t *testing.T) {
	scheme := newTestScheme(t)
	baseClient := newTestClient(scheme)
	boom := errors.New("get failed")
	client := &delegatingClient{
		Client: baseClient,
		get: func(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
			return boom
		},
	}

	_, err := FindNodeFeature(context.Background(), client, "node-error")
	if !errors.Is(err, boom) {
		t.Fatalf("expected get error, got %v", err)
	}
}

func TestFindNodeFeatureListError(t *testing.T) {
	scheme := newTestScheme(t)
	baseClient := newTestClient(scheme)
	boom := errors.New("list failed")
	client := &delegatingClient{
		Client: baseClient,
		get: func(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
			return apierrors.NewNotFound(schema.GroupResource{Group: "nfd", Resource: "nodefeatures"}, "node")
		},
		list: func(context.Context, client.ObjectList, ...client.ListOption) error {
			return boom
		},
	}

	_, err := FindNodeFeature(context.Background(), client, "node-list")
	if !errors.Is(err, boom) {
		t.Fatalf("expected list error, got %v", err)
	}
}

func TestChooseNodeFeaturePrefersExactName(t *testing.T) {
	items := []nfdv1alpha1.NodeFeature{
		{ObjectMeta: metav1.ObjectMeta{Name: "other", ResourceVersion: "1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node-1", ResourceVersion: "2"}},
	}
	selected := chooseNodeFeature(items, "node-1")
	if selected == nil || selected.GetName() != "node-1" {
		t.Fatalf("expected exact match, got %+v", selected)
	}
}

func TestChooseNodeFeatureUsesNewestResourceVersion(t *testing.T) {
	items := []nfdv1alpha1.NodeFeature{
		{ObjectMeta: metav1.ObjectMeta{Name: "nf-1", ResourceVersion: "5"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "nf-2", ResourceVersion: "7"}},
	}
	selected := chooseNodeFeature(items, "node")
	if selected == nil || selected.GetName() != "nf-2" {
		t.Fatalf("expected latest resource version, got %+v", selected)
	}
}

func TestChooseNodeFeatureHandlesEmptySlice(t *testing.T) {
	if chooseNodeFeature(nil, "node") != nil {
		t.Fatal("expected nil when slice empty")
	}
}
