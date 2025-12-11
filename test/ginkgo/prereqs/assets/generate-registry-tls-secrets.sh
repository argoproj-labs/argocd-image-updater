#!/bin/bash
# Assisted-by: Claude AI model
# Generate TLS certificates and create Kubernetes secrets for registry deployments
# This script generates self-signed certificates and creates kubernetes.io/tls secrets

set -e

NAMESPACE="${NAMESPACE:-argocd-operator-system}"
CERT_VALIDITY_DAYS="${CERT_VALIDITY_DAYS:-365}"
CN="${CN:-e2e-registry}"

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

  echo -e "${YELLOW}Generating certificate for ${SECRET_NAME}...${NC}"
  
  # Generate private key
  openssl genrsa -out "$KEY_FILE" 2048 2>/dev/null
  
  # Generate certificate signing request
  openssl req -new -key "$KEY_FILE" -out "$TMP_DIR/${SECRET_NAME}.csr" \
    -subj "/C=US/ST=State/L=City/O=Organization/CN=${CN}" 2>/dev/null
  
  # Generate self-signed certificate
  openssl x509 -req -days "$CERT_VALIDITY_DAYS" -in "$TMP_DIR/${SECRET_NAME}.csr" \
    -signkey "$KEY_FILE" -out "$CERT_FILE" 2>/dev/null
  
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

