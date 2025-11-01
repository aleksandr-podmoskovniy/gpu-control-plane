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

package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func TestPreDeleteHookRunRemovesResources(t *testing.T) {
	scheme := runtime.NewScheme()
	client := dynamicfake.NewSimpleDynamicClient(scheme)

	gvr := schema.GroupVersionResource{
		Group:    "deckhouse.io",
		Version:  "v1alpha1",
		Resource: "nodefeaturerules",
	}

	namespacedGVR := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "configmaps",
	}

	clusterScoped := &unstructured.Unstructured{}
	if err := clusterScoped.UnmarshalJSON(mustJSON(`{
		"apiVersion":"deckhouse.io/v1alpha1",
		"kind":"NodeFeatureRule",
		"metadata":{"name":"rule-to-delete"}
	}`)); err != nil {
		t.Fatalf("prepare cluster resource: %v", err)
	}

	namespaced := &unstructured.Unstructured{}
	if err := namespaced.UnmarshalJSON(mustJSON(`{
		"apiVersion":"v1",
		"kind":"ConfigMap",
		"metadata":{"name":"cm-to-delete","namespace":"gpu"}
	}`)); err != nil {
		t.Fatalf("prepare namespaced resource: %v", err)
	}

	if err := client.Tracker().Add(clusterScoped); err != nil {
		t.Fatalf("add cluster resource: %v", err)
	}
	if err := client.Tracker().Add(namespaced); err != nil {
		t.Fatalf("add namespaced resource: %v", err)
	}

	hook := &PreDeleteHook{
		dynamicClient: client,
		resources: []Resource{
			{GVR: gvr, Name: "rule-to-delete"},
			{GVR: namespacedGVR, Name: "cm-to-delete", Namespace: "gpu"},
			{GVR: gvr, Name: "missing"},
		},
		WaitTimeout: 2 * time.Second,
	}

	hook.Run(context.Background())

	if _, err := client.Tracker().Get(gvr, metav1.NamespaceNone, "rule-to-delete"); !errors.IsNotFound(err) {
		t.Fatalf("expected cluster resource to be deleted, err=%v", err)
	}

	if _, err := client.Tracker().Get(namespacedGVR, "gpu", "cm-to-delete"); !errors.IsNotFound(err) {
		t.Fatalf("expected namespaced resource to be deleted, err=%v", err)
	}
}

func mustJSON(s string) []byte {
	var obj map[string]any
	if err := json.Unmarshal([]byte(s), &obj); err != nil {
		panic(err)
	}
	data, err := json.Marshal(obj)
	if err != nil {
		panic(err)
	}
	return data
}
