# E2E Tests for Custom CA Certificate Support

## Overview

Test file: `1-012-custom-ca-registry_test.go`

This test suite validates the custom CA certificate feature for registries (PR #1743).
It tests all three ways to provide CA certificates for registries with self-signed TLS
certificates, plus a negative control that asserts what happens when none of them is used.

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

### 4. No CA configured (negative control)
**Test:** "should not update image when no CA certificate is configured for the registry"

- Identical to scenario 3 except that no `argocd-tls-certs-cm` is created, so the
  registry has no trust anchor anywhere: no `ca_data`, no `ca_file`, nothing to
  auto-discover
- Asserts the controller logs `x509: certificate signed by unknown authority`, then
  asserts with `Consistently` that the Application's Kustomize images stay empty
- Configuration:
  ```yaml
  registries:
  - name: Local Registry without a CA
    api_url: https://e2e-registry-public.argocd-operator-system.svc.cluster.local
    prefix: 127.0.0.1:30000
  ```

## How the Tests Work

Scenarios 1-3 follow this pattern:

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
   - Specifies `127.0.0.1:30000/test-image:~1.0` with the semver strategy
   - Sets `forceUpdate: true` because that image is not one the guestbook
     Application actually runs; without it `GetImagesAndAliasesFromApplication`
     skips any configured image missing from `.status.summary.images`

5. **Verify Image Update**
   - Monitors the Application's Kustomize images field
   - Confirms the image was rewritten to `127.0.0.1:30000/test-image:1.0.2`
     (proves TLS handshake worked with custom CA)

Scenario 4 shares steps 1-4, then asserts the opposite outcome: first that the
certificate was rejected, and only then that no update ever lands.

## Prerequisites

The simplest path is `make -C test/ginkgo test-e2e`, which does everything below
and then runs the full parallel and sequential suites. To set things up
piecemeal, from `test/ginkgo`:

1. **k3d Cluster is Running** (the suites run on k3d, not Kind)
   ```bash
   k3d cluster list     # check
   make k3d-cluster-create
   ```

2. **Test Registry is Running**
   ```bash
   make create-local-container-registry
   make push-signature-test-images   # pushes test-image 1.0.0/1.0.1/1.0.2
   ```
   This deploys:
   - `e2e-registry-public` deployment in `argocd-operator-system` namespace
   - TLS secret `e2e-registry-public-tls` with a self-signed certificate whose
     SANs cover the in-cluster Service DNS names and `127.0.0.1`
   - Service exposing registry on NodePort 30000

## Running the Tests

The parallel suite runs via the root Makefile's `e2e-tests-parallel-ginkgo`
target. To run only these specs, invoke the ginkgo CLI with a focus directly
from the repository root:

```bash
# All custom CA tests
./bin/ginkgo -v --trace --timeout 90m --focus "1-012-custom-ca-registry" -r ./test/ginkgo/parallel

# A specific scenario
./bin/ginkgo -v --trace --focus "ca_data" -r ./test/ginkgo/parallel
./bin/ginkgo -v --trace --focus "ca_file" -r ./test/ginkgo/parallel
./bin/ginkgo -v --trace --focus "auto-discovery" -r ./test/ginkgo/parallel
```

(`make ginkgo` installs the CLI into `./bin` if it is not present.)

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

### No Controller Restart Is Needed

`registries.conf` and `argocd-tls-certs-cm` are both read once, at controller
startup, and are never reloaded afterwards. In production that means a restart
after changing either one.

The tests avoid needing one by creating both ConfigMaps *before* the ArgoCD CR, so
the operator has them mounted by the time it creates the image updater Deployment
and the very first pod starts with the certificate already in place. That ordering
is load-bearing - moving the `argocd-tls-certs-cm` creation after the ArgoCD CR
would make scenarios 2 and 3 fail until the pod happened to restart.

### Why the Negative Control Checks the Logs

Scenario 4 asserts that no update happens, which on its own is a weak oracle: a
typo in the `prefix`, an ImageUpdater CR that matches nothing, or a controller that
never started would all produce the same "nothing happened" and keep the test green
long after it stopped guarding anything. Requiring the x509 error in the controller
logs first pins the reason, turning the assertion into "no update, *because* the
certificate was rejected".

## Troubleshooting

### Test Registry Not Found
```
registry TLS secret not found - ensure e2e prerequisites are deployed
```
Solution: Run prerequisite setup:
```bash
cd test/ginkgo
make create-local-container-registry
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

Scenario 4 spends a fixed 90 seconds in `Consistently` proving the absence of an
update. For reference, the positive scenarios land their update within roughly
20-30 seconds of the controller becoming ready, so that window is a wide margin.

## Related Files

- Feature implementation: `registry-scanner/pkg/registry/config.go`
- Unit tests: `registry-scanner/pkg/registry/endpoints_test.go`
- Documentation: `docs/configuration/registries.md`
- Test registry manifests: `test/ginkgo/prereqs/assets/registry.yaml`
- Registry certificate generation: `test/ginkgo/prereqs/assets/generate-registry-tls-secrets.sh`
  (the certificate must carry subjectAltName entries — Go has ignored the
  Common Name for hostname verification since 1.15, so a CN-only certificate
  fails verification no matter which of the three CA mechanisms is used)
