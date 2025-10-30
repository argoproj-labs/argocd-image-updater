# Argo CD Image Updater Runtime Architecture

## Overview

Argo CD Image Updater operates in two main execution modes:
1. **Cycle Mode** (`--mode=cycle`): Traditional polling mode that runs update cycles at fixed intervals
2. **Continuous Mode** (`--mode=continuous`): Event-driven mode where each application is processed independently on its own schedule

## Execution Flow

### Cycle Mode Flow

```
1. Startup
   ├── Initialize Kubernetes/ArgoCD clients
   ├── Start health/metrics servers (if enabled)
   ├── Start webhook server (if --enable-webhook)
   ├── Warm up cache (if --warmup-cache=true)
   └── Enter main loop

2. Main Loop (every --interval)
   ├── ListApplications() from ArgoCD API
   ├── FilterApplicationsForUpdate() - filter by annotations/patterns
   ├── gcRemovedAppMetrics() - DELETE metrics for apps no longer in list
   ├── orderApplications() - apply scheduling policy (lru/fail-first)
   ├── Process apps concurrently (up to --max-concurrency)
   │   ├── For each app (in parallel):
   │   │   ├── UpdateApplication() - check images, update if needed
   │   │   ├── Record metrics (success/failure timestamps)
   │   │   └── Update application state
   └── Wait for --interval before next cycle
```

### Continuous Mode Flow

```
1. Startup (same as cycle mode)

2. Main Loop (runs continuously, ~1s tick)
   ├── ListApplications() from ArgoCD API
   ├── FilterApplicationsForUpdate() - filter by annotations/patterns
   ├── gcRemovedAppMetrics() - DELETE metrics for apps no longer in list
   ├── For each app:
   │   ├── Check last attempt time
   │   ├── If interval elapsed AND not already in-flight:
   │   │   ├── Launch goroutine to process app
   │   │   ├── Process independently
   │   │   └── Re-schedule next attempt based on --interval
   └── Sleep 1s, repeat
```

## Metrics Garbage Collection

### Problem

When an Argo CD Application is deleted, its Prometheus metrics (like `argocd_image_updater_application_last_success_timestamp{application="deleted-app"}`) remain in memory. These stale metrics can trigger false alerts because:
- The metric series still exists with old timestamps
- Prometheus continues scraping these series
- Alerts based on "no recent success" fire even though the app no longer exists

### Solution

The `gcRemovedAppMetrics()` function runs **at the start of each update cycle** (both cycle and continuous modes). It:

1. **Tracks known applications**: Maintains a snapshot (`knownApps`) of applications seen in the previous cycle
2. **Detects deletions**: Compares current app list against `knownApps` to find apps that disappeared
3. **Deletes stale metrics**: Calls `DeleteAppMetrics()` for each removed app, which removes all per-application metric series:
   - `images_watched_total{application=...}`
   - `images_updated_total{application=...}`
   - `images_errors_total{application=...}`
   - `application_update_duration_seconds{application=...}`
   - `application_last_attempt_timestamp{application=...}`
   - `application_last_success_timestamp{application=...}`
   - `images_considered_total{application=...}`
   - `images_skipped_total{application=...}`
4. **Updates snapshot**: Replaces `knownApps` with the current app list for the next cycle

### When GC Runs

- **Cycle Mode**: Called in `runImageUpdater()` after filtering applications, before processing
- **Continuous Mode**: Called in `runContinuousOnce()` after filtering applications, before scheduling

### Example Timeline

```
Cycle 1:
  Apps: [app1, app2, app3]
  knownApps: {} (empty initially)
  GC: No deletions (knownApps empty, just populate it)
  Result: knownApps = {app1, app2, app3}

Cycle 2:
  Apps: [app1, app3]  // app2 was deleted
  knownApps: {app1, app2, app3}
  GC: Detect app2 missing → DeleteAppMetrics("app2")
  Result: knownApps = {app1, app3}, app2 metrics deleted

Cycle 3:
  Apps: [app1, app3, app4]  // app4 added
  knownApps: {app1, app3}
  GC: No deletions (app4 is new, will get metrics on first update)
  Result: knownApps = {app1, app3, app4}
```

## Thread Safety

- `knownApps` is protected by `knownAppsMu` mutex
- GC runs before concurrent app processing, minimizing lock contention
- `DeleteAppMetrics()` uses Prometheus's built-in thread-safe deletion (best-effort)

## Stability Guarantees

- **No panic on missing apps**: Deleting metrics for non-existent apps is safe (Prometheus handles it gracefully)
- **No race conditions**: GC happens synchronously before concurrent processing starts
- **Memory efficient**: Only tracks app names (string keys), not full application objects
- **Idempotent**: Multiple deletions for the same app are safe

## Testing

Tests verify:
- Metrics are deleted when apps disappear
- Remaining apps' metrics are unaffected
- Deleting non-existent apps doesn't panic
- GC works correctly with multiple apps being added/removed over time

