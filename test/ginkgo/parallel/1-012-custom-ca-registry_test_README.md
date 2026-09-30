# E2E Tests for Custom CA Certificate Support

## Overview

Test file: `1-012-custom-ca-registry_test.go`

This test suite validates the custom CA certificate feature for registries (PR #1743).
It tests all three ways to provide CA certificates for registries with self-signed TLS certificates.

## Test Scenarios

### 1. ca_data (Inline Certificate)
**Test:** "should connect to registry with self-signed cert using ca_data (inline certificate)"

- Extracts CA cert from `e2e-registry-public-tls` secret
- Embeds the PEM certificate directly in `registries.conf` as YAML multiline string
- Configuration:
  ```yaml
  registries:
  - name: Local Registry with ca_data
    api_url: https://e2e-registry-public.argocd-operator-system.svc.cluster.local
    prefix: 127.0.0.1:30000
    ca_data: |
      -----BEGIN CERTIFICATE-----
      ...
      -----END CERTIFICATE-----
  ```

### 2. ca_file (Mounted Certificate File)
**Test:** "should connect to registry with self-signed cert using ca_file (mounted in argocd-tls-certs-cm)"

- Creates `argocd-tls-certs-cm` ConfigMap with custom filename: `custom-registry-ca.crt`
- Argocd-operator automatically mounts this ConfigMap to `/app/config/tls`
- References the file explicitly via `ca_file` in `registries.conf`
- Configuration:
  ```yaml
  registries:
  - name: Local Registry with ca_file
    api_url: https://e2e-registry-public.argocd-operator-system.svc.cluster.local
    prefix: 127.0.0.1:30000
    ca_file: /app/config/tls/custom-registry-ca.crt
  ```

### 3. Auto-discovery
**Test:** "should connect to registry with self-signed cert using auto-discovery (argocd-tls-certs-cm)"

- Creates `argocd-tls-certs-cm` ConfigMap with hostname-based key
- Key matches the hostname from `api_url`: `e2e-registry-public.argocd-operator-system.svc.cluster.local`
- No `ca_file` or `ca_data` in `registries.conf` - auto-discovered from `/app/config/tls/<hostname>`
- Configuration:
  ```yaml
  registries:
  - name: Local Registry with auto-discovery
    api_url: https://e2e-registry-public.argocd-operator-system.svc.cluster.local
    prefix: 127.0.0.1:30000
  ```

## How the Tests Work

Each test follows this pattern:

1. **Extract CA Certificate**
   - Gets CA cert from `e2e-registry-public-tls` secret in `argocd-operator-system` namespace
   - This is the self-signed cert used by the test registry

2. **Setup Test Environment**
   - Creates random test namespace
   - Configures `argocd-image-updater-config` ConfigMap with appropriate registry configuration
   - Creates ArgoCD CR with ImageUpdater enabled
   - Waits for all ArgoCD components to be ready

3. **Create Application**
   - Creates an Argo CD Application using the public guestbook example
   - Uses image from `quay.io` initially

4. **Create ImageUpdater CR**
   - Configures ImageUpdater to watch applications matching `app*`
   - Specifies image update for guestbook using semver strategy

5. **Restart Controller** (for scenarios 2 & 3)
   - Deletes the ImageUpdater controller pod
   - Ensures `argocd-tls-certs-cm` changes are picked up

6. **Verify Image Update**
   - Monitors the Application's Kustomize images field
   - Confirms image update succeeded (proves TLS handshake worked with custom CA)

## Prerequisites

Before running these tests, ensure:

1. **Test Registry is Running**
   ```bash
   make test-e2e-prereqs
   ```
   This deploys:
   - `e2e-registry-public` deployment in `argocd-operator-system` namespace
   - TLS secret `e2e-registry-public-tls` with self-signed certificate
   - Service exposing registry on NodePort 30000

2. **Kind Cluster is Running**
   ```bash
   # Check if cluster exists
   kind get clusters
   
   # If not, create it
   make test-e2e-cluster
   ```

## Running the Tests

### Run all custom CA tests:
```bash
cd test/ginkgo
make test-parallel GINKGO_FOCUS="1-012-custom-ca-registry"
```

### Run a specific scenario:
```bash
# ca_data test
make test-parallel GINKGO_FOCUS="ca_data"

# ca_file test
make test-parallel GINKGO_FOCUS="ca_file"

# auto-discovery test
make test-parallel GINKGO_FOCUS="auto-discovery"
```

### Run with verbose output:
```bash
make test-parallel GINKGO_FOCUS="1-012" GINKGO_ARGS="-v"
```

## Key Design Decisions

### Why Test All Three Approaches?

1. **ca_data** - Most portable, doesn't require volume mounts, good for GitOps
2. **ca_file** - Useful when certs are managed externally or rotated frequently
3. **Auto-discovery** - Seamless integration with Argo CD's existing TLS cert management

### Using argocd-tls-certs-cm for ca_file Test

The `ca_file` test uses `argocd-tls-certs-cm` instead of a separate ConfigMap because:
- Argocd-operator already mounts this ConfigMap to `/app/config/tls`
- No need to modify the ArgoCD CR or operator behavior
- Demonstrates that `argocd-tls-certs-cm` can contain arbitrary certificate files, not just hostname-based keys

### Controller Restart Required

Scenarios 2 and 3 restart the controller pod because:
- ConfigMap changes aren't automatically reloaded
- The registry client caches TLS configuration at startup
- In production, users would typically restart the controller after updating `argocd-tls-certs-cm`

## Troubleshooting

### Test Registry Not Found
```
Error: registry TLS secret not found - run 'make test-e2e-prereqs' first
```
Solution: Run prerequisite setup:
```bash
cd test/ginkgo
make test-e2e-prereqs
```

### Vendor Sync Issues
```
Error: inconsistent vendoring
```
Solution: This is a pre-existing issue in the test module, bypass with:
```bash
go test -mod=mod ./parallel/...
```

### Image Update Not Happening
Check controller logs:
```bash
kubectl logs -n <test-namespace> deployment/argocd-argocd-image-updater-controller
```
Look for:
- TLS handshake errors
- Certificate validation failures
- Registry connection errors

## Test Duration

- Each scenario: ~3-5 minutes
- Total suite: ~10-15 minutes (parallel execution)

## Related Files

- Feature implementation: `registry-scanner/pkg/registry/config.go`
- Unit tests: `registry-scanner/pkg/registry/endpoints_test.go`
- Documentation: `docs/configuration/registries.md`
- Test registry manifests: `test/ginkgo/prereqs/assets/registry.yaml`
