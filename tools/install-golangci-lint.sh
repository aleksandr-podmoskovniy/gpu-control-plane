#!/usr/bin/env bash
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
INSTALL_DIR=${INSTALL_DIR:-$(pwd)/.bin}
VERSION=${GOLANGCI_LINT_VERSION:-2.6.2}
BINARY="${INSTALL_DIR}/golangci-lint"

mkdir -p "$INSTALL_DIR"

if [[ -x "${BINARY}" ]]; then
  if current_version="$("${BINARY}" version --format short 2>/dev/null)"; then
    if [[ "${current_version}" == "v${VERSION}" ]]; then
      exit 0
    fi
  fi
fi

if [[ "${GOLANGCI_LINT_USE_GO_INSTALL:-}" == "1" ]]; then
  MODULE="github.com/golangci/golangci-lint/cmd/golangci-lint"
  if [[ "${VERSION}" == 2.* ]]; then
    MODULE="github.com/golangci/golangci-lint/v2/cmd/golangci-lint"
  fi

  if [[ -n "${GOLANGCI_LINT_TOOLCHAIN:-}" ]]; then
    GOTOOLCHAIN="${GOLANGCI_LINT_TOOLCHAIN}" GOBIN="${INSTALL_DIR}" go install "${MODULE}@v${VERSION}"
    exit 0
  fi

  GOBIN="${INSTALL_DIR}" go install "${MODULE}@v${VERSION}"
  exit 0
fi

curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b "${INSTALL_DIR}" "v${VERSION}"
