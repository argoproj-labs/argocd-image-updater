# Argo CD Image Updater

![Integration tests](https://github.com/argoproj-labs/argocd-image-updater/workflows/Integration%20tests/badge.svg?branch=master&event=push)
[![Documentation Status](https://readthedocs.org/projects/argocd-image-updater/badge/?version=latest)](https://argocd-image-updater.readthedocs.io/en/latest/?badge=latest)
[![codecov](https://codecov.io/gh/argoproj-labs/argocd-image-updater/branch/master/graph/badge.svg)](https://codecov.io/gh/argoproj-labs/argocd-image-updater)
[![Go Report Card](https://goreportcard.com/badge/github.com/argoproj-labs/argocd-image-updater)](https://goreportcard.com/report/github.com/argoproj-labs/argocd-image-updater)

## Introduction

Argo CD Image Updater is a tool to automatically update the container
images of Kubernetes workloads which are managed by Argo CD. In a nutshell,
it will track image versions specified by annotations on the Argo CD
Application resources and update them by setting parameter overrides using
the Argo CD API.

Currently it will only work with applications that are built using *Kustomize*
or *Helm* tooling. Applications built from plain YAML or custom tools are not
supported yet (and maybe never will). 

## Documentation

Read
[the documentation](https://argocd-image-updater.readthedocs.io/en/stable/)
for more information on how to setup and run Argo CD Image Updater and to get
known to its features and limitations.

Above URL points to the documentation for the current release. If you are
interested in documentation of upcoming features, check out the
[the latest documentation](https://argocd-image-updater.readthedocs.io/en/latest/)
which is up-to-date with the master branch.

## Current status

Argo CD Image Updater is under active development. We would not recommend it
yet for *critical* production workloads, but feel free to give it a spin.

We're very interested in feedback on usability and the user experience as well
as in bug discoveries and enhancement requests.

**Important note:** Until the first stable version (i.e. `v1.0`) is released,
breaking changes between the releases must be expected. We will do our best
to indicate all breaking changes (and how to un-break them) in the
[Changelog](CHANGELOG.md)

## Contributing

You are welcome to contribute to this project by means of raising issues for
bugs, sending & discussing enhancement ideas or by contributing code via pull
requests.

In any case, please be sure that you have read & understood the currently known
design limitations before raising issues.

Also, if you want to contribute code, please make sure that your code

* has its functionality covered by unit tests (coverage goal is 80%),
* is correctly linted,
* is well commented,
* and last but not least is compatible with our license and CLA

Please note that in the current early phase of development, the code base is
a fast moving target and lots of refactoring will happen constantly.

## License

`argocd-image-updater` is open source software, released under the
[Apache 2.0 license](https://www.apache.org/licenses/LICENSE-2.0)

## Things that are planned (roadmap)

The following things are on the roadmap until the `v1.0` release

* [ ] Extend Argo CD functionality to be able to update images for other types
  of applications.

* [x] Extend Argo CD functionality to write back to Git

* [ ] Provide web hook support to trigger update check for a given image

* [x] Use concurrency for updating multiple applications at once

* [x] Improve error handling

* [x] Support for image tags with i.e. Git commit SHAs

For more details, check out the
[v1.0.0 milestone](https://github.com/argoproj-labs/argocd-image-updater/milestone/1)

## Frequently asked questions

**Does it write back the changes to Git?**

We're happy to announce that as of `v0.9.0` and Argo CD `v1.9.0`, Argo CD
Image Updater is able to commit changes to Git. It will not modify your
application's manifests, but instead writes
[Parameter Overrides](https://argoproj.github.io/argo-cd/user-guide/parameters/#store-overrides-in-git)
to the repository.

We think that this is a good compromise between functionality (have everything
in Git) and ease-of-use (minimize conflicts).

**Are there plans to extend functionality beyond Kustomize or Helm?**

Not yet, since we are dependent upon what functionality Argo CD provides for
these types of applications.

**Will it ever be fully integrated with Argo CD?**

In the current form, probably not. If there is community demand for it, let's
see how we can make this happen.

There is [an open proposal](https://github.com/argoproj/argo-cd/issues/7385) to migrate this project into the `argoproj` org (out
of the `argoproj-labs` org) and include it in the installation of Argo CD.

## Engineering notes (recent changes)

- Registry client hardening
  - HTTP transport reuse per registry with sane timeouts (keep-alives, capped TLS and response header timeouts) to cut connection churn under load.
  - Singleflight-style deduplication and jittered retries (tags, manifests) with per-attempt deadlines to avoid thundering herds and reduce /jwt/auth pressure.

- Git write-back throughput
  - Per-repo serialization to eliminate races in monorepos, plus a batched writer that coalesces multiple intents into a single commit/push per repo/branch.
  - Multi-branch grouping: intents for different write branches never mix; each branch flushes independently.
  - Logs reflect queued writes: look for "Queuing N parameter update(s) … (git write pending)" and the subsequent commit/push logs.

- Scheduling and fairness
  - Optional scheduler flags to prioritize apps: `--schedule` (default|lru|fail-first), `--cooldown` (deprioritize recently successful apps), and `--per-repo-cap` (cap updates per repo per cycle).
  - Goal: prevent a hot monorepo from starving others while keeping high concurrency.

- Operational guidance
  - Concurrency: set `--max-concurrency` roughly ≥ number of active repos; monorepos serialize on their writer, others proceed in parallel.
  - Registry RPS: tune `limit` in `registries.conf` (e.g., 30–50 RPS) and monitor latency/429s.
  - Monorepos: prefer per-app write branches or rely on batching to reduce fetch/commit/push churn.

Flags added
- `--schedule` (env: IMAGE_UPDATER_SCHEDULE): default|lru|fail-first
- `--cooldown` (env: IMAGE_UPDATER_COOLDOWN): duration (e.g., 30s)
- `--per-repo-cap` (env: IMAGE_UPDATER_PER_REPO_CAP): integer (0 = unlimited)

Notes
- For tests or legacy behavior, set `GIT_BATCH_DISABLE=true` to perform immediate (non-batched) write-back.

## Runtime limits and tunables (quick reference)

- Max concurrency: `--max-concurrency` (env `IMAGE_UPDATER_MAX_CONCURRENCY`), default 10; set `0` for auto sizing
- Interval: `--interval` (env `IMAGE_UPDATER_INTERVAL`), default 2m
- Scheduler: `--schedule` (env `IMAGE_UPDATER_SCHEDULE`), default `default` (also `lru|fail-first`)
- Cooldown: `--cooldown` (env `IMAGE_UPDATER_COOLDOWN`), default 0
- Per-repo cap: `--per-repo-cap` (env `IMAGE_UPDATER_PER_REPO_CAP`), default 0
- Git retries (env): `ARGOCD_GIT_ATTEMPTS_COUNT`=3, `ARGOCD_GIT_RETRY_DURATION`=500ms, `ARGOCD_GIT_RETRY_MAX_DURATION`=10s, `ARGOCD_GIT_RETRY_FACTOR`=2
- Registry rate limit: `limit` in `registries.conf` per registry, default 20 rps if unspecified
- HTTP transport (per registry, defaults): MaxIdleConns=1000, MaxIdleConnsPerHost=200, MaxConnsPerHost=30, IdleConnTimeout=90s, TLSHandshakeTimeout=10s, ResponseHeaderTimeout=60s, ExpectContinueTimeout=1s, HTTP/2 on HTTPS via ALPN

### What each tunable does and affects

- max-concurrency (env: `IMAGE_UPDATER_MAX_CONCURRENCY`)
  - What: Number of parallel application update workers.
  - Affects: CPU, concurrent registry and Git load. Higher = faster coverage but more burst pressure.
  - Guidance: 100–250 for large fleets; raise cautiously per registry/Git capacity.

- interval (env: `IMAGE_UPDATER_INTERVAL`)
  - What: Delay between full update cycles (0 = run once).
  - Affects: How often apps are reconsidered; shorter intervals increase burstiness and port churn.
  - Guidance: 30–60s is a good balance; 15s can cause synchronized spikes.

- schedule (env: `IMAGE_UPDATER_SCHEDULE`)
  - What: Processing order: `default` | `lru` | `fail-first`.
  - Affects: Fairness and recovery. `lru` prioritizes least-recently updated; `fail-first` attacks recent failures first.

- cooldown (env: `IMAGE_UPDATER_COOLDOWN`)
  - What: Deprioritizes apps updated within this duration.
  - Affects: Reduces thrash on “hot” apps; spreads work evenly.

- per-repo-cap (env: `IMAGE_UPDATER_PER_REPO_CAP`)
  - What: Max apps per repository processed per cycle (0 = unlimited).
  - Affects: Prevents a monorepo from monopolizing a cycle; improves fleet fairness.

- Git retry env (`ARGOCD_GIT_ATTEMPTS_COUNT`, `ARGOCD_GIT_RETRY_DURATION`, `ARGOCD_GIT_RETRY_MAX_DURATION`, `ARGOCD_GIT_RETRY_FACTOR`)
  - What: Exponential backoff for fetch/shallow-fetch/push.
  - Affects: Resilience vs. latency on transient failures.
  - Defaults: attempts=3; base=500ms; max=10s; factor=2.

- registries.conf `limit` (per registry)
  - What: Requests-per-second cap to a registry.
  - Affects: Upstream load and rate-limit avoidance. Higher = faster metadata fetches; too high = 429/timeouts.
  - Guidance: 30–80 RPS typical; tune to registry capacity.

- HTTP transport defaults (per registry)
  - MaxIdleConns / MaxIdleConnsPerHost: Size of keep-alive pools; larger pools reduce new dials/TLS handshakes.
  - MaxConnsPerHost: Cap on parallel sockets to a host; lower values reduce ephemeral port exhaustion.
  - Timeouts: `IdleConnTimeout`, `TLSHandshakeTimeout`, `ResponseHeaderTimeout`, `ExpectContinueTimeout` prevent hangs and free stale resources.
  - HTTP/2 (HTTPS + ALPN): Multiplexes many requests over few sockets, drastically reducing socket count under load.

- Combined effects (scheduler + limits)
  - Higher `--max-concurrency` with `--per-repo-cap` and `--cooldown` improves fleet throughput and fairness while avoiding monorepo starvation.

## Rate limiting and backpressure

This fork adds layered controls to protect upstreams and the process under load:

- Global worker pool
  - Controlled by `--max-concurrency` (or auto with `0`). Limits total concurrent app updates.

- Per-registry request rate (token bucket)
  - Configured via `registries.conf` `limit` per registry (requests/second).
  - Requests beyond the budget are delayed locally to smooth spikes; reduces 429/timeouts.

- Per-registry in-flight cap
  - Socket-level caps via HTTP transport (`MaxConnsPerHost`) plus internal semaphores where applicable.
  - Prevents connection storms and ephemeral port exhaustion.

- Singleflight de-duplication
  - Tags/manifests and JWT auth are de-duplicated. One leader performs the call, followers wait for the result.
  - Cuts redundant upstream traffic during bursts.

- Jittered exponential backoff retries
  - Applied to tags/manifests and JWT auth. Short, bounded retries with jitter to avoid synchronization.

- Git backpressure (batched writer)
  - Per-repo queue serializes commit/push; multiple app intents per branch coalesce into one commit.
  - Retries with backoff for transient fetch/push errors.

- Fair scheduling
  - `--per-repo-cap` limits apps from one repo per cycle; `--cooldown` deprioritizes recently updated apps.

Observability:
- Metrics expose queue lengths, in-flight counts, retry counts, singleflight leader/follower, and durations to tune the above without guesswork.

## ASCII architecture (fork-specific)

The same runtime, depicted in ASCII for environments without Mermaid rendering.

```
                               +-----------------------------------------+
                               |               Scheduler                 |
                               |-----------------------------------------|
  flags: --mode=continuous     |  per-app timers (interval)              |
         --max-concurrency=0   |  auto concurrency sizing                |
         --schedule=lru|fail   |  LRU / Fail-first prioritization        |
         --cooldown=30s        |  cooldown to dampen hot apps            |
         --per-repo-cap=20     |  fairness cap per Git repo per pass     |
                               +--------------------+--------------------+
                                                     |
                                                     v
                                      +--------------+--------------+
                                      |           Worker Pool       |
                                      +--------------+--------------+
                                                     |
                                                     v
                                         +-----------+-----------+
                                         |   Worker (per app)   |
                                         |----------------------|
                                         | 1) Compute images    |
                                         | 2) Registry ops      |
                                         | 3) Patch spec in mem |
                                         | 4a) WriteBack=Git -> |----+
                                         |     enqueue intent   |    |
                                         | 4b) WriteBack=ArgoCD |    |
                                         |     Update via API   |    |
                                         +----------------------+    |
                                                                       |
                                                                       v
                             +-----------------------------------+     |
                             |    Registry Client (per endpoint) |     |
                             |-----------------------------------|     |
                             | Transport cache (keep-alive)      |     |
                             | Sane timeouts, MaxConnsPerHost    |     |
                             | Per-reg in-flight cap (queue)     |     |
                             | Singleflight: tags/manifests      |     |
                             | JWT auth: singleflight + retries  |     |
                             | HTTP/2 over TLS when available    |     |
                             +------------------+----------------+     |
                                                |                      |
                                                v                      |
                                   +------------+-----------+          |
                                   |  Remote registry/API   |          |
                                   +------------------------+          |
                                                                       |
                                                                       v
                 +-------------------------------------------------------------+
                 |           Per-repo Batched Git Writer                      |
                 |-------------------------------------------------------------|
                 | intent queue (repo)  ->  group by branch  ->  commitBatch  |
                 | fetch/checkout/commit/push (retries/backoff)                |
                 +----------------------------+--------------------------------+
                                              |
                                              v
                                       +------+------+
                                       |    Remote   |
                                       |     Git     |
                                       +-------------+

Observability:
- Metrics: app timings (last attempt/success, durations), cycle duration, registry in-flight/duration/status/retries/errors,
  JWT auth (requests/errors/duration/TTL), singleflight leader/follower counts.
- Logs: startup settings; per-app "continuous: start/finished"; queued write-backs; Git and registry error details.
```

### Phase comparison: stock vs our tuned configuration

| Phase | Stock defaults (cycle mode, basic concurrency) | Tuned configuration (continuous, auto concurrency, LRU, cooldown, per-repo-cap, singleflight, retries) |
| --- | --- | --- |
| Startup | Minimal logging; default transports; limited tuning | Logs full settings; shared transports with timeouts; metrics/health; optional warmup |
| Scheduling | Global pass every `--interval`; fixed concurrency | Lightweight pass ~1s; per-app due check against `--interval`; auto concurrency sizing |
| Discovery/filter | List apps every pass; warn on unsupported each pass | Same listing; will throttle/dedupe repeated unsupported warnings; same filters |
| Prioritization | Default order | LRU or Fail-first; cooldown deprioritizes recent successes; per-repo-cap fairness |
| Dispatch | Semaphore up to `--max-concurrency` | Same guard; plus per-app in-flight guard to avoid double dispatch in continuous |
| Registry IO | Direct calls; limited retry semantics | Per-reg RPS limiter and in-flight cap; singleflight for tags/manifests and JWT; jittered backoff retries; shared transports; HTTP/2 |
| Update decision | Compare live vs candidate; may skip | Same logic, but less flap due to fairness/cooldown |
| Write-back | Immediate Git per app (can thrash in monorepos) | Per-repo batched writer; group by branch; one commit/push per batch; retries |
| Non-Git write-back | ArgoCD `UpdateSpec` | Same, with conflict-retry backoff |
| Observability | Basic metrics/logs | Expanded metrics (JWT, singleflight, durations); per-app continuous start/finish logs; queue and retry metrics |
