# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build and Development Commands

```bash
# Build the binary (output: dist/argocd-image-updater)
make build

# Run all unit tests (excludes test/, mocks/, e2e/)
make test

# Run tests with race detector
make test-race

# Run a single test package
KUBEBUILDER_ASSETS="$(./bin/setup-envtest use 1.31.0 --bin-dir bin -p path)" \
  go test ./pkg/argocd/... -run TestFunctionName -v

# Lint
make lint
make lint-fix

# Run the controller locally (requires kubeconfig)
make run

# Test image update behavior without a cluster
go run ./cmd/main.go test nginx --semver-constraint ">=1.20"

# Generate CRD manifests and DeepCopy code (required after changing api/v1alpha1 types)
make manifests generate

# Run Ginkgo E2E tests (requires a live cluster)
make e2e-tests-sequential-ginkgo
make e2e-tests-parallel-ginkgo
```

## Module Structure

This repository contains two Go modules:

- **Root module** (`github.com/argoproj-labs/argocd-image-updater`): The controller, CRD types, argocd integration, git write-back, and webhook server.
- **`registry-scanner/` submodule** (`github.com/argoproj-labs/argocd-image-updater/registry-scanner`): Container registry querying, image/tag parsing, version constraint evaluation, and credential handling. Used as a `require` dependency by the root module.

When changing types or logic in `registry-scanner/`, run `go mod tidy` in both the `registry-scanner/` directory and the root.

## Architecture Overview

### Core Reconciliation Loop

`internal/controller/imageupdater_controller.go` — The `ImageUpdaterReconciler` is a controller-runtime reconciler that watches `ImageUpdater` CRs. On each reconcile it calls `RunImageUpdater` (in `reconcile.go`), which:

1. Creates an `ArgoCDK8sClient` to fetch ArgoCD `Application` objects.
2. Calls `argocd.FilterApplicationsForUpdate` to match Applications against `ApplicationRef` patterns/selectors in the CR.
3. Fans out concurrently (bounded by `MaxConcurrentApps`) into `argocd.UpdateApplication` for each matched Application.
4. Writes back changes either via the ArgoCD API parameter overrides or via a Git commit.

### Configuration: CRD vs Annotations

`ImageUpdater` CRs (`api/v1alpha1/imageupdater_types.go`) are the primary configuration mechanism. Each CR specifies:
- `applicationRefs[]` — glob patterns + label selectors to match ArgoCD Applications
- `commonUpdateSettings` — update strategy, semver constraints, tag filters, pull secrets
- `writeBackConfig` — ArgoCD API or Git write-back, branch, commit message template
- `images[]` — list of container images to track

When `applicationRef.useAnnotations: true`, image configuration is read from ArgoCD Application annotations (`argocd-image-updater.argoproj.io/*`) instead of the CR — this is the legacy mode, handled in `pkg/argocd/annotations.go`.

### Image Update Pipeline (`pkg/argocd/update.go`)

`UpdateApplication` is the per-application update function:
1. Reads the current deployed image tags from Application status.
2. For each tracked image, queries the registry via `registry-scanner` for available tags.
3. Evaluates version constraints (semver, latest, digest, name, etc.) against available tags.
4. If a newer tag is found, calls the write-back method.

### Write-back Methods (`pkg/argocd/`)

- **ArgoCD API** (`argocd.go`, `update.go`): Updates Application `spec.source.helm.parameters` or `spec.source.kustomize.images` via `UpdateSpec`.
- **Git** (`git.go`): Clones the target repo, modifies Helm values or Kustomize overlays, commits and pushes. Uses `ext/git` for git operations. Supports PR creation via `pkg/argocd/pr_github.go`, `pr_gitlab.go`.

### Registry Scanner (`registry-scanner/pkg/`)

- `registry/` — `RegistryEndpoint` holds connection config and credential cache. `GetTags` fetches available tags from the OCI distribution API.
- `image/` — `ContainerImage` parses image identifiers (`registry/name:tag@digest`). `VersionConstraint` holds the update strategy + filter rules.
- `tag/` — `ImageTag` represents a parsed tag with semver, metadata, and digest. `ImageTagList` holds the full list from a registry fetch.
- `cache/` — in-memory tag cache with TTL to reduce registry API calls.

### Webhook Server (`pkg/webhook/`)

Handles registry push events from Docker Hub, Quay, GitHub Container Registry, Harbor, Alibaba ACR, and generic CloudEvents. On receiving a push event, it finds the matching ImageUpdater CRs and triggers an immediate reconcile instead of waiting for the poll interval.

### Metrics (`pkg/metrics/`)

Prometheus metrics for reconcile counts, image update counts, and per-CR application counts. Served on the metrics endpoint configured at startup.

## Key Conventions

**Testing**: Unit tests use `testify`. E2E tests use Ginkgo v2/Gomega. Mock generation via `mockery` — run `go generate ./...` in the relevant package. The `make test` target excludes `test/`, `mocks/`, and `e2e/` directories; run those separately.

**CRD changes**: After modifying `api/v1alpha1/imageupdater_types.go`, always run `make manifests generate` to regenerate `zz_generated.deepcopy.go` and CRD YAML in `config/crd/bases/`. Commit both.

**Registry credentials**: Pulled from Kubernetes Secrets at runtime via `pkg/kube/` — never hardcoded. The `registry-scanner/pkg/registry/creds.go` implements credential resolution from pull secrets.

**Write-back target files**: Git write-back writes to `.argocd-source-<appname>_<namespace>.yaml` (or `.argocd-source-<appname>.yaml` without namespace) by default — see `pkg/common/constants.go`.

**Semver constraints**: Uses `github.com/Masterminds/semver/v3`. Constraints in CRs/annotations follow Masterminds semver syntax (e.g., `>=1.20 <2.0`).
