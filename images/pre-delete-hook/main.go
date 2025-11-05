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
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
	_ "github.com/joho/godotenv/autoload"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type Resource struct {
	GVR       schema.GroupVersionResource `json:"gvr"`
	Name      string                      `json:"name"`
	Namespace string                      `json:"namespace,omitempty"`
	Selector  string                      `json:"selector,omitempty"`
}

func (r *Resource) gvrString() string {
	return fmt.Sprintf("%s %s/%s", r.GVR.Resource, r.GVR.Group, r.GVR.Version)
}

type PreDeleteHook struct {
	dynamicClient   dynamic.Interface
	resources       []Resource
	KubeConfigPath  string        `env:"KUBECONFIG"`
	ResourcesString string        `env:"RESOURCES"`
	WaitTimeout     time.Duration `env:"WAIT_TIMEOUT" env-default:"300s"`
}

func NewPreDeleteHook() (*PreDeleteHook, error) {
	hook := &PreDeleteHook{}

	if err := cleanenv.ReadEnv(hook); err != nil {
		return nil, fmt.Errorf("load environment: %w", err)
	}

	if hook.ResourcesString == "" {
		return nil, fmt.Errorf("RESOURCES env can't be empty")
	}

	if err := json.Unmarshal([]byte(hook.ResourcesString), &hook.resources); err != nil {
		return nil, fmt.Errorf("decode RESOURCES env: %w", err)
	}

	cfg, err := hook.buildConfig()
	if err != nil {
		return nil, fmt.Errorf("create kubernetes config: %w", err)
	}

	client, err := dynamicClientFactory(cfg)
	if err != nil {
		return nil, fmt.Errorf("create dynamic client: %w", err)
	}

	hook.dynamicClient = client
	return hook, nil
}

func (p *PreDeleteHook) buildConfig() (*rest.Config, error) {
	if p.KubeConfigPath != "" {
		return clientcmd.BuildConfigFromFlags("", p.KubeConfigPath)
	}
	return rest.InClusterConfig()
}

func (p *PreDeleteHook) Run(ctx context.Context) {
	if len(p.resources) == 0 {
		slog.Info("nothing to delete")
		return
	}

	var wg sync.WaitGroup
	for _, resource := range p.resources {
		res := resource

		slog.Info("Deleting resource ...",
			slog.String("gvr", res.gvrString()),
			slog.String("namespace", res.Namespace),
			slog.String("name", res.Name),
		)

		wg.Add(1)
		go func() {
			defer wg.Done()
			p.deleteResource(ctx, res)
		}()
	}

	wg.Wait()
}

func (p *PreDeleteHook) deleteResource(ctx context.Context, res Resource) {
	resourceClient := p.resourceClient(res)

	if res.Name == "" {
		if err := resourceClient.DeleteCollection(ctx, metav1.DeleteOptions{}, metav1.ListOptions{LabelSelector: res.Selector}); err != nil {
			p.handleDeleteError(err, res)
			return
		}

		if p.waitForCollectionRemoval(ctx, resourceClient, res) {
			return
		}

		slog.Error("Timeout waiting for collection deletion",
			slog.String("gvr", res.gvrString()),
			slog.String("namespace", res.Namespace),
			slog.String("selector", res.Selector),
		)
		return
	}

	if err := resourceClient.Delete(ctx, res.Name, metav1.DeleteOptions{}); err != nil {
		p.handleDeleteError(err, res)
		return
	}

	if p.waitForRemoval(ctx, resourceClient, res) {
		return
	}

	slog.Error("Timeout waiting for resource deletion",
		slog.String("gvr", res.gvrString()),
		slog.String("namespace", res.Namespace),
		slog.String("name", res.Name),
	)
}

func (p *PreDeleteHook) handleDeleteError(err error, res Resource) bool {
	if errors.IsNotFound(err) {
		slog.Info("Resource already absent",
			slog.String("gvr", res.gvrString()),
			slog.String("namespace", res.Namespace),
			slog.String("name", res.Name),
		)
		return true
	}

	slog.Error("Failed to delete resource",
		slog.Any("err", err),
		slog.String("gvr", res.gvrString()),
		slog.String("namespace", res.Namespace),
		slog.String("name", res.Name),
	)
	return true
}

func (p *PreDeleteHook) waitForRemoval(ctx context.Context, client dynamic.ResourceInterface, res Resource) bool {
	deadline := time.Now().Add(p.WaitTimeout)
	for time.Now().Before(deadline) {
		if _, err := client.Get(ctx, res.Name, metav1.GetOptions{}); errors.IsNotFound(err) {
			slog.Info("Resource is removed",
				slog.String("gvr", res.gvrString()),
				slog.String("namespace", res.Namespace),
				slog.String("name", res.Name),
			)
			return true
		} else if err != nil {
			slog.Error("Failed to check resource status",
				slog.Any("err", err),
				slog.String("gvr", res.gvrString()),
				slog.String("namespace", res.Namespace),
				slog.String("name", res.Name),
			)
			return true
		}

		select {
		case <-sleepAfter(2 * time.Second):
		case <-ctx.Done():
			slog.Error("Context cancelled while waiting resource removal",
				slog.Any("err", ctx.Err()),
				slog.String("gvr", res.gvrString()),
				slog.String("namespace", res.Namespace),
				slog.String("name", res.Name),
			)
			return true
		}
	}

	return false
}

func (p *PreDeleteHook) waitForCollectionRemoval(ctx context.Context, client dynamic.ResourceInterface, res Resource) bool {
	deadline := time.Now().Add(p.WaitTimeout)
	listOptions := metav1.ListOptions{LabelSelector: res.Selector}

	for time.Now().Before(deadline) {
		list, err := client.List(ctx, listOptions)
		if err != nil {
			slog.Error("Failed to list resources while waiting collection removal",
				slog.Any("err", err),
				slog.String("gvr", res.gvrString()),
				slog.String("namespace", res.Namespace),
				slog.String("selector", res.Selector),
			)
			return true
		}
		if len(list.Items) == 0 {
			slog.Info("Collection removed",
				slog.String("gvr", res.gvrString()),
				slog.String("namespace", res.Namespace),
				slog.String("selector", res.Selector),
			)
			return true
		}

		select {
		case <-sleepAfter(2 * time.Second):
		case <-ctx.Done():
			slog.Error("Context cancelled while waiting collection removal",
				slog.Any("err", ctx.Err()),
				slog.String("gvr", res.gvrString()),
				slog.String("namespace", res.Namespace),
				slog.String("selector", res.Selector),
			)
			return true
		}
	}

	return false
}

func (p *PreDeleteHook) resourceClient(res Resource) dynamic.ResourceInterface {
	ns := res.Namespace
	if ns == "" {
		ns = metav1.NamespaceNone
	}
	return p.dynamicClient.Resource(res.GVR).Namespace(ns)
}

func main() {
	ctx := context.Background()

	hook, err := newPreDeleteHook()
	if err != nil {
		slog.Error("Pre-delete hook initialisation failed", slog.Any("err", err))
		exitFunc(0)
		return
	}

	hook.Run(ctx)
}

var (
	newPreDeleteHook     = NewPreDeleteHook
	exitFunc             = os.Exit
	sleepAfter           = time.After
	dynamicClientFactory = dynamic.NewForConfig
)
