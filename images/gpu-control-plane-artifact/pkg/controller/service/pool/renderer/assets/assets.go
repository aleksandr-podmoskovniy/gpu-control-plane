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

package assets

import _ "embed"

// Embedded assets reused from MIG manager template to stay consistent with node-manager.

//go:embed reconfigure-mig.sh
var MIGReconfigureScript string

//go:embed prestop.sh
var MIGPrestopScript string

//go:embed gpu-clients.yaml
var MIGGPUClients string
