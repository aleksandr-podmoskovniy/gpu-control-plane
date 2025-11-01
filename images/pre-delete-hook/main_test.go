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
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

func TestResourceGVRString(t *testing.T) {
	res := Resource{GVR: schema.GroupVersionResource{Group: "deckhouse.io", Version: "v1", Resource: "tests"}}
	if got := res.gvrString(); got != "tests deckhouse.io/v1" {
		t.Fatalf("unexpected gvr string: %s", got)
	}
}

func TestNewPreDeleteHookRequiresResourcesEnv(t *testing.T) {
	t.Setenv("RESOURCES", "")
	if _, err := NewPreDeleteHook(); err == nil {
		t.Fatal("expected error when RESOURCES env is empty")
	}
}

func TestNewPreDeleteHookInvalidJSON(t *testing.T) {
	t.Setenv("RESOURCES", "not-json")
	if _, err := NewPreDeleteHook(); err == nil || !strings.Contains(err.Error(), "decode RESOURCES env") {
		t.Fatalf("expected decode error, got %v", err)
	}
}

func TestNewPreDeleteHookSuccess(t *testing.T) {
	t.Setenv("RESOURCES", `[{"gvr":{"group":"deckhouse.io","version":"v1","resource":"tests"},"name":"test"}]`)
	kubeconfig := writeTempKubeconfig(t)
	t.Setenv("KUBECONFIG", kubeconfig)

	hook, err := NewPreDeleteHook()
	if err != nil {
		t.Fatalf("unexpected error creating hook: %v", err)
	}
	if hook == nil || len(hook.resources) != 1 {
		t.Fatalf("expected hook with one resource, got %#v", hook)
	}
	if hook.dynamicClient == nil {
		t.Fatal("expected dynamic client to be initialised")
	}
}

func TestDeleteResourceHandlesDeleteError(t *testing.T) {
	resIface := &fakeResource{deleteErr: errors.New("boom")}
	hook := &PreDeleteHook{dynamicClient: &fakeDynamicClient{iface: &fakeNamespaceable{fakeResource: resIface}}}

	hook.deleteResource(context.Background(), Resource{
		GVR:  schema.GroupVersionResource{Group: "deckhouse.io", Version: "v1", Resource: "tests"},
		Name: "sample",
	})

	if resIface.deleteCalls.Load() != 1 {
		t.Fatalf("expected delete to be called once, got %d", resIface.deleteCalls.Load())
	}
}

func TestDeleteResourceHandlesNotFound(t *testing.T) {
	resIface := &fakeResource{deleteErr: kerrors.NewNotFound(schema.GroupResource{Group: "deckhouse.io", Resource: "tests"}, "missing")}
	hook := &PreDeleteHook{dynamicClient: &fakeDynamicClient{iface: &fakeNamespaceable{fakeResource: resIface}}}

	hook.deleteResource(context.Background(), Resource{
		GVR:  schema.GroupVersionResource{Group: "deckhouse.io", Version: "v1", Resource: "tests"},
		Name: "missing",
	})
}

func TestDeleteResourceSuccess(t *testing.T) {
	gr := schema.GroupResource{Group: "deckhouse.io", Resource: "tests"}
	gvr := schema.GroupVersionResource{Group: gr.Group, Version: "v1", Resource: gr.Resource}
	resIface := &fakeResource{
		getErrors: []error{kerrors.NewNotFound(gr, "test")},
	}
	nsStub := &fakeNamespaceable{fakeResource: resIface}
	hook := &PreDeleteHook{
		dynamicClient: &fakeDynamicClient{iface: nsStub},
		resources: []Resource{
			{GVR: gvr, Name: "test", Namespace: "ns-1"},
		},
		WaitTimeout: time.Second,
	}

	hook.Run(context.Background())

	if resIface.deleteCalls.Load() != 1 {
		t.Fatalf("expected delete to be called once, got %d", resIface.deleteCalls.Load())
	}
	if nsStub.namespace != "ns-1" {
		t.Fatalf("expected namespace ns-1, got %s", nsStub.namespace)
	}
}

func TestNewPreDeleteHookEnvError(t *testing.T) {
	t.Setenv("RESOURCES", `[{"gvr":{"group":"deckhouse.io","version":"v1","resource":"tests"},"name":"test"}]`)
	t.Setenv("WAIT_TIMEOUT", "not-a-duration")

	if _, err := NewPreDeleteHook(); err == nil || !strings.Contains(err.Error(), "load environment") {
		t.Fatalf("expected environment error, got %v", err)
	}
}

func TestNewPreDeleteHookBuildConfigError(t *testing.T) {
	t.Setenv("RESOURCES", `[{"gvr":{"group":"deckhouse.io","version":"v1","resource":"tests"},"name":"test"}]`)
	t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "missing"))

	if _, err := NewPreDeleteHook(); err == nil || !strings.Contains(err.Error(), "create kubernetes config") {
		t.Fatalf("expected config error, got %v", err)
	}
}

func TestNewPreDeleteHookDynamicClientError(t *testing.T) {
	t.Setenv("RESOURCES", `[{"gvr":{"group":"deckhouse.io","version":"v1","resource":"tests"},"name":"test"}]`)
	t.Setenv("KUBECONFIG", writeTempKubeconfig(t))
	originalFactory := dynamicClientFactory
	dynamicClientFactory = func(*rest.Config) (*dynamic.DynamicClient, error) {
		return nil, errors.New("dyn fail")
	}
	defer func() { dynamicClientFactory = originalFactory }()

	if _, err := NewPreDeleteHook(); err == nil || !strings.Contains(err.Error(), "dynamic client") {
		t.Fatalf("expected dynamic client error, got %v", err)
	}
}
func TestDeleteResourceImmediateRemoval(t *testing.T) {
	gr := schema.GroupResource{Group: "deckhouse.io", Resource: "tests"}
	gvr := schema.GroupVersionResource{Group: gr.Group, Version: "v1", Resource: gr.Resource}
	resIface := &fakeResource{getErrors: []error{kerrors.NewNotFound(gr, "test")}}
	hook := &PreDeleteHook{
		dynamicClient: &fakeDynamicClient{iface: &fakeNamespaceable{fakeResource: resIface}},
		WaitTimeout:   time.Second,
	}

	hook.deleteResource(context.Background(), Resource{GVR: gvr, Name: "test"})

	if resIface.deleteCalls.Load() != 1 {
		t.Fatalf("expected delete to be called once, got %d", resIface.deleteCalls.Load())
	}
}

func TestDeleteResourceContextCancelled(t *testing.T) {
	gr := schema.GroupResource{Group: "deckhouse.io", Resource: "tests"}
	gvr := schema.GroupVersionResource{Group: gr.Group, Version: "v1", Resource: gr.Resource}
	resIface := &fakeResource{}
	hook := &PreDeleteHook{dynamicClient: &fakeDynamicClient{iface: &fakeNamespaceable{fakeResource: resIface}}}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	hook.deleteResource(ctx, Resource{GVR: gvr, Name: "test"})

	if resIface.deleteCalls.Load() != 1 {
		t.Fatalf("expected delete to be called once, got %d", resIface.deleteCalls.Load())
	}
}

func TestDeleteResourceGetError(t *testing.T) {
	resIface := &fakeResource{getErrors: []error{errors.New("get fail")}}
	hook := &PreDeleteHook{dynamicClient: &fakeDynamicClient{iface: &fakeNamespaceable{fakeResource: resIface}}}

	hook.deleteResource(context.Background(), Resource{
		GVR:  schema.GroupVersionResource{Group: "deckhouse.io", Version: "v1", Resource: "tests"},
		Name: "trouble",
	})

	if resIface.deleteCalls.Load() != 1 {
		t.Fatalf("expected delete to be called once, got %d", resIface.deleteCalls.Load())
	}
}

func TestDeleteResourceTimeout(t *testing.T) {
	resIface := &fakeResource{}
	hook := &PreDeleteHook{
		dynamicClient: &fakeDynamicClient{iface: &fakeNamespaceable{fakeResource: resIface}},
		WaitTimeout:   0,
	}

	hook.deleteResource(context.Background(), Resource{
		GVR:  schema.GroupVersionResource{Group: "deckhouse.io", Version: "v1", Resource: "tests"},
		Name: "stuck",
	})

	if resIface.deleteCalls.Load() != 1 {
		t.Fatalf("expected delete to be called once, got %d", resIface.deleteCalls.Load())
	}
}

func TestDeleteResourceWaitLoop(t *testing.T) {
	gr := schema.GroupResource{Group: "deckhouse.io", Resource: "tests"}
	gvr := schema.GroupVersionResource{Group: gr.Group, Version: "v1", Resource: gr.Resource}
	resIface := &fakeResource{
		getErrors: []error{nil, kerrors.NewNotFound(gr, "test")},
	}
	hook := &PreDeleteHook{
		dynamicClient: &fakeDynamicClient{iface: &fakeNamespaceable{fakeResource: resIface}},
		WaitTimeout:   time.Second,
	}

	originalSleep := sleepAfter
	sleepAfter = func(time.Duration) <-chan time.Time {
		ch := make(chan time.Time, 1)
		ch <- time.Now()
		return ch
	}
	defer func() { sleepAfter = originalSleep }()

	hook.deleteResource(context.Background(), Resource{GVR: gvr, Name: "test"})

	if resIface.getIndex < 2 {
		t.Fatalf("expected multiple get attempts, got %d", resIface.getIndex)
	}
}

func TestResourceClientRespectsNamespace(t *testing.T) {
	nsStub := &fakeNamespaceable{fakeResource: &fakeResource{}}
	client := &fakeDynamicClient{iface: nsStub}
	hook := &PreDeleteHook{dynamicClient: client}

	hook.resourceClient(Resource{
		GVR:       schema.GroupVersionResource{Group: "deckhouse.io", Version: "v1", Resource: "tests"},
		Namespace: "deckhouse",
	})

	if nsStub.namespace != "deckhouse" {
		t.Fatalf("expected namespace to be recorded, got %s", nsStub.namespace)
	}
}

func TestRunSkipsWhenNoResources(t *testing.T) {
	hook := &PreDeleteHook{}
	hook.Run(context.Background())
}

func TestMainHandlesInitialisationError(t *testing.T) {
	t.Cleanup(func() {
		newPreDeleteHook = NewPreDeleteHook
		exitFunc = os.Exit
	})

	newPreDeleteHook = func() (*PreDeleteHook, error) {
		return nil, errors.New("boom")
	}

	var exited atomic.Int32
	exitFunc = func(code int) { exited.Store(int32(code)) }

	main()

	if exited.Load() != 0 {
		t.Fatalf("expected custom exitFunc to be invoked with code 0, got %d", exited.Load())
	}
}

func TestMainExecutesHook(t *testing.T) {
	t.Cleanup(func() {
		newPreDeleteHook = NewPreDeleteHook
		exitFunc = os.Exit
	})

	gr := schema.GroupResource{Group: "deckhouse.io", Resource: "tests"}
	gvr := schema.GroupVersionResource{Group: gr.Group, Version: "v1", Resource: gr.Resource}
	resIface := &fakeResource{
		getErrors: []error{kerrors.NewNotFound(gr, "test")},
	}
	nsStub := &fakeNamespaceable{fakeResource: resIface}

	newPreDeleteHook = func() (*PreDeleteHook, error) {
		return &PreDeleteHook{
			dynamicClient: &fakeDynamicClient{iface: nsStub},
			resources: []Resource{
				{GVR: gvr, Name: "test"},
			},
			WaitTimeout: time.Second,
		}, nil
	}
	exitFunc = func(int) {}

	main()

	if resIface.deleteCalls.Load() != 1 {
		t.Fatalf("expected delete to be called once, got %d", resIface.deleteCalls.Load())
	}
}

func TestBuildConfigUsesKubeconfig(t *testing.T) {
	hook := &PreDeleteHook{KubeConfigPath: writeTempKubeconfig(t)}
	if _, err := hook.buildConfig(); err != nil {
		t.Fatalf("expected buildConfig to succeed, got %v", err)
	}
}

func TestWaitForRemovalHandlesNotFound(t *testing.T) {
	gr := schema.GroupResource{Group: "deckhouse.io", Resource: "tests"}
	hook := &PreDeleteHook{WaitTimeout: time.Second}
	client := &fakeResource{getErrors: []error{kerrors.NewNotFound(gr, "test")}}
	res := Resource{GVR: schema.GroupVersionResource{Group: gr.Group, Version: "v1", Resource: gr.Resource}, Name: "test"}

	if !hook.waitForRemoval(context.Background(), client, res) {
		t.Fatal("expected waitForRemoval to report success on not found")
	}
}

func TestWaitForRemovalHandlesError(t *testing.T) {
	hook := &PreDeleteHook{WaitTimeout: time.Second}
	client := &fakeResource{getErrors: []error{errors.New("get fail")}}
	res := Resource{GVR: schema.GroupVersionResource{Group: "deckhouse.io", Version: "v1", Resource: "tests"}, Name: "test"}

	if !hook.waitForRemoval(context.Background(), client, res) {
		t.Fatal("expected waitForRemoval to stop on error")
	}
}

func TestWaitForRemovalContextCancelled(t *testing.T) {
	hook := &PreDeleteHook{WaitTimeout: time.Second}
	client := &fakeResource{}
	res := Resource{GVR: schema.GroupVersionResource{Group: "deckhouse.io", Version: "v1", Resource: "tests"}, Name: "test"}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	originalSleep := sleepAfter
	sleepAfter = func(time.Duration) <-chan time.Time {
		ch := make(chan time.Time, 1)
		return ch
	}
	defer func() { sleepAfter = originalSleep }()

	if !hook.waitForRemoval(ctx, client, res) {
		t.Fatal("expected waitForRemoval to exit on context cancellation")
	}
}

func TestWaitForRemovalTimeout(t *testing.T) {
	hook := &PreDeleteHook{WaitTimeout: 0}
	client := &fakeResource{}
	res := Resource{GVR: schema.GroupVersionResource{Group: "deckhouse.io", Version: "v1", Resource: "tests"}, Name: "test"}

	if hook.waitForRemoval(context.Background(), client, res) {
		t.Fatal("expected waitForRemoval to report timeout")
	}
}
func TestBuildConfigInClusterError(t *testing.T) {
	hook := &PreDeleteHook{}
	if _, err := hook.buildConfig(); err == nil {
		t.Fatal("expected in-cluster configuration to fail in tests")
	}
}

type fakeDynamicClient struct {
	iface dynamic.NamespaceableResourceInterface
}

func (f *fakeDynamicClient) Resource(schema.GroupVersionResource) dynamic.NamespaceableResourceInterface {
	return f.iface
}

type fakeNamespaceable struct {
	*fakeResource
	namespace string
}

func (f *fakeNamespaceable) Namespace(ns string) dynamic.ResourceInterface {
	f.namespace = ns
	return f
}

func (f *fakeNamespaceable) Cluster(ns string) dynamic.ResourceInterface {
	f.namespace = ns
	return f
}

type fakeResource struct {
	deleteErr   error
	getErrors   []error
	getIndex    int
	deleteCalls atomic.Int32
}

func (f *fakeResource) Create(context.Context, *unstructured.Unstructured, metav1.CreateOptions, ...string) (*unstructured.Unstructured, error) {
	return nil, nil
}

func (f *fakeResource) Update(context.Context, *unstructured.Unstructured, metav1.UpdateOptions, ...string) (*unstructured.Unstructured, error) {
	return nil, nil
}

func (f *fakeResource) UpdateStatus(context.Context, *unstructured.Unstructured, metav1.UpdateOptions) (*unstructured.Unstructured, error) {
	return nil, nil
}

func (f *fakeResource) Delete(context.Context, string, metav1.DeleteOptions, ...string) error {
	f.deleteCalls.Add(1)
	return f.deleteErr
}

func (f *fakeResource) DeleteCollection(context.Context, metav1.DeleteOptions, metav1.ListOptions) error {
	return nil
}

func (f *fakeResource) Get(context.Context, string, metav1.GetOptions, ...string) (*unstructured.Unstructured, error) {
	if f.getIndex < len(f.getErrors) {
		err := f.getErrors[f.getIndex]
		f.getIndex++
		return nil, err
	}
	return &unstructured.Unstructured{}, nil
}

func (f *fakeResource) List(context.Context, metav1.ListOptions) (*unstructured.UnstructuredList, error) {
	return nil, nil
}

func (f *fakeResource) Watch(context.Context, metav1.ListOptions) (watch.Interface, error) {
	return nil, nil
}

func (f *fakeResource) Patch(context.Context, string, types.PatchType, []byte, metav1.PatchOptions, ...string) (*unstructured.Unstructured, error) {
	return nil, nil
}

func (f *fakeResource) Apply(context.Context, string, *unstructured.Unstructured, metav1.ApplyOptions, ...string) (*unstructured.Unstructured, error) {
	return nil, nil
}

func (f *fakeResource) ApplyStatus(context.Context, string, *unstructured.Unstructured, metav1.ApplyOptions) (*unstructured.Unstructured, error) {
	return nil, nil
}

func writeTempKubeconfig(t *testing.T) string {
	t.Helper()
	content := `
apiVersion: v1
clusters:
- cluster:
    server: https://127.0.0.1
  name: fake
contexts:
- context:
    cluster: fake
    user: fake
  name: fake
current-context: fake
users:
- name: fake
  user:
    token: fake
`
	path := filepath.Join(t.TempDir(), "kubeconfig")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}
	return path
}
