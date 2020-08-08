# Changelog for argocd-image-updater

This is the change log for `argocd-image-updater`. Please read thoroughly
when you upgrade versions, as there might be non-obvious changes that need
handling on your side.

## Unreleased

### Upgrade notes (no really, you MUST read this)

* Syntax change for running: `argocd-image-updater run [flags]` instead of `argocd-image-updater [flags]` has now to be used

### Bug fixes

### New features

### Other changes

* refactor: Change run behaviour by providing `run` and `version` commands
* enhancement: Provide a `version` command to print out version information
* enhancment: Allow storing metadata for image tags

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
