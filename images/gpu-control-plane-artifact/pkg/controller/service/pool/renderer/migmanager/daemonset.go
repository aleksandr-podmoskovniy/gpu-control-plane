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

package migmanager

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
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/renderer/tolerations"
)

func migManagerDaemonSet(ctx context.Context, d deps.Deps, pool *v1alpha1.GPUPool) *appsv1.DaemonSet {
	poolKey := poolsvc.PoolLabelKey(pool)
	cmName := fmt.Sprintf("nvidia-mig-manager-%s-config", pool.Name)
	clientsName := fmt.Sprintf("nvidia-mig-manager-%s-gpu-clients", pool.Name)
	scriptsName := fmt.Sprintf("nvidia-mig-manager-%s-scripts", pool.Name)

	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("nvidia-mig-manager-%s", pool.Name),
			Namespace: d.Config.Namespace,
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
					Tolerations: tolerations.Merge([]corev1.Toleration{
						{Key: "node.kubernetes.io/unschedulable", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule},
						{Key: "mig-reconfigure", Operator: corev1.TolerationOpEqual, Value: "true", Effect: corev1.TaintEffectNoSchedule},
						{Key: poolKey, Operator: corev1.TolerationOpEqual, Value: pool.Name, Effect: corev1.TaintEffectNoSchedule},
					}, append(d.CustomTolerations, tolerations.PoolNodeTolerations(ctx, d.Client, pool)...)),
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
							Image:           d.Config.MIGManagerImage,
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
								{Name: "DEFAULT_GPU_CLIENTS_NAMESPACE", Value: d.Config.Namespace},
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
						{Name: "host-root", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/", Type: kube.HostPathType(corev1.HostPathDirectory)}}},
						{Name: "host-sys", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/sys", Type: kube.HostPathType(corev1.HostPathDirectory)}}},
						{Name: "dev", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/dev", Type: kube.HostPathType(corev1.HostPathDirectory)}}},
						{Name: "config", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: cmName}}}},
						{Name: "gpu-clients", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: clientsName}}}},
						{Name: "mig-scripts", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: scriptsName}, DefaultMode: ptr.To[int32](0o755)}}},
					},
				},
			},
		},
	}
}
