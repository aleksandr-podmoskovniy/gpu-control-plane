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

package validation

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	moduleLabelKey   = "module"
	moduleLabelValue = "gpu-control-plane"
	nodeNameField    = "spec.nodeName"
)

// Validator encapsulates bootstrap validation health checks for GPU nodes.
// Bootstrap workloads are rendered by Helm from upstream manifests.
// Status reports readiness/health of workloads running on the node.
type Validator interface {
	Status(ctx context.Context, nodeName string) (Result, error)
}

// Result is a structured view of validation health for a node.
type Result struct {
	Ready             bool
	DriverReady       bool
	ToolkitReady      bool
	GFDReady          bool
	DCGMReady         bool
	DCGMExporterReady bool
	MonitoringReady   bool
	Message           string
}

// Config configures how to locate bootstrap workloads.
type Config struct {
	WorkloadsNamespace string
	ValidatorApp       string
	GFDApp             string
	DCGMApp            string
	DCGMExporterApp    string
}

func NewValidator(cl client.Client, cfg Config) Validator {
	cfg = applyDefaults(cfg)
	return &workloadValidator{
		client:      cl,
		cfg:         cfg,
		allowedApps: allowedApps(cfg),
	}
}

type workloadValidator struct {
	client      client.Client
	cfg         Config
	allowedApps map[string]struct{}
}

func (v *workloadValidator) Status(ctx context.Context, nodeName string) (Result, error) {
	result := Result{}

	pods, err := v.listPods(ctx, nodeName)
	if err != nil {
		return result, err
	}

	validatorReady := hasReadyPod(pods.Items, v.cfg.ValidatorApp)
	gfdReady := hasReadyPod(pods.Items, v.cfg.GFDApp)
	dcgmReady := hasReadyPod(pods.Items, v.cfg.DCGMApp)
	exporterReady := hasReadyPod(pods.Items, v.cfg.DCGMExporterApp)

	result.DriverReady = validatorReady
	result.ToolkitReady = validatorReady
	result.GFDReady = gfdReady
	result.DCGMReady = dcgmReady
	result.DCGMExporterReady = exporterReady
	result.MonitoringReady = dcgmReady && exporterReady
	result.Ready = validatorReady && gfdReady && dcgmReady && exporterReady
	if !result.Ready {
		missing := make([]string, 0, 4)
		if !validatorReady {
			missing = append(missing, "validator")
		}
		if !gfdReady {
			missing = append(missing, "gfd")
		}
		if !dcgmReady {
			missing = append(missing, "dcgm")
		}
		if !exporterReady {
			missing = append(missing, "dcgm-exporter")
		}
		result.Message = fmt.Sprintf("validation workloads not ready: %s", strings.Join(missing, ", "))
	}

	return result, nil
}

func (v *workloadValidator) listPods(ctx context.Context, nodeName string) (*corev1.PodList, error) {
	pods := &corev1.PodList{}
	opts := []client.ListOption{
		client.InNamespace(v.cfg.WorkloadsNamespace),
		client.MatchingLabels{moduleLabelKey: moduleLabelValue},
	}
	if nodeName != "" {
		opts = append(opts, client.MatchingFields{nodeNameField: nodeName})
	}

	if err := v.client.List(ctx, pods, opts...); err != nil {
		if apierrors.IsNotFound(err) {
			return pods, nil
		}
		return nil, err
	}
	pods.Items = filterPods(pods.Items, nodeName, v.allowedApps)
	return pods, nil
}

func hasReadyPod(pods []corev1.Pod, app string) bool {
	for i := range pods {
		p := &pods[i]
		if p.Labels["app"] != app {
			continue
		}
		for _, cond := range p.Status.Conditions {
			if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
				return true
			}
		}
	}
	return false
}

func applyDefaults(cfg Config) Config {
	if cfg.WorkloadsNamespace == "" {
		cfg.WorkloadsNamespace = "d8-gpu-control-plane"
	}
	if cfg.ValidatorApp == "" {
		cfg.ValidatorApp = "nvidia-operator-validator"
	}
	if cfg.GFDApp == "" {
		cfg.GFDApp = "gpu-control-plane-gpu-feature-discovery"
	}
	if cfg.DCGMApp == "" {
		cfg.DCGMApp = "gpu-control-plane-dcgm"
	}
	if cfg.DCGMExporterApp == "" {
		cfg.DCGMExporterApp = "gpu-control-plane-dcgm-exporter"
	}
	return cfg
}

func allowedApps(cfg Config) map[string]struct{} {
	apps := []string{cfg.ValidatorApp, cfg.GFDApp, cfg.DCGMApp, cfg.DCGMExporterApp}
	out := make(map[string]struct{}, len(apps))
	for _, app := range apps {
		if app == "" {
			continue
		}
		out[app] = struct{}{}
	}
	return out
}

func filterPods(pods []corev1.Pod, nodeName string, allowed map[string]struct{}) []corev1.Pod {
	out := make([]corev1.Pod, 0, len(pods))
	for i := range pods {
		pod := pods[i]
		if nodeName != "" && pod.Spec.NodeName != nodeName {
			continue
		}
		if len(allowed) > 0 {
			app := ""
			if pod.Labels != nil {
				app = pod.Labels["app"]
			}
			if _, ok := allowed[app]; !ok {
				continue
			}
		}
		out = append(out, pod)
	}
	return out
}

type statusContextKey struct{}

// ContextWithStatus stores validator status in the context for downstream consumers.
func ContextWithStatus(ctx context.Context, status Result) context.Context {
	return context.WithValue(ctx, statusContextKey{}, status)
}

// StatusFromContext extracts validator status stored by ContextWithStatus.
func StatusFromContext(ctx context.Context) (Result, bool) {
	val, ok := ctx.Value(statusContextKey{}).(Result)
	return val, ok
}
