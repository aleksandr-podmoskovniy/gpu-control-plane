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

func (r *Runner) emitHints(result ProbeResult, attempt int) {
	if attempt%r.hintEvery != 0 {
		return
	}

	r.logf("\n")
	r.emitCommonErr()

	if r.driverRoot != "/" {
		if result.DriverRootEmpty {
			r.logf("Hint: Directory %s on the host is empty\n", r.driverRoot)
		} else if result.NvidiaSMIPath == "" || result.NVMLLibPath == "" {
			r.logf("Hint: Directory %s is not empty but at least one of the binaries wasn't found.\n", r.driverRoot)
		}
	}

	if r.driverRoot == "/" && result.OperatorSMIDetected {
		r.logf("Hint: '/run/nvidia/driver/usr/bin/nvidia-smi' exists on the host, you may want to re-install the DRA driver Helm chart with --set nvidiaDriverRoot=/run/nvidia/driver\n")
	}

	if r.driverRoot == "/run/nvidia/driver" {
		r.logf("Hint: NVIDIA_DRIVER_ROOT is set to '/run/nvidia/driver' which typically means that the NVIDIA GPU Operator manages the GPU driver. Make sure that the GPU Operator is deployed and healthy.\n")
	}

	r.logf("\n")
}

func (r *Runner) emitCommonErr() {
	r.logf("Check failed. Has the NVIDIA GPU driver been set up? It is expected to be installed under NVIDIA_DRIVER_ROOT (currently set to '%s') in the host filesystem. If that path appears to be unexpected: review the DRA driver's 'nvidiaDriverRoot' Helm chart variable. Otherwise, review if the GPU driver has actually been installed under that path.\n", r.driverRoot)
}
