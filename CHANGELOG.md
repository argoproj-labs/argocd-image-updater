# Changelog for argocd-image-updater

This is the change log for `argocd-image-updater`. Please read thoroughly
when you upgrade versions, as there might be non-obvious changes that need
handling on your side.

## Unreleased

### Upgrade notes (no really, you MUST read this)

N/A

### Bug fixes

### New features

### Other changes

## 2020-12-06 - Release v0.8.0

### Upgrade notes (no really, you MUST read this)

* **Attention**: For the `latest` update-strategy, `argocd-image-updater` now fetches v2 manifests by default, instead of the v1 manifests in previous versions. This is to improve compatibility with registry APIs, but may result in a significant higher number of manifest pulls. Due to the recent pull limits imposed by Docker Hub, it is **not recommended** to use `latest` updated strategy with Docker Hub registry anymore if those pull limits are enforced on your account and/or images, especially if you have more than a couple of tags in your image's repository. Fetching meta data for any given tag counts as two pulls from the view point of Docker Hub.

* The default rate limit for API requests is 20 requests per second per registry. If this is too much for your registry, please lower this value in the `registries.conf` by setting `ratelimit` to a lower value.

### Bug fixes

* fix: Correctly apply ignore list when matchfunc is not set (#116)
* fix: Prevent nil pointer dereference in image creds (#126)

### New features

* feat: Get tag creation date also from v2 manifest schemas (#115)
* feat: add argocd-image-updater test command (#117)
* feat: Implement rate limiter and metadata parallelism (#118)
* feat: Support for getting pull creds from external scripts (#121)
* feat: Export Prometheus compatible metrics (#123)
* feat: Support for expiring credentials (#124)

### Other changes

* chore: Update to Golang v1.14.13

## 2020-09-27 - Release v0.7.0

### Upgrade notes (no really, you MUST read this)

**Deprecation notice:** The annotation `argocd-image-updater.argoproj/<image>.tag-match` has been deprecated in favour of `argocd-image-updater.argoproj/<image>.allow-tags` to be consistent with the new `argocd-image-updater.argoproj/<image>.ignore-tags` annotation. The old annotation will still work, but a warning message will be issued in the log. Users are encouraged to rename their annotations asap, as the `tag-match` annotation is subject to removal in a future version of the image updater.

### Bug fixes
* fix: Correctly parse & use pull secret entries without protocol

### New features

* feat: Support for GitHub Container Registry (ghcr.io)
* feat: Allow setting log level from configmap (and environment)
* feat: Allow ignoring set of tags

### Other changes

* refactor: Introduce allow-tags and deprecate tag-match annotation
* chore: Externalize version & build information


## 2020-09-25 - Release v0.6.2

### Upgrade notes (no really, you MUST read this)
N/A

### Bug fixes
* fix: Tag sort mode for custom registries aren't honored

### New features
* feat: Allow configuration of default namespace for registries

### Other changes
N/A

## 2020-09-22 - Release v0.6.1

### Upgrade notes (no really, you MUST read this)
N/A

### Bug fixes
* fix: Make insecure TLS connections to registries actually work

### New features
N/A

### Other changes
N/A

## 2020-09-22 - Release v0.6.0

### Upgrade notes (no really, you MUST read this)
N/A

### Bug fixes

* fix: Use default Helm parameter names if none given in annotations 
* fix: Application spec updates should be atomic

### New features

* feat: Allow insecure TLS connections to registries

### Other changes

* chore: Update Argo CD client to 1.7.4
* chore: Update K8s client to v1.18.8

## 2020-09-10 - Release v0.5.1

### Upgrade notes (no really, you MUST read this)
N/A

### Bug fixes

* fix: Correctly parse version constraints containing equal signs

### New features
N/A

### Other changes
N/A

## 2020-08-29 - Release v0.5.0

### Upgrade notes (no really, you MUST read this)

If you use the `latest` or `name` update strategy and relied on the semantic
version constraint to limit the list of tags to consider, you will need to
use an additional `tag-match` annotation to limit the tags. The constraint
will only be used for update strategy `semver` from v0.5.0 onwards.

### Bug fixes

* fix: Do not constraint tags to semver if update strategy is latest
* fix: Multiple same images in the same application not possible

### New features

* feat: Allow filtering applications by name patterns

### Other changes

* enhancement: Slightly increase verbosity in default log level
* enhancement: Provide default RBAC rules for serviceaccount
* enhancement: Warm-up cache before starting image cycle

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
