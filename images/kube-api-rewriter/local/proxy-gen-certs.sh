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

NAMESPACE="${1:-gpu-control-plane-local}"
OUT_DIR="${2:-$(pwd)/local/_certs}"

mkdir -p "${OUT_DIR}"

CA_KEY="${OUT_DIR}/ca.key"
CA_CRT="${OUT_DIR}/ca.crt"
SRV_KEY="${OUT_DIR}/tls.key"
SRV_CSR="${OUT_DIR}/server.csr"
SRV_CRT="${OUT_DIR}/tls.crt"
SRV_CNF="${OUT_DIR}/server.cnf"

cat <<EOF
Generating certificates for kube-api-rewriter in namespace '${NAMESPACE}'.
Output directory: ${OUT_DIR}
EOF

cat >"${SRV_CNF}" <<EOF
[req]
distinguished_name = dn
req_extensions = v3_req
prompt = no

[dn]
CN = gpu-api-rewriter.${NAMESPACE}.svc

[v3_req]
keyUsage = critical,digitalSignature,keyEncipherment
extendedKeyUsage = serverAuth
subjectAltName = @alt_names

[alt_names]
DNS.1 = gpu-api-rewriter
DNS.2 = gpu-api-rewriter.${NAMESPACE}
DNS.3 = gpu-api-rewriter.${NAMESPACE}.svc
DNS.4 = gpu-api-rewriter.${NAMESPACE}.svc.cluster.local
EOF

openssl req -x509 -nodes -newkey rsa:4096 \
  -subj "/CN=gpu-api-rewriter-ca" \
  -days 365 \
  -keyout "${CA_KEY}" \
  -out "${CA_CRT}"

openssl req -nodes -newkey rsa:4096 \
  -keyout "${SRV_KEY}" \
  -out "${SRV_CSR}" \
  -config "${SRV_CNF}"

openssl x509 -req \
  -in "${SRV_CSR}" \
  -CA "${CA_CRT}" \
  -CAkey "${CA_KEY}" \
  -CAcreateserial \
  -out "${SRV_CRT}" \
  -days 365 \
  -extensions v3_req \
  -extfile "${SRV_CNF}"

cat <<EOF

Certificates generated:
  CA:   ${CA_CRT}
  Key:  ${SRV_KEY}
  Cert: ${SRV_CRT}

Create the Kubernetes secret with:

kubectl -n ${NAMESPACE} create secret generic gpu-api-rewriter-tls \\
  --from-file=tls.crt=${SRV_CRT} \\
  --from-file=tls.key=${SRV_KEY} \\
  --from-file=ca.crt=${CA_CRT}

EOF
