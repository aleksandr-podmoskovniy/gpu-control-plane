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

# Helm version handling (download if local binary is too old).
HELM_BIN=""
HELM_TMPDIR=""
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
  trap 'rm -rf "${HELM_TMPDIR}"' EXIT
  curl -sSL "https://get.helm.sh/helm-v${desired_version}-${os}-${arch}.tar.gz" -o "${HELM_TMPDIR}/helm.tar.gz"
  tar -xzf "${HELM_TMPDIR}/helm.tar.gz" -C "${HELM_TMPDIR}"
  HELM_BIN="${HELM_TMPDIR}/${os}-${arch}/helm"
  chmod +x "${HELM_BIN}"
}

ensure_helm

if [[ -z "${HELM_BIN}" ]]; then
  echo "Error: Helm binary was not configured" >&2
  exit 1
fi

echo "Using helm binary: ${HELM_BIN}" >&2
if ! command -v jq >/dev/null 2>&1; then
  echo "Error: jq binary not found in PATH" >&2
  exit 1
fi

KUBECONFORM_IMAGE=ghcr.io/yannh/kubeconform:v0.7.0
USE_DOCKER=false

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
  echo "Error: install kubeconform or Docker to run this script" >&2
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
  curl -sLo podmonitors.yaml https://raw.githubusercontent.com/deckhouse/deckhouse/main/modules/200-operator-prometheus/crds/podmonitors.yaml
  curl -sLo scrapeconfigs.yaml https://raw.githubusercontent.com/deckhouse/deckhouse/main/modules/200-operator-prometheus/crds/scrapeconfigs.yaml
  curl -sLo prometheusrules.yaml https://raw.githubusercontent.com/deckhouse/deckhouse/main/modules/200-operator-prometheus/crds/internal/prometheusrules.yaml
  curl -sLo verticalpodautoscalers.yaml https://raw.githubusercontent.com/deckhouse/deckhouse/main/modules/302-vertical-pod-autoscaler/crds/verticalpodautoscaler.yaml
  curl -sLo operation-policy.yaml https://raw.githubusercontent.com/deckhouse/deckhouse/main/modules/015-admission-policy-engine/crds/operation-policy.yaml
  curl -sLo security-policy.yaml https://raw.githubusercontent.com/deckhouse/deckhouse/main/modules/015-admission-policy-engine/crds/security-policy.yaml
  curl -sLo nodegroupconfiguration.yaml https://raw.githubusercontent.com/deckhouse/deckhouse/main/modules/040-node-manager/crds/nodegroupconfiguration.yaml
  curl -sLo certificates.yaml https://raw.githubusercontent.com/deckhouse/deckhouse/main/modules/101-cert-manager/crds/crd-certificates.yaml
  curl -sLo grafanadashboarddefinition.yaml https://raw.githubusercontent.com/deckhouse/deckhouse/main/modules/300-prometheus/crds/grafanadashboarddefinition.yaml
  curl -sLo clusterlogdestination.yaml https://raw.githubusercontent.com/deckhouse/deckhouse/main/modules/460-log-shipper/crds/cluster-log-destination.yaml
  curl -sLo clusterloggingconfig.yaml https://raw.githubusercontent.com/deckhouse/deckhouse/main/modules/460-log-shipper/crds/cluster-logging-config.yaml
  curl -sLo deschedulers.yaml https://raw.githubusercontent.com/deckhouse/deckhouse/main/modules/400-descheduler/crds/deschedulers.yaml

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

  # Relax metadata requirements for Deckhouse descheduler CRDs (matches virtualization tooling)
  find . -iname "descheduler-deckhouse-*.json" | while read -r schema; do
    jq '(.properties.metadata) |= {type: "object"}' "$schema" > tmp.json && mv tmp.json "$schema"
  done

  popd >/dev/null
fi

HELM_RENDER=helm-template-render.yaml
"${HELM_BIN}" template gpu-control-plane ../.. -f ../../fixtures/module-values.yaml --devel > "${HELM_RENDER}"

cat "${HELM_RENDER}" | _kubeconform -verbose -strict \
  -kubernetes-version 1.30.0 \
  -schema-location default \
  -schema-location 'schemas/{{ .ResourceKind }}{{ .KindSuffix }}.json' \
  -output json - > kubeconform-report.json

if ! jq type kubeconform-report.json >/dev/null 2>&1; then
  echo "Error: kubeconform-report.json is not valid JSON" >&2
  cat kubeconform-report.json
  exit 1
fi

cat kubeconform-report.json | jq -r '
def indent(n): split("\n")|map((" "*n)+.)|join("\n");
def noSchemaError(msg): msg|test("could not find schema for");
def regularError(msg): msg|test("could not find schema for")|not;

(.resources|sort_by(.kind,.name)) as $r |
[$r[]|select(.status=="statusValid")]      as $valid |
[$r[]|select(.status=="statusSkipped")]    as $skipped |
[$r[]|select(.status=="statusError" and noSchemaError(.msg))] as $noSchema |
[$r[]|select(.status=="statusError" and regularError(.msg))]  as $errors |
[$r[]|select(.status=="statusInvalid")]    as $invalid |

($valid   | map("VALID     "+.kind+" "+.name)) as $validRows |
($skipped | map("SKIPPED   "+.kind+" "+.name)) as $skippedRows |
($noSchema| map("NO SCHEMA "+.kind+" "+.name)) as $noSchemaRows |
($errors  | map("ERROR     "+.kind+" "+.name, (.msg|indent(2)))) as $errorRows |
($invalid | map("INVALID   "+.kind+" "+.name, (.validationErrors|map("- "+.path+":"|indent(2), (.msg|indent(6)))))) as $invalidRows |

[
  "--- Kubeconform report ---",
  $validRows, $skippedRows, $noSchemaRows, $errorRows, $invalidRows,
  "------- Summary -------",
  "      valid: "+($valid|length|tostring),
  "    skipped: "+($skipped|length|tostring),
  "  no schema: "+($noSchema|length|tostring),
  "     errors: "+($errors|length|tostring),
  "    invalid: "+($invalid|length|tostring)
] | flatten | join("\n")
'

exit_code=$(jq -r '[.resources[] | select(.status=="statusError" or .status=="statusInvalid")] | length' kubeconform-report.json)
exit "${exit_code}"
