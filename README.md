# Argo CD Image Updater - Custom Fork

> **⚠️ Custom Version Notice**  
> This is a **custom fork** of Argo CD Image Updater, enhanced specifically for **continuous mode** operation in large-scale Kubernetes environments. This fork includes performance optimizations, better resource management, and enhanced observability that are not present in the upstream version.

## 🎯 Why This Fork?

This custom version addresses performance and scalability limitations of the stock Argo CD Image Updater for production workloads with many applications:

- **⚡ Continuous Mode**: Independent per-application scheduling instead of batch cycles
- **🔧 Performance Tuning**: Optimized registry connections, retries, and concurrency
- **📊 Enhanced Metrics**: Better observability for debugging and monitoring
- **🛡️ Resource Management**: Prevents port exhaustion, connection leaks, and memory issues
- **🚀 Production Ready**: Battle-tested in large fleets with hundreds of applications

## 📈 Architecture Comparison

### Stock Version (Cycle Mode)
```
┌─────────────────────────────────────────────────────────┐
│                  Cycle Mode (Stock)                      │
├─────────────────────────────────────────────────────────┤
│                                                           │
│  ┌──────────────────────────────────────┐               │
│  │  Every --interval (e.g., 2 minutes)  │               │
│  └──────────────────────────────────────┘               │
│                    │                                      │
│                    ▼                                      │
│  ┌──────────────────────────────────────┐               │
│  │  1. List ALL applications             │               │
│  │  2. Process ALL in parallel           │               │
│  │  3. Wait for ALL to finish            │               │
│  │  4. Sleep until next cycle            │               │
│  └──────────────────────────────────────┘               │
│                                                           │
│  Problems:                                                │
│  ❌ Starvation: Slow apps block fast ones                 │
│  ❌ Thrashing: Hot apps get processed repeatedly          │
│  ❌ Resource spikes: All apps processed at once           │
│  ❌ Poor fairness: No prioritization                      │
└─────────────────────────────────────────────────────────┘
```

### Custom Version (Continuous Mode)
```
┌─────────────────────────────────────────────────────────┐
│              Continuous Mode (This Fork)                 │
├─────────────────────────────────────────────────────────┤
│                                                           │
│  ┌──────────────────────────────────────┐               │
│  │  Every ~1 second (lightweight tick)   │               │
│  └──────────────────────────────────────┘               │
│                    │                                      │
│                    ▼                                      │
│  ┌──────────────────────────────────────┐               │
│  │  For each application:                │               │
│  │  • Check if --interval elapsed         │               │
│  │  • If due: Launch independent worker   │               │
│  │  • Else: Skip until next tick          │               │
│  └──────────────────────────────────────┘               │
│                    │                                      │
│        ┌───────────┴───────────┐                         │
│        ▼                       ▼                         │
│  ┌──────────┐          ┌──────────┐                      │
│  │ App A    │          │ App B    │                      │
│  │ Worker   │          │ Worker   │                      │
│  │ (60s)    │          │ (60s)    │                      │
│  └──────────┘          └──────────┘                      │
│                                                           │
│  Benefits:                                                │
│  ✅ Fairness: Each app on its own schedule               │
│  ✅ Efficiency: No blocking, better resource use          │
│  ✅ Prioritization: LRU/fail-first scheduling            │
│  ✅ Rate limiting: Per-registry caps prevent overload    │
└─────────────────────────────────────────────────────────┘
```

## 🆚 Feature Comparison

| Feature | Stock Version | Custom Fork |
|---------|--------------|-------------|
| **Scheduling** | Batch cycles (all apps together) | Independent per-app scheduling |
| **Concurrency** | Fixed `--max-concurrency` | Auto-scaling + fixed cap option |
| **Prioritization** | None (default order) | LRU, fail-first, cooldown, per-repo-cap |
| **Resource Management** | Basic | HTTP transport janitor, port exhaustion detection |
| **Connection Reuse** | Limited | Shared transports, connection pooling |
| **Retries** | Basic | JWT auth retries, jittered backoff, singleflight |
| **Git Operations** | Per-app commits | Batched per-repo commits |
| **Metrics** | Basic | Expanded (JWT, singleflight, durations, GC) |
| **Health Checks** | Basic | Port exhaustion detection with auto-restart |
| **Webhook** | Supported | Supported + sidecar deployment pattern |

## 🚀 Quick Start

### Kubernetes Deployment Example

This example shows a production-ready configuration using continuous mode with all performance optimizations:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: argocd-image-updater
  namespace: argocd
spec:
  replicas: 1
  template:
    spec:
      containers:
      - name: argocd-image-updater
        image: your-registry/argocd-image-updater:custom
        args:
          - run
          - "--interval"
          - 60s
          - "--max-concurrency"
          - "8"
          - "--match-application-label"
          - argocd-project=my-project
          - "--mode"
          - continuous
          - "--schedule"
          - lru
          - "--warmup-cache=false"
        env:
          # Registry HTTP timeouts (prevent hangs)
          - name: REGISTRY_TLS_HANDSHAKE_TIMEOUT
            value: 30s
          - name: REGISTRY_RESPONSE_HEADER_TIMEOUT
            value: 120s
          
          # JWT authentication retries (for flaky registries)
          - name: REGISTRY_JWT_ATTEMPTS
            value: "10"
          - name: REGISTRY_JWT_RETRY_BASE
            value: 500ms
          - name: REGISTRY_JWT_RETRY_MAX
            value: 5s
          
          # Tag and manifest fetch timeouts
          - name: REGISTRY_TAG_TIMEOUT
            value: 120s
          - name: REGISTRY_MANIFEST_TIMEOUT
            value: 120s
          
          # Connection pooling (prevent port exhaustion)
          - name: REGISTRY_MAX_CONNS_PER_HOST
            value: "10"
          
          # HTTP transport janitor (cleanup idle connections)
          - name: REGISTRY_TRANSPORT_JANITOR_INTERVAL
            value: 5m
          
          # Health check for port exhaustion
          - name: HEALTH_FAIL_ON_PORT_EXHAUSTION
            value: "true"
          - name: PORT_EXHAUSTION_WINDOW
            value: 60s
          - name: PORT_EXHAUSTION_THRESHOLD
            value: "8"
        ports:
        - containerPort: 8080
          name: health
        - containerPort: 8081
          name: metrics
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8080
          initialDelaySeconds: 3
          periodSeconds: 30
          timeoutSeconds: 1
          failureThreshold: 3
        readinessProbe:
          httpGet:
            path: /healthz
            port: 8080
          initialDelaySeconds: 3
          periodSeconds: 10
```

### Key Configuration Explained

**`--mode continuous`**: Enables independent per-application scheduling  
**`--schedule lru`**: Processes least-recently-updated apps first (improves fairness)  
**`--max-concurrency 8`**: Limits parallel workers (adjust based on registry capacity)  
**`--interval 60s`**: Each app checks for updates every 60 seconds  
**`--warmup-cache=false`**: Skips startup cache warmup (faster startup in continuous mode)

**Environment Variables**:
- Registry timeouts prevent hanging requests
- JWT retries handle flaky authentication
- Connection limits prevent port exhaustion
- Transport janitor cleans up idle connections
- Health checks auto-restart on port exhaustion

## 📊 Monitoring & Observability

### Metrics Endpoints

- **Health**: `http://localhost:8080/healthz`
- **Metrics**: `http://localhost:8081/metrics`

### Key Metrics

```prometheus
# Application-level metrics
argocd_image_updater_application_last_success_timestamp{application="app-name"}
argocd_image_updater_application_last_attempt_timestamp{application="app-name"}
argocd_image_updater_application_update_duration_seconds{application="app-name"}

# Registry metrics
argocd_image_updater_registry_in_flight_requests{registry="registry-url"}
argocd_image_updater_registry_request_duration_seconds{registry="registry-url"}
argocd_image_updater_registry_errors_total{registry="registry-url",kind="timeout"}

# JWT authentication metrics
argocd_image_updater_registry_jwt_auth_requests_total{registry="registry-url"}
argocd_image_updater_registry_jwt_auth_errors_total{registry="registry-url"}
```

### Alerts Example

```yaml
- alert: ImageUpdaterPortExhaustion
  expr: up{job="argocd-image-updater"} == 0
  annotations:
    summary: "Image updater pod restarted due to port exhaustion"
    description: "Pod was auto-restarted by health check"

- alert: ImageUpdaterHighErrors
  expr: rate(argocd_image_updater_registry_errors_total[5m]) > 0.1
  annotations:
    summary: "High registry error rate detected"
```

## 🔧 Advanced Features

### Per-Repository Rate Limiting

Prevent a single monorepo from monopolizing workers:

```bash
--per-repo-cap 5  # Max 5 apps from same repo per cycle
```

### Cooldown Period

Deprioritize recently updated apps:

```bash
--cooldown 5m  # Skip apps updated in last 5 minutes
```

### Auto Concurrency

Let the system automatically size concurrency:

```bash
--max-concurrency 0  # 0 = auto (8x CPU count, capped to app count)
```

## 📚 Documentation

- [Runtime Architecture](RUNTIME_ARCHITECTURE.md) - Detailed execution flow
- [Building from Scratch](BUILDING_FROM_SCRATCH.md) - Guide for developers
- [Upstream Documentation](https://argocd-image-updater.readthedocs.io/) - Base features

## 🔄 Differences from Upstream

This fork maintains compatibility with upstream Argo CD Image Updater while adding:

1. **Continuous mode scheduler** - Independent per-app scheduling
2. **HTTP transport janitor** - Prevents connection leaks
3. **Port exhaustion detection** - Auto-restart via health checks
4. **Metrics garbage collection** - Prevents false alerts for deleted apps
5. **Enhanced retry logic** - JWT auth retries with backoff
6. **Batched Git writes** - Coalesces commits in monorepos
7. **Performance tuning** - Shared transports, connection pooling, singleflight

## ⚖️ License

This fork maintains the same [Apache 2.0 license](https://www.apache.org/licenses/LICENSE-2.0) as the upstream project.

## 🙏 Acknowledgments

Based on [argoproj-labs/argocd-image-updater](https://github.com/argoproj-labs/argocd-image-updater), enhanced for production workloads.

---

**Note**: This is a custom fork. If you encounter issues, please check upstream documentation first, as this fork focuses on continuous mode optimizations.
