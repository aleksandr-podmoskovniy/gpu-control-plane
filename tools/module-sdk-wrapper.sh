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

MODULE_SDK_VERSION="${MODULE_SDK_VERSION:-0.3.7}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

main() {
  local cmd="${1:-}"
  case "${cmd}" in
    version)
      echo "module-sdk version ${MODULE_SDK_VERSION}"
      ;;
    test)
      shift || true
      if [[ $# -eq 0 ]]; then
        echo "module-sdk test: missing package path" >&2
        exit 1
      fi
      run_go_test "$@"
      ;;
    *)
      echo "module-sdk: unsupported command ${cmd}" >&2
      exit 1
      ;;
  esac
}

run_go_test() {
  local pkg="${1#./}"
  shift || true

  case "${pkg}" in
    hooks/*)
      (cd "${ROOT}/hooks" && GO111MODULE=on go test "./${pkg#hooks/}" "$@")
      ;;
    src/controller/*)
      (cd "${ROOT}/src/controller" && GO111MODULE=on go test "./${pkg#src/controller/}" "$@")
      ;;
    *)
      (cd "${ROOT}" && GO111MODULE=on go test "${pkg}" "$@")
      ;;
  esac
}

main "$@"
