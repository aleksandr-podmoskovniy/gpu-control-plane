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

if [[ ! -d ../../templates ]]; then
  echo "Error: run this script from tools/kubeconform" >&2
  exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "Error: jq binary not found in PATH" >&2
  exit 1
fi

USE_DOCKER=false
KUBECONFORM_IMAGE=ghcr.io/yannh/kubeconform:latest

_kubeconform() {
  if [[ ${USE_DOCKER} == true ]]; then
    docker run --rm -i -v "$(pwd)":/workdir -w /workdir --entrypoint /kubeconform "${KUBECONFORM_IMAGE}" "$@"
  else
    kubeconform "$@"
  fi
}

if command -v kubeconform >/dev/null 2>&1; then
  echo "Use local kubeconform $(kubeconform -v)" >&2
elif command -v docker >/dev/null 2>&1; then
  echo "Use kubeconform via docker" >&2
  USE_DOCKER=true
else
  echo "Error: install kubeconform binary or Docker" >&2
  exit 1
fi

KUBECONFORM_REPO="kubeconform.git"
if [[ ! -d "${KUBECONFORM_REPO}" ]]; then
  echo "Clone kubeconform repository to convert schemas ..." >&2
  git clone https://github.com/yannh/kubeconform.git "${KUBECONFORM_REPO}" >/dev/null 2>&1
fi
OPENAPI2JSONSCHEMA="$(pwd)/${KUBECONFORM_REPO}/scripts/openapi2jsonschema.py"
if [[ ! -f "${OPENAPI2JSONSCHEMA}" ]]; then
  echo "Error: ${OPENAPI2JSONSCHEMA} not found" >&2
  exit 1
fi

# Helm limits packaged files to 5MiB by default which is not enough for some Deckhouse charts.
export HELM_MAX_FILE_SIZE="${HELM_MAX_FILE_SIZE:-52428800}"

if [[ ! -d schemas ]]; then
  mkdir -p schemas
  pushd schemas >/dev/null

  echo "Download Deckhouse CRDs ..." >&2
  curl -sLo servicemonitors.yaml https://raw.githubusercontent.com/deckhouse/deckhouse/main/modules/200-operator-prometheus/crds/servicemonitors.yaml
  curl -sLo podmonitors.yaml https://raw.githubusercontent.com/deckhouse/deckhouse/main/modules/200-operator-prometheus/crds/podmonitors.yaml
  curl -sLo scrapeconfigs.yaml https://raw.githubusercontent.com/deckhouse/deckhouse/main/modules/200-operator-prometheus/crds/scrapeconfigs.yaml
  curl -sLo prometheusrules.yaml https://raw.githubusercontent.com/deckhouse/deckhouse/main/modules/200-operator-prometheus/crds/internal/prometheusrules.yaml
  curl -sLo verticalpodautoscalers.yaml https://raw.githubusercontent.com/deckhouse/deckhouse/main/modules/302-vertical-pod-autoscaler/crds/verticalpodautoscaler.yaml
  curl -sLo nodegroupconfiguration.yaml https://raw.githubusercontent.com/deckhouse/deckhouse/main/modules/040-node-manager/crds/nodegroupconfiguration.yaml
  curl -sLo certificates.yaml https://raw.githubusercontent.com/deckhouse/deckhouse/main/modules/101-cert-manager/crds/crd-certificates.yaml
  curl -sLo grafanadashboarddefinition.yaml https://raw.githubusercontent.com/deckhouse/deckhouse/main/modules/300-prometheus/crds/grafanadashboarddefinition.yaml
  curl -sLo cluster-logging-config.yaml https://raw.githubusercontent.com/deckhouse/deckhouse/main/modules/460-log-shipper/crds/cluster-logging-config.yaml
  curl -sLo cluster-log-destination.yaml https://raw.githubusercontent.com/deckhouse/deckhouse/main/modules/460-log-shipper/crds/cluster-log-destination.yaml
  curl -sLo deschedulers.yaml https://raw.githubusercontent.com/deckhouse/deckhouse/main/modules/400-descheduler/crds/deschedulers.yaml

  echo "Transform Deckhouse CRDs to JSON schema ..." >&2
  export FILENAME_FORMAT='{kind}-{group}-{version}'
  for crd in *.yaml; do
    "${OPENAPI2JSONSCHEMA}" "$crd"
  done

  echo "Transform gpu-control-plane CRDs ..." >&2
  shopt -s nullglob
  for crd in ../../crds/*.yaml; do
    [[ "$crd" == *doc-ru* ]] && continue
    "${OPENAPI2JSONSCHEMA}" "$crd"
  done
  shopt -u nullglob

  # Relax metadata requirements for Deckhouse descheduler CRDs (matches virtualization tooling)
  find . -iname "descheduler-deckhouse-*.json" | while read -r schema; do
    jq '(.properties.metadata) |= {type: "object"}' "$schema" > tmp.json && mv tmp.json "$schema"
  done

  popd >/dev/null
fi

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
  trap 'rm -rf "${HELM_TMPDIR}" "${KUBECONFORM_REPO}"' EXIT
  curl -sSL "https://get.helm.sh/helm-v${desired_version}-${os}-${arch}.tar.gz" -o "${HELM_TMPDIR}/helm.tar.gz"
  tar -xzf "${HELM_TMPDIR}/helm.tar.gz" -C "${HELM_TMPDIR}"
  HELM_BIN="${HELM_TMPDIR}/${os}-${arch}/helm"
  chmod +x "${HELM_BIN}"
}

HELM_RENDER=helm-template-render.yaml
HELM_BIN=""
HELM_TMPDIR=""

render_with_helm() {
  "${HELM_BIN}" template gpu-control-plane ../.. -f ../../fixtures/module-values.yaml --devel
}

render_with_werf() {
  WERF_DEV=1 werf helm template gpu-control-plane ../.. -f ../../fixtures/module-values.yaml --devel \
    | awk 'BEGIN{emit=0} { if (!emit && ($0 ~ /^#/ || $0 ~ /^apiVersion:/)) emit=1 } emit { print }'
}

ensure_helm

if render_with_helm > "${HELM_RENDER}" 2>/tmp/kubeconform.render.log; then
  :
elif command -v werf >/dev/null 2>&1; then
  echo "helm template failed; falling back to werf helm template" >&2
  render_with_werf > "${HELM_RENDER}"
else
  echo "helm template failed; install werf or fix helm rendering" >&2
  cat /tmp/kubeconform.render.log >&2 || true
  exit 1
fi

_kubeconform -verbose -strict \
  -kubernetes-version 1.30.0 \
  -schema-location default \
  -schema-location 'schemas/{{ .ResourceKind }}{{ .KindSuffix }}.json' \
  -summary -output json "${HELM_RENDER}" > kubeconform-report.json || true

if ! jq type kubeconform-report.json >/dev/null 2>&1; then
  echo "Error: kubeconform-report.json is not valid JSON" >&2
  cat kubeconform-report.json
  exit 1
fi

echo "--- Kubeconform report ---"
jq -r '(.resources // [])[] | select(.status != "") | "\(.status)\t\(.kind) \(.name)"' kubeconform-report.json

errors=$(jq -r '[.resources[] | select(.status != "" and .status != "statusValid")] | length' kubeconform-report.json)
if [[ ${errors} -ne 0 ]]; then
  echo "Detailed report saved to tools/kubeconform/kubeconform-report.json" >&2
  exit 1
fi
