#!/bin/bash

# Copyright 2025 Flant JSC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -euo pipefail
ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
FIXTURES_DIR="${ROOT_DIR}/fixtures/bootstrap-states"
VALUES_BASE="${ROOT_DIR}/fixtures/module-values.yaml"
HELM_CMD="helm"
if ! command -v helm >/dev/null 2>&1; then
  echo "Error: helm binary not found" >&2
  exit 1
fi
for state in "${FIXTURES_DIR}"/*.yaml; do
  [ -e "$state" ] || continue
  echo "-- helm template with ${state##*/}"
  ${HELM_CMD} template gpu-control-plane "${ROOT_DIR}" \
    -f "${VALUES_BASE}" \
    -f "$state" \
    --set global.enabledModules={gpu-control-plane} \
    --set global.deckhouseVersion="dev" \
    --set global.discovery.clusterDomain="cluster.local" \
    --set global.internal.modules.gpuControlPlane=true \
    >/dev/null
  echo "   ok"
done
