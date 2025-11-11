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
ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
FIXTURES_DIR="${ROOT_DIR}/fixtures/bootstrap-states"
VALUES_BASE="${ROOT_DIR}/fixtures/module-values.yaml"
HELM_BIN=""
HELM_TMPDIR=""
cleanup() {
  if [[ -n "${HELM_TMPDIR}" && -d "${HELM_TMPDIR}" ]]; then
    rm -rf "${HELM_TMPDIR}"
  fi
}
trap cleanup EXIT

ensure_helm() {
  local min_version="3.14.0"
  local desired_version="${HELM_DESIRED_VERSION:-3.17.2}"

  version_ge() {
    [ "$(printf '%s\n' "$2" "$1" | sort -V | head -n1)" = "$2" ]
  }

  if command -v helm >/dev/null 2>&1; then
    local current
    current="$(helm version --template '{{.Version}}' 2>/dev/null | sed 's/^v//')"
    if [ -n "$current" ] && version_ge "$current" "$min_version"; then
      HELM_BIN="$(command -v helm)"
      return
    fi
  fi

  local os arch
  case "$(uname -s)" in
    Linux) os="linux" ;;
    Darwin) os="darwin" ;;
    *)
      echo "Error: unsupported OS $(uname -s). Install Helm >= ${min_version} and re-run." >&2
      exit 1
      ;;
  esac

  case "$(uname -m)" in
    x86_64|amd64) arch="amd64" ;;
    arm64|aarch64) arch="arm64" ;;
    *)
      echo "Error: unsupported arch $(uname -m). Install Helm >= ${min_version} and re-run." >&2
      exit 1
      ;;
  esac

  echo "Downloading Helm v${desired_version} (${os}/${arch}) ..." >&2
  HELM_TMPDIR=$(mktemp -d "${TMPDIR:-/tmp}/helm.XXXXXX")
  curl -sSL "https://get.helm.sh/helm-v${desired_version}-${os}-${arch}.tar.gz" -o "${HELM_TMPDIR}/helm.tar.gz"
  tar -xzf "${HELM_TMPDIR}/helm.tar.gz" -C "${HELM_TMPDIR}"
  HELM_BIN="${HELM_TMPDIR}/${os}-${arch}/helm"
  chmod +x "${HELM_BIN}"
}

render_chart() {
  "${HELM_BIN}" template "$@"
}

ensure_helm
for state in "${FIXTURES_DIR}"/*.yaml; do
  [ -e "$state" ] || continue
  echo "-- helm template with ${state##*/}"
  render_chart gpu-control-plane "${ROOT_DIR}" \
    -f "${VALUES_BASE}" \
    -f "$state" \
    --set global.enabledModules={gpu-control-plane} \
    --set global.deckhouseVersion="dev" \
    --set global.discovery.clusterDomain="cluster.local" \
    --set global.internal.modules.gpuControlPlane=true \
    >/dev/null
  echo "   ok"
done

STRING_OVERRIDE_MATRIX=(
  "dcgm|gpuControlPlane.bootstrap.dcgm"
  "dcgm-exporter|gpuControlPlane.bootstrap.dcgmExporter"
  "validator|gpuControlPlane.bootstrap.validator"
  "gpu-feature-discovery|gpuControlPlane.bootstrap.gfd"
)

for entry in "${STRING_OVERRIDE_MATRIX[@]}"; do
  component="${entry%%|*}"
  key="${entry##*|}"
  echo "-- helm template with string override for ${component}"
  render_chart gpu-control-plane "${ROOT_DIR}" \
    -f "${VALUES_BASE}" \
    --set global.enabledModules={gpu-control-plane} \
    --set global.deckhouseVersion="dev" \
    --set global.discovery.clusterDomain="cluster.local" \
    --set global.internal.modules.gpuControlPlane=true \
    --set-string "${key}=legacy" \
    >/dev/null
  echo "   ok"
done
