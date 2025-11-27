#!/bin/bash
set -e

# Generate self-signed certificates for the webhook
# These are used when not using cert-manager

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CERT_DIR="${SCRIPT_DIR}/../certs"
NAMESPACE="${NAMESPACE:-workload-schedule-operator-system}"
SERVICE_NAME="${SERVICE_NAME:-workload-schedule-operator-webhook-service}"

echo "=== Generating self-signed certificates ==="

# Create cert directory
mkdir -p "${CERT_DIR}"

# Generate CA
echo "Generating CA..."
openssl genrsa -out "${CERT_DIR}/ca.key" 2048
openssl req -x509 -new -nodes -key "${CERT_DIR}/ca.key" -subj "/CN=Webhook CA" -days 3650 -out "${CERT_DIR}/ca.crt"

# Generate server key and CSR
echo "Generating server certificate..."
openssl genrsa -out "${CERT_DIR}/tls.key" 2048

# Create CSR config
cat > "${CERT_DIR}/csr.conf" <<EOF
[req]
req_extensions = v3_req
distinguished_name = req_distinguished_name
prompt = no

[req_distinguished_name]
CN = ${SERVICE_NAME}.${NAMESPACE}.svc

[v3_req]
keyUsage = digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth
subjectAltName = @alt_names

[alt_names]
DNS.1 = ${SERVICE_NAME}
DNS.2 = ${SERVICE_NAME}.${NAMESPACE}
DNS.3 = ${SERVICE_NAME}.${NAMESPACE}.svc
DNS.4 = ${SERVICE_NAME}.${NAMESPACE}.svc.cluster.local
EOF

# Generate CSR
openssl req -new -key "${CERT_DIR}/tls.key" -out "${CERT_DIR}/server.csr" -config "${CERT_DIR}/csr.conf"

# Sign the certificate
openssl x509 -req -in "${CERT_DIR}/server.csr" -CA "${CERT_DIR}/ca.crt" -CAkey "${CERT_DIR}/ca.key" \
    -CAcreateserial -out "${CERT_DIR}/tls.crt" -days 365 -extensions v3_req -extfile "${CERT_DIR}/csr.conf"

# Generate base64 encoded CA bundle for webhook configuration
CA_BUNDLE=$(base64 -w 0 "${CERT_DIR}/ca.crt")
echo "${CA_BUNDLE}" > "${CERT_DIR}/ca-bundle.b64"

echo "=== Certificates generated in ${CERT_DIR} ==="
echo "  - ca.crt: CA certificate"
echo "  - ca.key: CA private key"  
echo "  - tls.crt: Server certificate"
echo "  - tls.key: Server private key"
echo "  - ca-bundle.b64: Base64 encoded CA bundle for webhook config"

