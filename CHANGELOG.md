# Changelog for argocd-image-updater

This is the change log for `argocd-image-updater`. Please read thoroughly
when you upgrade versions, as there might be non-obvious changes that need
handling on your side.

## Unreleased

### Upgrade notes (no really, you MUST read this)

N/A

### Bug fixes

* fix: Do not check for semver if update strategy is latest

### New features

* feat: Allow filtering applications by name patterns

### Other changes

* enhancement: Slightly increase verbosity in default log level
* enhancement: Provide default RBAC rules for serviceaccount

## 2020-08-18 - Release v0.4.0

### Upgrade notes (no really, you MUST read this)

N/A

### Bug fixes

* fix: Properly load registry configuration
* fix: Use a default path for registries.conf
* fix: Make installation base compatible with Kustomize v2

### New features

* feat: Allow filtering of tags using built-in filter functions
* feat: Allow specifying per-image pull secrets
* feat: Support GitHub Docker registry

### Other changes

* refactor: Lots of refactoring "under the hood"

## 2020-08-11 - Release v0.3.1

### Upgrade notes (no really, you MUST read this)

### Bug fixes

* fix: Only fetch metadata when require by update strategy

### New features

### Other changes

## 2020-08-10 - Release v0.3.0

### Upgrade notes (no really, you MUST read this)

* Syntax change for running: `argocd-image-updater run [flags]` instead of `argocd-image-updater [flags]` has now to be used
* **Attention:** Helm annotation names have changed from `<image_alias>.image-{name,tag,spec}` to `<image_alias>.helm.image-{name,tag,spec}`
* Specifying target image name for Kustomize applications now require their own annotation, the image alias is not re-used for this anymore

### Bug fixes

* fix: Possible race while waiting for app updating goroutines

### New features

* feat: Allow setting the sort mode for tags per image via annotation

### Other changes

* refactor: Change run behaviour by providing `run` and `version` commands
* enhancement: Provide a `version` command to print out version information
* enhancement: Allow storing metadata for image tags
* enhancement: Fetch tag metadata along with tags and store creation timestamp
* enhancement: Introduce simple cache for immutable metadata
* refactor: Make version constraints parametrizable
* enhancement: Allow sorting of tags by semver, date or name
* refactor: Give annotation names their own namespace-like living room
* enhancement: Kustomize target image name got its own annotation

## 2020-08-06 - Release v0.2.0

### Upgrade notes (no really, you MUST read this)

### Bug fixes

* fix: Correctly get Helm target parameter names from annotations
* fix: Enforce sane concurrency limit

### New features

* feat: Introduce dry run mode
* feat: Allow for concurrent update of multiple applications

### Other changes

refactor: Reduced number of necessary ArgoCD API requests (#4)

## 2020-08-06 - Release v0.1.1

Quick bug-fix release to get rid of some left-over working names

### Upgrade notes (no really, you MUST read this)

### Bug fixes

* Changed the binary name from `argocd-image-controller` (old working name) to
`argocd-image-updater`.

### New features

N/A

### Other changes

N/A

## 2020-08-05 - Release v0.1.0

Initial release.

### Upgrade notes (no really, you MUST read this)

N/A

### Bug fixes

N/A

### New features

N/A

### Other changes
