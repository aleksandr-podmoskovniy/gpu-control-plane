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
VERSION=${MODULE_SDK_VERSION:-0.5.0}
BINARY_PATH="${INSTALL_DIR}/module-sdk"
WRAPPER_SOURCE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/module-sdk-wrapper.sh"

mkdir -p "${INSTALL_DIR}"

if [[ -x "${BINARY_PATH}" ]]; then
  if MODULE_SDK_VERSION="${VERSION}" "${BINARY_PATH}" version 2>/dev/null | grep -q "${VERSION}"; then
    echo "module-sdk ${VERSION} is already installed in ${BINARY_PATH}" >&2
    exit 0
  fi
  echo "module-sdk found with a different version — reinstalling" >&2
fi

cp "${WRAPPER_SOURCE}" "${BINARY_PATH}"
chmod +x "${BINARY_PATH}"

echo "Installed module-sdk wrapper ${VERSION} → ${BINARY_PATH}" >&2
