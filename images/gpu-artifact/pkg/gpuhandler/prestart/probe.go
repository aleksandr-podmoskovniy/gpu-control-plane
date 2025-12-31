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

package prestart

import (
	"path/filepath"
	"sort"
	"strings"
)

// ProbeResult describes a single probe attempt.
type ProbeResult struct {
	NvidiaSMIPath       string
	NVMLLibPath         string
	DriverRootContents  string
	DriverRootEmpty     bool
	OperatorSMIDetected bool
}

// CanInvoke reports whether nvidia-smi can be executed.
func (r ProbeResult) CanInvoke() bool {
	return r.NvidiaSMIPath != "" && r.NVMLLibPath != ""
}

func (r *Runner) probe() ProbeResult {
	nvidiaSMIPath := r.findFirstFile(nvidiaSMIRelDirs, "nvidia-smi")
	nvmlLibPath := r.findFirstFile(nvmlLibRelDirs, "libnvidia-ml.so.1")
	contents, empty := r.listDriverRoot()

	operatorSMI := r.isRegularFile(filepath.Join(r.driverRootMount, "run/nvidia/driver/usr/bin/nvidia-smi"))

	return ProbeResult{
		NvidiaSMIPath:       nvidiaSMIPath,
		NVMLLibPath:         nvmlLibPath,
		DriverRootContents:  contents,
		DriverRootEmpty:     empty,
		OperatorSMIDetected: operatorSMI,
	}
}

func (r *Runner) findFirstFile(relDirs []string, name string) string {
	for _, relDir := range relDirs {
		path := filepath.Join(r.driverRootMount, relDir, name)
		if r.isRegularFile(path) {
			return path
		}
	}
	return ""
}

func (r *Runner) isRegularFile(path string) bool {
	info, err := r.fs.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular()
}

func (r *Runner) listDriverRoot() (string, bool) {
	entries, err := r.fs.ReadDir(r.driverRootMount)
	if err != nil {
		return "", true
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if name == "." || name == ".." {
			continue
		}
		names = append(names, name)
	}

	if len(names) == 0 {
		return "", true
	}

	sort.Strings(names)
	return strings.Join(names, " "), false
}
