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

package validator

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	poolsvc "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/renderer/deps"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/renderer/kube"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/renderer/names"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/renderer/tolerations"
)

func validatorDaemonSet(ctx context.Context, d deps.Deps, pool *v1alpha1.GPUPool) *appsv1.DaemonSet {
	poolKey := poolsvc.PoolLabelKey(pool)
	mergedTolerations := tolerations.Merge([]corev1.Toleration{
		{
			Effect:   corev1.TaintEffectNoSchedule,
			Key:      poolKey,
			Operator: corev1.TolerationOpEqual,
			Value:    pool.Name,
		},
	}, append(d.CustomTolerations, tolerations.PoolNodeTolerations(ctx, d.Client, pool)...))
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("nvidia-operator-validator-%s", pool.Name),
			Namespace: d.Config.Namespace,
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
					InitContainers: validatorInitContainers(d, pool),
					Containers: []corev1.Container{
						{
							Name:            "watchdog",
							Image:           d.Config.ValidatorImage,
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
									Type: kube.HostPathType(corev1.HostPathDirectoryOrCreate),
								},
							},
						},
						{
							Name: "kubelet-device-plugins",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/var/lib/kubelet/device-plugins",
									Type: kube.HostPathType(corev1.HostPathDirectory),
								},
							},
						},
						{
							Name: "kubelet-pod-resources",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/var/lib/kubelet/pod-resources",
									Type: kube.HostPathType(corev1.HostPathDirectory),
								},
							},
						},
					},
				},
			},
		},
	}
}

// validatorInitContainers always injects plugin-validation; resource name is passed explicitly so validator can see custom resources.
func validatorInitContainers(d deps.Deps, pool *v1alpha1.GPUPool) []corev1.Container {
	return []corev1.Container{
		{
			Name:            "plugin-validation",
			Image:           d.Config.ValidatorImage,
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
				{Name: "NVIDIA_RESOURCE_NAME", Value: names.PoolResourceName(pool)},
				{Name: "MIG_STRATEGY", Value: d.Config.DefaultMIGStrategy},
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
				{Name: "VALIDATOR_IMAGE", Value: d.Config.ValidatorImage},
				{Name: "VALIDATOR_IMAGE_PULL_POLICY", Value: "IfNotPresent"},
				{Name: "VALIDATOR_RUNTIME_CLASS", Value: "nvidia"},
			},
			VolumeMounts: []corev1.VolumeMount{
				{Name: "run-nvidia-validations", MountPath: "/run/nvidia/validations", MountPropagation: &[]corev1.MountPropagationMode{corev1.MountPropagationBidirectional}[0]},
				{Name: "kubelet-device-plugins", MountPath: "/var/lib/kubelet/device-plugins", ReadOnly: true},
				{Name: "kubelet-pod-resources", MountPath: "/var/lib/kubelet/pod-resources", ReadOnly: true},
			},
		},
	}
}
