# Metrics and useful PromQL

This page lists the most useful Prometheus metrics exported by `argocd-image-updater` and ready‑to‑use PromQL queries.

## Core counters and gauges

- Applications watched: `argocd_image_updater_applications_watched_total`
- Images watched: `argocd_image_updater_images_watched_total`
- Images updated: `argocd_image_updater_images_updated_total`
- Image update errors: `argocd_image_updater_images_updated_error_total`
- Registry requests by status: `argocd_image_updater_registry_http_status_total{registry,code}`
- Registry request duration: `argocd_image_updater_registry_request_duration_seconds{registry}` (histogram)
- JWT auth metrics: `argocd_image_updater_registry_jwt_*`
- Per‑app timestamps: `argocd_image_updater_application_last_attempt_timestamp{application}` and `argocd_image_updater_application_last_success_timestamp{application}`

## Which apps are lagging more than 5 minutes?

Minutes since last success (collapsed across replicas):

```promql
((time() - max by (application) (argocd_image_updater_application_last_success_timestamp)) / 60)
```

Filter for > 5 minutes:

```promql
((time() - max by (application) (argocd_image_updater_application_last_success_timestamp)) / 60) > 5
```

Fallback to last attempt if there has never been a success:

```promql
(
  (time() - max by (application) (argocd_image_updater_application_last_success_timestamp))
  or on (application)
  (time() - max by (application) (argocd_image_updater_application_last_attempt_timestamp))
) / 60 > 5
```

Top 20 most lagging apps (minutes):

```promql
topk(20, (time() - max by (application) (argocd_image_updater_application_last_success_timestamp)) / 60)
```

## Update throughput

Total updates and errors per minute:

```promql
sum(increase(argocd_image_updater_images_updated_total[1m]))
sum(increase(argocd_image_updater_images_updated_error_total[1m]))
```

Per‑application updates (5m window):

```promql
sum by (application) (increase(argocd_image_updater_images_updated_total[5m]))
```

## Registry health

Requests vs errors per registry (1m window):

```promql
sum by (registry) (increase(argocd_image_updater_registry_requests_total[1m]))
sum by (registry) (increase(argocd_image_updater_registry_requests_failed_total[1m]))
```

Latency (p50/p90/p99):

```promql
histogram_quantile(0.5, sum by (le, registry) (rate(argocd_image_updater_registry_request_duration_seconds_bucket[5m])))
histogram_quantile(0.9, sum by (le, registry) (rate(argocd_image_updater_registry_request_duration_seconds_bucket[5m])))
histogram_quantile(0.99, sum by (le, registry) (rate(argocd_image_updater_registry_request_duration_seconds_bucket[5m])))
```

HTTP status breakdown per registry:

```promql
sum by (registry, code) (increase(argocd_image_updater_registry_http_status_total[5m]))
```

## JWT auth health

Requests, errors, and durations (5m window):

```promql
sum by (registry, service, scope) (increase(argocd_image_updater_registry_jwt_auth_requests_total[5m]))
sum by (registry, service, scope) (increase(argocd_image_updater_registry_jwt_auth_errors_total[5m]))
```

## Tips

- Use `max by (application)` to deduplicate across multiple replicas.
- Switch Grafana to Table view to list lagging apps with values.
- Consider adding thresholds/alerts for lag > 10–15 minutes.



