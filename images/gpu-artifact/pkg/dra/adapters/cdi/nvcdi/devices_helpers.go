//go:build linux && cgo && nvml
// +build linux,cgo,nvml

/*
Copyright 2025 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package nvcdi

import (
	"strings"

	cdispec "tags.cncf.io/container-device-interface/specs-go"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
)

func attrString(attrs map[string]allocatable.AttributeValue, key string) string {
	if attrs == nil {
		return ""
	}
	val, ok := attrs[key]
	if !ok || val.String == nil {
		return ""
	}
	return strings.TrimSpace(*val.String)
}

func applyMpsEdits(edits *cdispec.ContainerEdits, attrs map[string]allocatable.AttributeValue) {
	if edits == nil {
		return
	}
	pipeDir := attrString(attrs, allocatable.AttrMpsPipeDir)
	shmDir := attrString(attrs, allocatable.AttrMpsShmDir)
	logDir := attrString(attrs, allocatable.AttrMpsLogDir)
	if pipeDir == "" && shmDir == "" && logDir == "" {
		return
	}

	if pipeDir != "" {
		appendEnv(edits, "CUDA_MPS_PIPE_DIRECTORY=/tmp/nvidia-mps")
		edits.Mounts = append(edits.Mounts, &cdispec.Mount{
			HostPath:      pipeDir,
			ContainerPath: "/tmp/nvidia-mps",
		})
	}
	if logDir != "" {
		appendEnv(edits, "CUDA_MPS_LOG_DIRECTORY=/var/log/nvidia-mps")
		edits.Mounts = append(edits.Mounts, &cdispec.Mount{
			HostPath:      logDir,
			ContainerPath: "/var/log/nvidia-mps",
		})
	}
	if shmDir != "" {
		edits.Mounts = append(edits.Mounts, &cdispec.Mount{
			HostPath:      shmDir,
			ContainerPath: "/dev/shm",
		})
	}
}

func appendEnv(edits *cdispec.ContainerEdits, entry string) {
	for _, existing := range edits.Env {
		if existing == entry {
			return
		}
	}
	edits.Env = append(edits.Env, entry)
}
