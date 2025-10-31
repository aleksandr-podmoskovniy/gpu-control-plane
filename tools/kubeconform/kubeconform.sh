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

if ! command -v helm >/dev/null 2>&1; then
  echo "Error: helm v3 binary not found in PATH" >&2
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

if [[ ! -d kubeconform.git ]]; then
  echo "Clone kubeconform repository to convert schemas ..." >&2
  git clone https://github.com/yannh/kubeconform.git kubeconform.git >/dev/null 2>&1
fi

if [[ ! -d schemas ]]; then
  mkdir -p schemas
  pushd schemas >/dev/null

  echo "Download Deckhouse CRDs ..." >&2
  curl -sLo servicemonitors.yaml https://raw.githubusercontent.com/deckhouse/deckhouse/main/modules/200-operator-prometheus/crds/servicemonitors.yaml
  curl -sLo prometheusrules.yaml https://raw.githubusercontent.com/deckhouse/deckhouse/main/modules/200-operator-prometheus/crds/internal/prometheusrules.yaml

  echo "Transform Deckhouse CRDs to JSON schema ..." >&2
  export FILENAME_FORMAT='{kind}-{group}-{version}'
  for crd in *.yaml; do
    ../kubeconform.git/scripts/openapi2jsonschema.py "$crd"
  done

  echo "Transform gpu-control-plane CRDs ..." >&2
  shopt -s nullglob
  for crd in ../../crds/*.yaml; do
    [[ "$crd" == *doc-ru* ]] && continue
    ../kubeconform.git/scripts/openapi2jsonschema.py "$crd"
  done
  shopt -u nullglob

  popd >/dev/null
fi

HELM_RENDER=helm-template-render.yaml
WERF_DEV=1 werf helm template gpu-control-plane ../.. -f ../../fixtures/module-values.yaml > "${HELM_RENDER}"

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
jq -r '(.resources // [])[] | "\(.status)\t\(.kind) \(.name)"' kubeconform-report.json

errors=$(jq -r '[.resources[] | select(.status != "statusValid")] | length' kubeconform-report.json)
if [[ ${errors} -ne 0 ]]; then
  echo "Detailed report saved to tools/kubeconform/kubeconform-report.json" >&2
  exit 1
fi
