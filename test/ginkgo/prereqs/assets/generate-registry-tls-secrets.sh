#!/bin/bash
# Assisted-by: Claude AI model
# Generate TLS certificates and create Kubernetes secrets for registry deployments
# This script generates self-signed certificates and creates kubernetes.io/tls secrets

set -e

NAMESPACE="${NAMESPACE:-argocd-operator-system}"
CERT_VALIDITY_DAYS="${CERT_VALIDITY_DAYS:-365}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Check if required tools are available
if ! command -v openssl &> /dev/null; then
  echo -e "${RED}Error: openssl is not installed or not in PATH${NC}" >&2
  exit 1
fi

if ! command -v kubectl &> /dev/null; then
  echo -e "${RED}Error: kubectl is not installed or not in PATH${NC}" >&2
  exit 1
fi

echo -e "${GREEN}Generating TLS certificates for registry deployments...${NC}"

# Create temporary directory for certificates
TMP_DIR=$(mktemp -d)
trap "rm -rf $TMP_DIR" EXIT

# Function to generate certificate and create secret
generate_secret() {
  local SECRET_NAME=$1
  local CERT_FILE="$TMP_DIR/${SECRET_NAME}.crt"
  local KEY_FILE="$TMP_DIR/${SECRET_NAME}.key"

  # The secret is named after the registry Service with a "-tls" suffix, e.g.
  # e2e-registry-public-tls -> Service e2e-registry-public.
  local SERVICE_NAME="${SECRET_NAME%-tls}"

  # Go has ignored the certificate Common Name for hostname verification since 1.15,
  # so the cert must carry subjectAltName entries for every name a client may dial:
  # the in-cluster Service DNS names (used by the image-updater pod) and 127.0.0.1
  # (used by the host when pushing test images through the NodePort).
  local SANS="DNS:${SERVICE_NAME}"
  SANS="${SANS},DNS:${SERVICE_NAME}.${NAMESPACE}"
  SANS="${SANS},DNS:${SERVICE_NAME}.${NAMESPACE}.svc"
  SANS="${SANS},DNS:${SERVICE_NAME}.${NAMESPACE}.svc.cluster.local"
  SANS="${SANS},DNS:localhost,IP:127.0.0.1"

  echo -e "${YELLOW}Generating certificate for ${SECRET_NAME} (SANs: ${SANS})...${NC}"

  # Generate private key
  openssl genrsa -out "$KEY_FILE" 2048 2>/dev/null

  # Generate self-signed certificate in a single step. "openssl x509 -req" does not
  # copy extensions from a CSR by default, so the SANs would be silently dropped if
  # the CSR and signing steps were kept separate.
  # The same certificate is used both as the registry's server cert and as the trust
  # anchor clients import, hence CA:TRUE plus the key usages needed for both roles.
  openssl req -x509 -new -key "$KEY_FILE" -days "$CERT_VALIDITY_DAYS" -out "$CERT_FILE" \
    -subj "/C=US/ST=State/L=City/O=Organization/CN=${SERVICE_NAME}.${NAMESPACE}.svc.cluster.local" \
    -addext "subjectAltName=${SANS}" \
    -addext "basicConstraints=critical,CA:TRUE" \
    -addext "keyUsage=critical,digitalSignature,keyEncipherment,keyCertSign" \
    -addext "extendedKeyUsage=serverAuth" 2>/dev/null

  # Create or update Kubernetes secret
  echo -e "${YELLOW}Creating Kubernetes secret ${SECRET_NAME} in namespace ${NAMESPACE}...${NC}"
  
  # Check if namespace exists, create if it doesn't
  if ! kubectl get namespace "$NAMESPACE" &>/dev/null; then
    echo -e "${YELLOW}Namespace ${NAMESPACE} does not exist, creating it...${NC}"
    kubectl create namespace "$NAMESPACE"
  fi
  
  # Create secret using kubectl create secret tls
  kubectl create secret tls "$SECRET_NAME" \
    --cert="$CERT_FILE" \
    --key="$KEY_FILE" \
    --namespace="$NAMESPACE" \
    --dry-run=client -o yaml | kubectl apply -f -
  
  echo -e "${GREEN}Secret ${SECRET_NAME} created/updated successfully${NC}"
}

# Generate secrets for both registries
generate_secret "e2e-registry-public-tls"
generate_secret "e2e-registry-private-tls"

echo -e "${GREEN}All TLS secrets generated and applied successfully!${NC}"

