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

package deviceplugin

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	poolcommon "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/common"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/deps"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/kube"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/tolerations"
)

func devicePluginDaemonSet(ctx context.Context, d deps.Deps, pool *v1alpha1.GPUPool) *appsv1.DaemonSet {
	poolKey := poolcommon.PoolLabelKey(pool)
	mergedTolerations := tolerations.Merge([]corev1.Toleration{
		{
			Key:      poolKey,
			Operator: corev1.TolerationOpEqual,
			Value:    pool.Name,
			Effect:   corev1.TaintEffectNoSchedule,
		},
	}, append(d.CustomTolerations, tolerations.PoolNodeTolerations(ctx, d.Client, pool)...))
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("nvidia-device-plugin-%s", pool.Name),
			Namespace: d.Config.Namespace,
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
					Tolerations:        mergedTolerations,
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
							Image:           d.Config.DevicePluginImage,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Command:         []string{"/nvidia-device-plugin"},
							SecurityContext: &corev1.SecurityContext{
								Privileged:               ptr.To(true),
								RunAsUser:                ptr.To[int64](0),
								RunAsNonRoot:             ptr.To(false),
								AllowPrivilegeEscalation: ptr.To(true),
								ReadOnlyRootFilesystem:   ptr.To(false),
							},
							// pass-device-specs aligns with plugin config; device list/id strategies are set via ConfigMap.
							Args: []string{"--config-file=/config/config.yaml", "--pass-device-specs=true", "--fail-on-init-error=false"},
							Env: []corev1.EnvVar{
								{Name: "NVIDIA_VISIBLE_DEVICES", Value: "all"},
								{Name: "NVIDIA_RESOURCE_PREFIX", Value: poolcommon.PoolResourcePrefixFor(pool)},
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
									Type: kube.HostPathType(corev1.HostPathDirectory),
								},
							},
						},
						{
							Name: "dev",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/dev",
									Type: kube.HostPathType(corev1.HostPathDirectory),
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
