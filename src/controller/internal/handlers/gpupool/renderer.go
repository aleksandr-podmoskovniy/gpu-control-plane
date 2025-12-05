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

package gpupool

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"strings"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
)

// Embedded assets reused from MIG manager template to stay consistent with node-manager.
//
//go:embed assets/reconfigure-mig.sh
var migReconfigureScript string

//go:embed assets/prestop.sh
var migPrestopScript string

//go:embed assets/gpu-clients.yaml
var migGPUClients string

// RenderConfig carries per-pool workload rendering settings.
type RenderConfig struct {
	Namespace            string
	DevicePluginImage    string
	MIGManagerImage      string
	DefaultMIGStrategy   string
	CustomTolerationKeys []string
	ValidatorImage       string
}

// RendererHandler ensures per-pool workloads (device-plugin, MIG manager) are deployed.
type RendererHandler struct {
	log               logr.Logger
	client            client.Client
	cfg               RenderConfig
	customTolerations []corev1.Toleration
}

// NewRendererHandler builds a RendererHandler using env fallbacks.
func NewRendererHandler(log logr.Logger, c client.Client, cfg RenderConfig) *RendererHandler {
	defaults := renderConfigFromEnv()
	if cfg.Namespace == "" {
		cfg.Namespace = defaults.Namespace
	}
	if cfg.DevicePluginImage == "" {
		cfg.DevicePluginImage = defaults.DevicePluginImage
	}
	if cfg.MIGManagerImage == "" {
		cfg.MIGManagerImage = defaults.MIGManagerImage
	}
	if cfg.DefaultMIGStrategy == "" {
		cfg.DefaultMIGStrategy = defaults.DefaultMIGStrategy
	}
	if cfg.ValidatorImage == "" {
		if defaults.ValidatorImage != "" {
			cfg.ValidatorImage = defaults.ValidatorImage
		} else {
			cfg.ValidatorImage = cfg.DevicePluginImage
		}
	}
	return &RendererHandler{
		log:               log,
		client:            c,
		cfg:               cfg,
		customTolerations: buildCustomTolerations(cfg.CustomTolerationKeys),
	}
}

func poolResourceName(pool *v1alpha1.GPUPool) string {
	prefix := "gpu.deckhouse.io"
	// Cluster-scoped pools must always expose a distinct prefix to avoid collisions with namespaced pools.
	if pool != nil && (pool.Namespace == "" || pool.Kind == "ClusterGPUPool") {
		prefix = "cluster.gpu.deckhouse.io"
	}
	return fmt.Sprintf("%s/%s", prefix, pool.Name)
}

func renderConfigFromEnv() RenderConfig {
	ns := strings.TrimSpace(os.Getenv("POD_NAMESPACE"))
	if ns == "" {
		ns = "d8-gpu-control-plane"
	}
	strategy := strings.TrimSpace(os.Getenv("DEFAULT_MIG_STRATEGY"))
	if strategy == "" {
		strategy = "none"
	}
	return RenderConfig{
		Namespace:          ns,
		DevicePluginImage:  strings.TrimSpace(os.Getenv("NVIDIA_DEVICE_PLUGIN_IMAGE")),
		MIGManagerImage:    strings.TrimSpace(os.Getenv("NVIDIA_MIG_MANAGER_IMAGE")),
		DefaultMIGStrategy: strategy,
		ValidatorImage:     strings.TrimSpace(os.Getenv("NVIDIA_VALIDATOR_IMAGE")),
	}
}

func (h *RendererHandler) Name() string {
	return "renderer"
}

func (h *RendererHandler) HandlePool(ctx context.Context, pool *v1alpha1.GPUPool) (contracts.Result, error) {
	if h.client == nil {
		return contracts.Result{}, fmt.Errorf("client is required")
	}
	if h.cfg.Namespace == "" {
		return contracts.Result{}, fmt.Errorf("namespace is not configured")
	}
	if h.cfg.DevicePluginImage == "" {
		return contracts.Result{}, fmt.Errorf("device-plugin image is not configured")
	}

	// Only Nvidia/DevicePlugin supported for now.
	if pool.Spec.Provider != "" && pool.Spec.Provider != "Nvidia" {
		return contracts.Result{}, nil
	}
	if pool.Spec.Backend != "" && pool.Spec.Backend != "DevicePlugin" {
		return contracts.Result{}, h.cleanupPoolResources(ctx, pool.Name)
	}
	if pool.Status.Capacity.Total == 0 {
		return contracts.Result{}, h.cleanupPoolResources(ctx, pool.Name)
	}
	if err := h.reconcileDevicePlugin(ctx, pool); err != nil {
		return contracts.Result{}, err
	}
	if err := h.reconcileValidator(ctx, pool); err != nil {
		return contracts.Result{}, err
	}

	if strings.EqualFold(pool.Spec.Resource.Unit, "MIG") {
		if h.cfg.MIGManagerImage == "" {
			h.log.Info("MIG pool detected but MIG manager image not configured, skipping MIG manager rendering", "pool", pool.Name)
		} else if err := h.reconcileMIGManager(ctx, pool); err != nil {
			return contracts.Result{}, err
		}
	} else {
		if err := h.cleanupMIGResources(ctx, pool.Name); err != nil {
			return contracts.Result{}, err
		}
	}

	return contracts.Result{}, nil
}

func (h *RendererHandler) reconcileDevicePlugin(ctx context.Context, pool *v1alpha1.GPUPool) error {
	cm := h.devicePluginConfigMap(pool)
	if err := h.createOrUpdate(ctx, cm, pool); err != nil {
		return fmt.Errorf("reconcile device-plugin ConfigMap: %w", err)
	}

	ds := h.devicePluginDaemonSet(ctx, pool)
	if err := h.createOrUpdate(ctx, ds, pool); err != nil {
		return fmt.Errorf("reconcile device-plugin DaemonSet: %w", err)
	}

	return nil
}

func (h *RendererHandler) reconcileValidator(ctx context.Context, pool *v1alpha1.GPUPool) error {
	if h.cfg.ValidatorImage == "" {
		return fmt.Errorf("validator image is not configured")
	}

	ds := h.validatorDaemonSet(ctx, pool)
	if err := h.createOrUpdate(ctx, ds, pool); err != nil {
		return fmt.Errorf("reconcile validator DaemonSet: %w", err)
	}

	return nil
}

func (h *RendererHandler) reconcileMIGManager(ctx context.Context, pool *v1alpha1.GPUPool) error {
	configCM := h.migManagerConfigMap(pool)
	if err := h.createOrUpdate(ctx, configCM, pool); err != nil {
		return fmt.Errorf("reconcile MIG manager config: %w", err)
	}

	scriptsCM := h.migManagerScriptsConfigMap(pool)
	if err := h.createOrUpdate(ctx, scriptsCM, pool); err != nil {
		return fmt.Errorf("reconcile MIG manager scripts: %w", err)
	}

	clientsCM := h.migManagerClientsConfigMap(pool)
	if err := h.createOrUpdate(ctx, clientsCM, pool); err != nil {
		return fmt.Errorf("reconcile MIG manager clients: %w", err)
	}

	ds := h.migManagerDaemonSet(ctx, pool)
	if err := h.createOrUpdate(ctx, ds, pool); err != nil {
		return fmt.Errorf("reconcile MIG manager DaemonSet: %w", err)
	}
	return nil
}

func (h *RendererHandler) devicePluginConfigMap(pool *v1alpha1.GPUPool) *corev1.ConfigMap {
	resourcePrefix := poolPrefix(pool)
	resourceName := pool.Name
	replicas := h.timeSlicingReplicas(pool)

	// Normalize resource name/prefix if the pool name already contains a prefix.
	if strings.Contains(resourceName, "/") {
		parts := strings.Split(resourceName, "/")
		if len(parts) > 1 {
			inferredPrefix := strings.Join(parts[:len(parts)-1], "/")
			// If the name already carries the cluster prefix, align resourcePrefix with it.
			if inferredPrefix == "cluster.gpu.deckhouse.io" {
				resourcePrefix = inferredPrefix
			}
			resourceName = parts[len(parts)-1]
		}
	}

	resources := make([]map[string]any, 0, len(pool.Spec.Resource.TimeSlicingResources)+1)
	for _, ts := range pool.Spec.Resource.TimeSlicingResources {
		if ts.SlicesPerUnit < 1 {
			continue
		}
		name := ts.Name
		if name == "" {
			name = pool.Name
		}
		if strings.Contains(name, "/") {
			parts := strings.Split(name, "/")
			if len(parts) > 1 && parts[0] == "cluster.gpu.deckhouse.io" {
				// Keep the cluster prefix consistent even if user provided a prefixed name override.
				resourcePrefix = "cluster.gpu.deckhouse.io"
			}
			name = parts[len(parts)-1]
		}
		resources = append(resources, map[string]any{
			"name":     name,
			"replicas": int(ts.SlicesPerUnit),
		})
	}
	if len(resources) == 0 {
		resources = append(resources, map[string]any{
			"name":     pool.Name,
			"replicas": int(replicas),
		})
	}

	hasSharing := false
	for _, r := range resources {
		if v, ok := r["replicas"].(int); ok && v > 1 {
			hasSharing = true
		}
	}

	cfg := map[string]any{
		"version": "v1",
		"flags": map[string]any{
			"migStrategy":    h.cfg.DefaultMIGStrategy,
			"resourcePrefix": resourcePrefix,
		},
	}

	cfg["resources"] = map[string]any{
		"gpus": []map[string]any{
			{
				"pattern": "*",
				"name":    resourceName,
			},
		},
	}

	if hasSharing {
		cfg["sharing"] = map[string]any{
			"timeSlicing": map[string]any{
				"resources": resources,
			},
		}
	}

	data, _ := yaml.Marshal(cfg)
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("nvidia-device-plugin-%s-config", pool.Name),
			Namespace: h.cfg.Namespace,
			Labels: map[string]string{
				"app":  "nvidia-device-plugin",
				"pool": pool.Name,
			},
		},
		Data: map[string]string{"config.yaml": string(data)},
	}
}

func (h *RendererHandler) timeSlicingReplicas(pool *v1alpha1.GPUPool) int32 {
	replicas := int32(1)
	if pool.Spec.Resource.SlicesPerUnit > 0 {
		replicas = pool.Spec.Resource.SlicesPerUnit
	}
	for _, layout := range pool.Spec.Resource.MIGLayout {
		if layout.SlicesPerUnit != nil && *layout.SlicesPerUnit > 0 {
			replicas = *layout.SlicesPerUnit
		}
		for _, p := range layout.Profiles {
			if p.SlicesPerUnit != nil && *p.SlicesPerUnit > 0 {
				replicas = *p.SlicesPerUnit
			}
		}
	}
	return replicas
}

func (h *RendererHandler) devicePluginDaemonSet(ctx context.Context, pool *v1alpha1.GPUPool) *appsv1.DaemonSet {
	poolKey := poolLabelKey(pool)
	tolerations := mergeTolerations([]corev1.Toleration{
		{
			Key:      poolKey,
			Operator: corev1.TolerationOpEqual,
			Value:    pool.Name,
			Effect:   corev1.TaintEffectNoSchedule,
		},
	}, append(h.customTolerations, h.poolNodeTolerations(ctx, pool)...))
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("nvidia-device-plugin-%s", pool.Name),
			Namespace: h.cfg.Namespace,
			Labels: map[string]string{
				"app":  "nvidia-device-plugin",
				"pool": pool.Name,
			},
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":  "nvidia-device-plugin",
					"pool": pool.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":    "nvidia-device-plugin",
						"pool":   pool.Name,
						"module": "gpu-control-plane",
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: "nvidia-device-plugin",
					Tolerations:        tolerations,
					SecurityContext: &corev1.PodSecurityContext{
						RunAsUser:    ptr.To[int64](0),
						RunAsNonRoot: ptr.To(false),
					},
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      poolKey,
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{pool.Name},
											},
										},
									},
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:            "device-plugin",
							Image:           h.cfg.DevicePluginImage,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Command:         []string{"/nvidia-device-plugin"},
							SecurityContext: &corev1.SecurityContext{
								Privileged:               ptr.To(true),
								RunAsUser:                ptr.To[int64](0),
								RunAsNonRoot:             ptr.To(false),
								AllowPrivilegeEscalation: ptr.To(true),
								ReadOnlyRootFilesystem:   ptr.To(false),
							},
							Args: []string{"--config-file=/config/config.yaml", "--pass-device-specs=false", "--fail-on-init-error=false"},
							Env: []corev1.EnvVar{
								{Name: "NVIDIA_VISIBLE_DEVICES", Value: "all"},
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "device-plugin", MountPath: "/var/lib/kubelet/device-plugins"},
								{Name: "config", MountPath: "/config"},
								{Name: "dev", MountPath: "/dev"},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "device-plugin",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/var/lib/kubelet/device-plugins",
									Type: hostPathType(corev1.HostPathDirectory),
								},
							},
						},
						{
							Name: "dev",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/dev",
									Type: hostPathType(corev1.HostPathDirectory),
								},
							},
						},
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: fmt.Sprintf("nvidia-device-plugin-%s-config", pool.Name),
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func (h *RendererHandler) validatorDaemonSet(ctx context.Context, pool *v1alpha1.GPUPool) *appsv1.DaemonSet {
	poolKey := poolLabelKey(pool)
	tolerations := mergeTolerations([]corev1.Toleration{
		{
			Effect:   corev1.TaintEffectNoSchedule,
			Key:      poolKey,
			Operator: corev1.TolerationOpEqual,
			Value:    poolValueFromKey(poolKey),
		},
	}, append(h.customTolerations, h.poolNodeTolerations(ctx, pool)...))
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("nvidia-operator-validator-%s", pool.Name),
			Namespace: h.cfg.Namespace,
			Labels: map[string]string{
				"app":  "nvidia-operator-validator",
				"pool": pool.Name,
			},
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":  "nvidia-operator-validator",
					"pool": pool.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":    "nvidia-operator-validator",
						"pool":   pool.Name,
						"module": "gpu-control-plane",
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: "nvidia-operator-validator",
					Tolerations:        tolerations,
					SecurityContext: &corev1.PodSecurityContext{
						RunAsUser:    ptr.To[int64](0),
						RunAsNonRoot: ptr.To(false),
					},
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      poolKey,
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{pool.Name},
											},
										},
									},
								},
							},
						},
					},
					InitContainers: []corev1.Container{
						{
							Name:            "plugin-validation",
							Image:           h.cfg.ValidatorImage,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Command:         []string{"/usr/bin/nvidia-validator"},
							SecurityContext: &corev1.SecurityContext{
								Privileged:               ptr.To(true),
								RunAsUser:                ptr.To[int64](0),
								RunAsNonRoot:             ptr.To(false),
								AllowPrivilegeEscalation: ptr.To(true),
								ReadOnlyRootFilesystem:   ptr.To(false),
							},
							Env: []corev1.EnvVar{
								{Name: "PATH", Value: "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"},
								{Name: "COMPONENT", Value: "plugin"},
								{Name: "WITH_WAIT", Value: "true"},
								{Name: "WITH_WORKLOAD", Value: "false"},
								// Validator must look for the exact resource name exposed by the device plugin (prefix + pool name).
								{Name: "NVIDIA_RESOURCE_NAME", Value: poolResourceName(pool)},
								{Name: "MIG_STRATEGY", Value: h.cfg.DefaultMIGStrategy},
								{
									Name: "NODE_NAME",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{FieldPath: "spec.nodeName"},
									},
								},
								{
									Name: "OPERATOR_NAMESPACE",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.namespace"},
									},
								},
								{Name: "OUTPUT_DIR", Value: "/run/nvidia/validations"},
								{Name: "VALIDATOR_IMAGE", Value: h.cfg.ValidatorImage},
								{Name: "VALIDATOR_IMAGE_PULL_POLICY", Value: "IfNotPresent"},
								{Name: "VALIDATOR_RUNTIME_CLASS", Value: "nvidia"},
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "run-nvidia-validations", MountPath: "/run/nvidia/validations", MountPropagation: &[]corev1.MountPropagationMode{corev1.MountPropagationBidirectional}[0]},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:            "watchdog",
							Image:           h.cfg.ValidatorImage,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Command:         []string{"/bin/busybox", "sh", "-c", "echo all validations are successful; exec /bin/busybox sleep infinity"},
							SecurityContext: &corev1.SecurityContext{
								Privileged: ptr.To(true),
							},
							Env: []corev1.EnvVar{
								{Name: "PATH", Value: "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"},
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "run-nvidia-validations", MountPath: "/run/nvidia/validations", MountPropagation: &[]corev1.MountPropagationMode{corev1.MountPropagationBidirectional}[0]},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "run-nvidia-validations",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/run/nvidia/validations",
									Type: hostPathType(corev1.HostPathDirectoryOrCreate),
								},
							},
						},
					},
				},
			},
		},
	}
}

func (h *RendererHandler) migManagerConfigMap(pool *v1alpha1.GPUPool) *corev1.ConfigMap {
	devices := h.buildMIGDevices(pool)
	if len(devices) == 0 {
		devices = []map[string]any{{
			"pciBusId":   "all",
			"migEnabled": true,
			"migDevices": []map[string]any{{"profile": pool.Spec.Resource.MIGProfile}},
		}}
	}

	cfg := map[string]any{
		"version": 1,
		"mig-configs": []map[string]any{
			{
				"name":    "default",
				"devices": devices,
			},
		},
	}

	data, _ := yaml.Marshal(cfg)
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("nvidia-mig-manager-%s-config", pool.Name),
			Namespace: h.cfg.Namespace,
			Labels: map[string]string{
				"app":  "nvidia-mig-manager",
				"pool": pool.Name,
			},
		},
		Data: map[string]string{"config.yaml": string(data)},
	}
}

func (h *RendererHandler) migManagerScriptsConfigMap(pool *v1alpha1.GPUPool) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("nvidia-mig-manager-%s-scripts", pool.Name),
			Namespace: h.cfg.Namespace,
			Labels: map[string]string{
				"app":  "nvidia-mig-manager",
				"pool": pool.Name,
			},
		},
		Data: map[string]string{
			"reconfigure-mig.sh": migReconfigureScript,
			"prestop.sh":         migPrestopScript,
		},
	}
}

func (h *RendererHandler) migManagerClientsConfigMap(pool *v1alpha1.GPUPool) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("nvidia-mig-manager-%s-gpu-clients", pool.Name),
			Namespace: h.cfg.Namespace,
			Labels: map[string]string{
				"app":  "nvidia-mig-manager",
				"pool": pool.Name,
			},
		},
		Data: map[string]string{
			"clients.yaml": migGPUClients,
		},
	}
}

func (h *RendererHandler) migManagerDaemonSet(ctx context.Context, pool *v1alpha1.GPUPool) *appsv1.DaemonSet {
	poolKey := poolLabelKey(pool)
	cmName := fmt.Sprintf("nvidia-mig-manager-%s-config", pool.Name)
	clientsName := fmt.Sprintf("nvidia-mig-manager-%s-gpu-clients", pool.Name)
	scriptsName := fmt.Sprintf("nvidia-mig-manager-%s-scripts", pool.Name)

	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("nvidia-mig-manager-%s", pool.Name),
			Namespace: h.cfg.Namespace,
			Labels: map[string]string{
				"app":  "nvidia-mig-manager",
				"pool": pool.Name,
			},
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":  "nvidia-mig-manager",
					"pool": pool.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":    "nvidia-mig-manager",
						"pool":   pool.Name,
						"module": "gpu-control-plane",
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: "nvidia-mig-manager",
					HostPID:            true,
					HostNetwork:        true,
					DNSPolicy:          corev1.DNSClusterFirstWithHostNet,
					Tolerations: mergeTolerations([]corev1.Toleration{
						{Key: "node.kubernetes.io/unschedulable", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule},
						{Key: "mig-reconfigure", Operator: corev1.TolerationOpEqual, Value: "true", Effect: corev1.TaintEffectNoSchedule},
						{Key: poolKey, Operator: corev1.TolerationOpEqual, Value: pool.Name, Effect: corev1.TaintEffectNoSchedule},
					}, append(h.customTolerations, h.poolNodeTolerations(ctx, pool)...)),
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      poolKey,
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{pool.Name},
											},
										},
									},
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:            "mig-manager",
							Image:           h.cfg.MIGManagerImage,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Args:            []string{"-config-file=/mig-parted-config/config.yaml"},
							SecurityContext: &corev1.SecurityContext{Privileged: ptr.To(true)},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "host-root", MountPath: "/host"},
								{Name: "host-sys", MountPath: "/sys"},
								{Name: "gpu-clients", MountPath: "/gpu-clients"},
								{Name: "config", MountPath: "/mig-parted-config"},
								{Name: "dev", MountPath: "/dev"},
								{Name: "mig-scripts", MountPath: "/usr/bin/reconfigure-mig.sh", SubPath: "reconfigure-mig.sh"},
								{Name: "mig-scripts", MountPath: "/usr/bin/prestop.sh", SubPath: "prestop.sh"},
							},
							Env: []corev1.EnvVar{
								{Name: "NODE_NAME", ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "spec.nodeName"}}},
								{Name: "CONFIG_FILE", Value: "/mig-parted-config/config.yaml"},
								{Name: "GPU_CLIENTS_FILE", Value: "/gpu-clients/clients.yaml"},
								{Name: "HOST_ROOT_MOUNT", Value: "/host"},
								{Name: "HOST_NVIDIA_DIR", Value: "/usr/local/nvidia"},
								{Name: "HOST_KUBELET_SYSTEMD_SERVICE", Value: "kubelet.service"},
								{Name: "HOST_MIG_MANAGER_STATE_FILE", Value: "/etc/systemd/system/nvidia-mig-manager.service.d/override.conf"},
								{Name: "DEFAULT_GPU_CLIENTS_NAMESPACE", Value: h.cfg.Namespace},
								{Name: "WITH_SHUTDOWN_HOST_GPU_CLIENTS", Value: "true"},
								{Name: "WITH_REBOOT", Value: "false"},
							},
							Lifecycle: &corev1.Lifecycle{
								PreStop: &corev1.LifecycleHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"bash", "-c", "/usr/bin/prestop.sh"},
									},
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{Name: "host-root", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/", Type: hostPathType(corev1.HostPathDirectory)}}},
						{Name: "host-sys", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/sys", Type: hostPathType(corev1.HostPathDirectory)}}},
						{Name: "dev", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/dev", Type: hostPathType(corev1.HostPathDirectory)}}},
						{Name: "config", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: cmName}}}},
						{Name: "gpu-clients", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: clientsName}}}},
						{Name: "mig-scripts", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: scriptsName}, DefaultMode: ptr.To[int32](0o755)}}},
					},
				},
			},
		},
	}
}

func (h *RendererHandler) buildMIGDevices(pool *v1alpha1.GPUPool) []map[string]any {
	var devices []map[string]any

	for _, layout := range pool.Spec.Resource.MIGLayout {
		profiles := layout.Profiles
		if len(profiles) == 0 && pool.Spec.Resource.MIGProfile != "" {
			profiles = []v1alpha1.GPUPoolMIGProfile{{Name: pool.Spec.Resource.MIGProfile}}
		}
		if len(profiles) == 0 {
			continue
		}

		profileList := make([]map[string]any, 0, len(profiles))
		for _, p := range profiles {
			entry := map[string]any{"profile": p.Name}
			if p.Count != nil && *p.Count > 0 {
				entry["count"] = *p.Count
			}
			profileList = append(profileList, entry)
		}

		targets := 0
		addDevice := func(dev map[string]any) {
			dev["migEnabled"] = true
			dev["migDevices"] = profileList
			devices = append(devices, dev)
			targets++
		}

		for _, uuid := range layout.UUIDs {
			addDevice(map[string]any{"uuid": uuid})
		}
		for _, bus := range layout.PCIBusIDs {
			addDevice(map[string]any{"pciBusId": bus})
		}
		if len(layout.DeviceFilter) > 0 {
			addDevice(map[string]any{"deviceFilter": layout.DeviceFilter})
		}
		if targets == 0 {
			addDevice(map[string]any{"pciBusId": "all"})
		}
	}

	return devices
}

func buildCustomTolerations(keys []string) []corev1.Toleration {
	if len(keys) == 0 {
		return nil
	}
	out := make([]corev1.Toleration, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		if k == "" {
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, corev1.Toleration{
			Key:      k,
			Operator: corev1.TolerationOpExists,
		})
	}
	return out
}

func mergeTolerations(base []corev1.Toleration, extra []corev1.Toleration) []corev1.Toleration {
	if len(extra) == 0 {
		return base
	}
	out := make([]corev1.Toleration, 0, len(base)+len(extra))
	seen := make(map[string]struct{}, len(base)+len(extra))
	for _, t := range base {
		out = append(out, t)
		seen[t.Key+string(t.Operator)+t.Value+string(t.Effect)] = struct{}{}
	}
	for _, t := range extra {
		key := t.Key + string(t.Operator) + t.Value + string(t.Effect)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, t)
	}
	return out
}

func (h *RendererHandler) createOrUpdate(ctx context.Context, obj client.Object, pool *v1alpha1.GPUPool) error {
	desired := obj.DeepCopyObject().(client.Object)

	switch want := desired.(type) {
	case *corev1.ConfigMap:
		current := &corev1.ConfigMap{}
		if err := h.client.Get(ctx, client.ObjectKeyFromObject(want), current); err != nil {
			if apierrors.IsNotFound(err) {
				addOwner(want, pool)
				return h.client.Create(ctx, want)
			}
			return err
		}
		hadOwner := hasOwner(current, pool)
		addOwner(want, pool)
		addOwner(current, pool)
		if hadOwner && h.configMapEqual(current, want) {
			return nil
		}
		current.Labels = want.Labels
		current.Annotations = want.Annotations
		current.Data = want.Data
		current.BinaryData = want.BinaryData
		return h.client.Update(ctx, current)
	case *appsv1.DaemonSet:
		current := &appsv1.DaemonSet{}
		if err := h.client.Get(ctx, client.ObjectKeyFromObject(want), current); err != nil {
			if apierrors.IsNotFound(err) {
				addOwner(want, pool)
				return h.client.Create(ctx, want)
			}
			return err
		}
		hadOwner := hasOwner(current, pool)
		addOwner(want, pool)
		addOwner(current, pool)
		if hadOwner && h.daemonSetEqual(current, want) {
			return nil
		}
		current.Labels = want.Labels
		current.Annotations = want.Annotations
		current.Spec = want.Spec
		return h.client.Update(ctx, current)
	default:
		return fmt.Errorf("unsupported object type %T", obj)
	}
}

func addOwner(obj client.Object, pool *v1alpha1.GPUPool) {
	// Namespaced GPUPool cannot own resources in a different namespace; rely on explicit cleanup for those.
	if pool.Namespace != "" && obj.GetNamespace() != pool.Namespace {
		return
	}

	kind := pool.Kind
	if kind == "" {
		if pool.Namespace == "" {
			kind = "ClusterGPUPool"
		} else {
			kind = "GPUPool"
		}
	}
	owner := metav1.OwnerReference{
		APIVersion: v1alpha1.GroupVersion.String(),
		Kind:       kind,
		Name:       pool.Name,
		UID:        pool.UID,
		Controller: ptr.To(true),
	}
	refs := obj.GetOwnerReferences()
	for _, ref := range refs {
		if ref.APIVersion == owner.APIVersion && ref.Kind == owner.Kind && ref.Name == owner.Name {
			return
		}
	}
	obj.SetOwnerReferences(append(refs, owner))
}

func hasOwner(obj client.Object, pool *v1alpha1.GPUPool) bool {
	kind := pool.Kind
	if kind == "" {
		if pool.Namespace == "" {
			kind = "ClusterGPUPool"
		} else {
			kind = "GPUPool"
		}
	}
	for _, ref := range obj.GetOwnerReferences() {
		if ref.APIVersion == v1alpha1.GroupVersion.String() && ref.Kind == kind && ref.Name == pool.Name {
			return true
		}
	}
	return false
}

// poolNodeTolerations adds Exists-tolerations for taints present on nodes referenced by the pool status.
func (h *RendererHandler) poolNodeTolerations(ctx context.Context, pool *v1alpha1.GPUPool) []corev1.Toleration {
	if h.client == nil {
		return nil
	}
	tolerations := make([]corev1.Toleration, 0)
	seen := make(map[string]struct{})
	for _, n := range pool.Status.Nodes {
		node := &corev1.Node{}
		if err := h.client.Get(ctx, client.ObjectKey{Name: n.Name}, node); err != nil {
			continue
		}
		for _, t := range node.Spec.Taints {
			key := t.Key + string(t.Effect)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			tolerations = append(tolerations, corev1.Toleration{
				Key:      t.Key,
				Operator: corev1.TolerationOpExists,
				Effect:   t.Effect,
			})
		}
	}
	return tolerations
}

func hostPathType(t corev1.HostPathType) *corev1.HostPathType {
	return &t
}

func (h *RendererHandler) configMapEqual(current, desired *corev1.ConfigMap) bool {
	return apiequality.Semantic.DeepEqual(current.Labels, desired.Labels) &&
		apiequality.Semantic.DeepEqual(current.Annotations, desired.Annotations) &&
		apiequality.Semantic.DeepEqual(current.Data, desired.Data) &&
		apiequality.Semantic.DeepEqual(current.BinaryData, desired.BinaryData) &&
		apiequality.Semantic.DeepEqual(current.OwnerReferences, desired.OwnerReferences)
}

func (h *RendererHandler) daemonSetEqual(current, desired *appsv1.DaemonSet) bool {
	return apiequality.Semantic.DeepEqual(current.Labels, desired.Labels) &&
		apiequality.Semantic.DeepEqual(current.Annotations, desired.Annotations) &&
		apiequality.Semantic.DeepEqual(current.Spec, desired.Spec) &&
		apiequality.Semantic.DeepEqual(current.OwnerReferences, desired.OwnerReferences)
}

// cleanupPoolResources removes per-pool workloads when backend/provider changes.
func (h *RendererHandler) cleanupPoolResources(ctx context.Context, poolName string) error {
	ds := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{
		Name:      fmt.Sprintf("nvidia-device-plugin-%s", poolName),
		Namespace: h.cfg.Namespace,
	}}
	if err := h.client.Delete(ctx, ds); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
		Name:      fmt.Sprintf("nvidia-device-plugin-%s-config", poolName),
		Namespace: h.cfg.Namespace,
	}}
	if err := h.client.Delete(ctx, cm); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if err := h.cleanupMIGResources(ctx, poolName); err != nil {
		return err
	}
	validator := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{
		Name:      fmt.Sprintf("nvidia-operator-validator-%s", poolName),
		Namespace: h.cfg.Namespace,
	}}
	if err := h.client.Delete(ctx, validator); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

func (h *RendererHandler) cleanupMIGResources(ctx context.Context, poolName string) error {
	ds := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{
		Name:      fmt.Sprintf("nvidia-mig-manager-%s", poolName),
		Namespace: h.cfg.Namespace,
	}}
	if err := h.client.Delete(ctx, ds); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	for _, name := range []string{
		fmt.Sprintf("nvidia-mig-manager-%s-config", poolName),
		fmt.Sprintf("nvidia-mig-manager-%s-scripts", poolName),
		fmt.Sprintf("nvidia-mig-manager-%s-gpu-clients", poolName),
	} {
		cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: h.cfg.Namespace,
		}}
		if err := h.client.Delete(ctx, cm); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}
