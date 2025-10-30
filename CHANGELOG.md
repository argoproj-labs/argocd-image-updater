# Changelog for argocd-image-updater

This is the change log for `argocd-image-updater`. Please read thoroughly
when you upgrade versions, as there might be non-obvious changes that need
handling on your side.

## Unreleased

<<<<<<< HEAD
## 2025-10-30 - Release v100.0.12

### Fixes/Improvements

- metrics: Garbage-collect stale metrics for deleted applications
  - Prevents false alerts from deleted apps by removing their metric series
  - New `DeleteAppMetrics()` function removes all per-application metric series
  - GC runs automatically at start of each update cycle (both cycle and continuous modes)
  - Thread-safe implementation with mutex-protected app tracking
  - Affects all per-application metrics: images_watched_total, images_updated_total, 
    images_errors_total, application_update_duration_seconds, 
    application_last_attempt_timestamp, application_last_success_timestamp,
    images_considered_total, images_skipped_total

### Notes

- No configuration changes required
- Metrics for deleted applications are automatically cleaned up on next update cycle
- Old metric series will disappear from Prometheus scrapes after GC runs

## 2025-09-23 - Release v100.0.11

### Fixes/Improvements

- health: Fail liveness on sustained port exhaustion to auto-restart pod
  - /healthz returns 503 when EADDRNOTAVAIL errors exceed threshold within window
  - Enabled by default; control via envs:
    - `HEALTH_FAIL_ON_PORT_EXHAUSTION` (default: true)
    - `PORT_EXHAUSTION_WINDOW` (default: 60s)
    - `PORT_EXHAUSTION_THRESHOLD` (default: 8)
- registry: Record EADDRNOTAVAIL in HTTP transport; sliding-window tracker powers health gate

### Notes

- No manifest changes required; your existing livenessProbe on `/healthz` will now trigger restart when ports are exhausted.

=======
>>>>>>> dff4023 (feat(registry): add transport janitor; tests; docs; bump to v100.0.10\n\n- registry: periodic CloseIdleConnections across cached transports\n- cmd(run): wire janitor via REGISTRY_TRANSPORT_JANITOR_INTERVAL (default 5m)\n- tests: janitor unit test; stabilize webhook tests (ports, status, body)\n- docs: changelog entry\n- version: 100.0.10)
## 2025-09-23 - Release v100.0.10a

### Fixes/Improvements

- registry: Add periodic HTTP transport janitor to proactively close idle connections
  - New helper `StartTransportJanitor(interval)` runs `CloseIdleConnections()` across all cached transports
  - Wired into both `run` and `webhook` commands; stops gracefully on shutdown
  - Default interval: 5m; can be tuned via `REGISTRY_TRANSPORT_JANITOR_INTERVAL` (set `0` to disable)

### Rationale

- Mitigates long-running cluster issues where outbound dials eventually fail with
  `connect: cannot assign requested address` (ephemeral port/SNAT exhaustion) by
  improving connection reuse and cleaning up idle sockets over time.

### Notes

- This complements existing changes: shared transports per registry, tuned
  `MaxConnsPerHost`/idle pools/timeouts, and per‑registry in‑flight caps.

## 2025-09-19 - Release v100.0.9a

### Changes

- registry: Increase default per-attempt timeouts to 60s for tag and manifest fetches
- registry: Make per-attempt timeouts env-tunable via `REGISTRY_TAG_TIMEOUT` and `REGISTRY_MANIFEST_TIMEOUT`
- http transport: Default `ResponseHeaderTimeout` raised to 60s; env-tunable via `REGISTRY_RESPONSE_HEADER_TIMEOUT`
- docs: Document new envs and updated defaults

### Notes

- If your registries are occasionally slow under load, you can set `REGISTRY_TAG_TIMEOUT=90s`, `REGISTRY_MANIFEST_TIMEOUT=90s`, and `REGISTRY_RESPONSE_HEADER_TIMEOUT=90s` to tolerate longer server delays. Consider also lowering concurrency and adding per‑registry rate limits.

## 2025-09-19 - Release v100.0.8a

### Changes

- registry-scanner: JWT auth dedupe and retries stabilized; add metrics nil-guards to avoid panics in tests
- registry-scanner: Fix jittered exponential backoff math for retries
- tests(registry): Add JWT singleflight, different scopes, and retry/backoff tests; reset Prometheus registry per test

### Notes

- This release contains the JWT singleflight + authorizer transport cache improvements; ensure you update the embedded `registry-scanner` module.

## 2025-09-19 - Release v100.0.7

### Fixes

- fix(continuous): initialize Argo client when warm-up is disabled (`--warmup-cache=false`) to prevent panic in `runContinuousOnce`

### Changes

- scheduler: continuous tick cadence set to ~1s (from ~100ms)
- docs: clarify boolean flag usage for `--warmup-cache=false`

### Notes

- If you disable warm-up, continuous starts immediately; each ~1s tick lists and filters apps, then dispatches those due. Unsupported apps are skipped.

## 2025-09-19 - Release v100.0.6a

### Changes

- scheduler(continuous): increase tick cadence from ~100ms to ~1s to reduce log noise and API/list pressure; no change to per-app `--interval` gating
- docs(readme): remove Mermaid diagram; add ASCII architecture; add rate limiting/backpressure section; add phase comparison table (stock vs tuned)

### Notes

- Behavior impact: only the scheduler’s discovery cadence changes; application dispatch still respects `--interval`, in-flight guards, fairness (LRU/fail-first, cooldown, per-repo-cap), and concurrency caps.
- Recommended: if startup delay is undesirable, run with `--warmup-cache=false`.

### Upgrade notes (no really, you MUST read this)

* **Attention**: By default, `argocd-image-updater` now uses the K8s API to retrieve applications, instead of the Argo CD API. Also, it is now recommended to install in the same namespace as Argo CD is running in (`argocd` by default). For existing installations, which are running in a dedicated namespace.

  To retain previous behaviour, set `applications_api: argocd` in `argocd-image-updater-config` ConfigMap before updating. However, it is recommended to move your installation into the `argocd` namespace (or wherever your Argo CD is installed to)

* The permissions for the `argocd-image-updater-sa` ServiceAccount have been adapted to allow read access to resources of type `Secret` and `argoproj.io/Application`

### Bug fixes

* fix: install missing git binary (#148)
* fix: run 'git add' for create files before pushing back (#143)

### New features

* feat: support managing Application CRD using K8S client (#144)
* feat: Allow reuse of Argo CD repo credentials
* feat: Git write-back of parameters (#133)

### Other changes

* refactor: make argocd-image-updater-config volume mapping optional (#145)


## 2025-09-18 - Release v100.0.5a

### Fixes

- fix(git): Prevent panic in batched writer when `GetCreds` is nil or write-back method is not Git
  - Only enqueue batched writes when `wbc.Method == git`
  - Guard in `repoWriter.commitBatch` for missing `GetCreds` (skip with log)

### Tests

- test(git): Strengthen batched writer test to set `Method: WriteBackGit` and provide `GetCreds` stub, so missing-GetCreds would fail tests

### Notes

- No flags or defaults changed; safe upgrade from v100.0.4a

## 2025-09-18 - Release v100.0.4a

### Changes

- test(git): Add unit test verifying batched writer flushes per-branch (monorepo safety)
- fix(git): Guard `getWriteBackBranch` against nil Application source
- docs: Clarify `--max-concurrency=0` (auto) in README quick reference

### Notes

- All existing tests pass. No changes to defaults or flags.

## 2025-09-18 - Release v100.0.3a

### Highlights

- Continuous mode: per-app scheduling with independent timers (no full-cycle waits)
- Auto concurrency: `--max-concurrency=0` computes workers from CPUs/apps
- Robust registry auth and I/O: singleflight + retries with backoff on `/jwt/auth`, tag and manifest operations
- Safer connection handling: transport reuse, tuned timeouts, per‑registry in‑flight caps
- Git efficiency: per‑repo batched writer + retries
- Deep metrics: apps, cycles, registry, JWT

### New features

- feat(mode): `--mode=continuous` (default remains `cycle`)
- feat(concurrency): `--max-concurrency=0` for auto sizing
- feat(schedule): LRU / fail-first with `--schedule`; fairness with `--per-repo-cap`, `--cooldown`
- feat(auth): JWT `/jwt/auth` retries with backoff (singleflight dedupe)
  - Env: `REGISTRY_JWT_ATTEMPTS` (default 7), `REGISTRY_JWT_RETRY_BASE` (200ms), `REGISTRY_JWT_RETRY_MAX` (3s)
- feat(metrics): Per-application timings and state
  - `argocd_image_updater_application_update_duration_seconds{application}`
  - `argocd_image_updater_application_last_attempt_timestamp{application}`
  - `argocd_image_updater_application_last_success_timestamp{application}`
  - `argocd_image_updater_images_considered_total{application}`
  - `argocd_image_updater_images_skipped_total{application}`
  - `argocd_image_updater_scheduler_skipped_total{reason}`
- feat(metrics): Cycle timing
  - `argocd_image_updater_update_cycle_duration_seconds`
  - `argocd_image_updater_update_cycle_last_end_timestamp`
- feat(metrics): Registry visibility
  - `argocd_image_updater_registry_in_flight_requests{registry}`
  - `argocd_image_updater_registry_request_duration_seconds{registry}`
  - `argocd_image_updater_registry_http_status_total{registry,code}`
  - `argocd_image_updater_registry_request_retries_total{registry,op}`
  - `argocd_image_updater_registry_errors_total{registry,kind}`
- feat(metrics): Singleflight effectiveness
  - `argocd_image_updater_singleflight_leaders_total{kind}`
  - `argocd_image_updater_singleflight_followers_total{kind}`
- feat(metrics): JWT visibility
  - `argocd_image_updater_registry_jwt_auth_requests_total{registry,service,scope}`
  - `argocd_image_updater_registry_jwt_auth_errors_total{registry,service,scope,reason}`
  - `argocd_image_updater_registry_jwt_auth_duration_seconds{registry,service,scope}`
  - `argocd_image_updater_registry_jwt_token_ttl_seconds{registry,service,scope}`

### Improvements

- perf(registry): HTTP transport reuse; tuned `MaxIdleConns`, `MaxIdleConnsPerHost`, `MaxConnsPerHost`; response and handshake timeouts
- perf(registry): Per‑registry in‑flight cap to prevent connection storms
- resiliency(registry): Jittered retries for tags/manifests; `/jwt/auth` retries with backoff
- perf(git): Batched per‑repo writer; retries for fetch/shallow-fetch/push
- sched: Fairness via LRU/fail-first, cooldown, and per-repo caps

### Defaults enabled (no flags)

- Transport reuse and tuned timeouts
- Per‑registry in‑flight cap (default 15)
- Authorizer cache per (registry, repo)
- Singleflight on tags, manifests, and `/jwt/auth`
- Retries: tags/manifests (3x), JWT auth (defaults above)
- Git retries (env-overridable); Batched writer (disable via `GIT_BATCH_DISABLE=true`)

### Docs

- docs(install): Performance flags and defaults (continuous mode, auto concurrency, JWT retry envs)
- docs(metrics): Expanded metrics section

### Tests

- test: Unit tests for transport caching, metrics wrappers, continuous scheduler basics, and end-to-end build

### Known issues

- Under very high concurrency and bursty load, upstream registry/SNAT limits may still cause intermittent timeouts. The new caps, retries, and singleflight significantly reduce impact; tune per‑registry limits and consider HTTP/2 where available.

## 2025-09-17 - Release v99.9.9 - 66de072

### New features

* feat: Reuse HTTP transports for registries with keep-alives and timeouts
* feat: Initialize registry refresh-token map to enable token reuse
* feat: Add Makefile `DOCKER` variable to support `podman`

### Improvements

* perf: Cache transports per registry+TLS mode; add sensible connection/timeouts
* resiliency: Retry/backoff for registry tag listing
* resiliency: Retry/backoff for git fetch/shallow-fetch/push during write-back

### Tests/Docs

* test: Add unit tests for transport caching and token map init
* docs: Requirements/notes updates

### Upgrade notes

* None

### Bug fixes

* None

### Bugs

* Under very high concurrency (300–500) after 2–3 hours, nodes may hit ephemeral port exhaustion causing registry dials to fail:

    Example error observed:

    `dial tcp 10.2.163.141:5000: connect: cannot assign requested address`

    Notes:
    - This typically manifests across all registries simultaneously under heavy outbound connection churn.
    - Root cause is excessive parallel dials combined with short‑lived connections (TIME_WAIT buildup), not a specific registry outage.
    - Mitigations available in v100.0.0a: larger keep‑alive pools, lower MaxConnsPerHost, and ability to close idle on cache clear. Operational mitigations: reduce updater concurrency and/or per‑registry limits (e.g., 500→250; 50 rps→20–30 rps) while investigating.

    Details:
    - Old ports are “released” only after TIME_WAIT (2MSL). With HTTP/1.1 and big bursts, you create more concurrent outbound sockets than the ephemeral range can recycle before TIME_WAIT expires, so you hit “cannot assign requested address” even though old sockets eventually close.
    - Why it still happens under 250/100 RPS:
      - Each new dial consumes a unique local ephemeral port to the same dst tuple. TIME_WAIT lasts ~60–120s (kernel dependent). Bursty concurrency + short interval means you outpace reuse.
      - Go HTTP/1.1 doesn’t pipeline; reuse works only if there’s an idle kept‑alive socket. If many goroutines need sockets at once, you dial anyway.
      - Often compounded by SNAT limits at the node (Kubernetes egress): per‑dst NAT port cap can exhaust even faster.
    - How to confirm quickly:
      - Check TIME_WAIT to the registry IP:port: `ss -antp | grep :5000 | grep TIME_WAIT | wc -l`
      - Check ephemeral range: `sysctl net.ipv4.ip_local_port_range`
      - In Kubernetes, inspect node SNAT usage (some clouds cap SNAT ports per node/destination).
    - What fixes it (software‑side, regardless of kernel/NAT tuning):
      - Add a hard per‑registry in‑flight cap (e.g., 10–15) so requests queue instead of dialing new sockets.
      - Lower `MaxConnsPerHost` further (e.g., 15). Keep large idle pools to maximize reuse.
      - Add jitter to scheduling (avoid synchronized bursts); consider 30s interval over 15s.
      - If the registry supports HTTP/2 over TLS, H2 multiplexing drastically reduces sockets.

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
