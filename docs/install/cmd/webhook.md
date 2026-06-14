## Command "webhook"

### Synopsis

`argocd-image-updater webhook [flags]`

### Description

Starts a server that listens for webhook events from container registries. When an event is received, it can trigger an image update check for the affected images.

Supported Registries:

- Docker Hub
- GitHub Container Registry (GHCR)
- Quay
- Harbor
- Aliyun ACR (Alibaba Cloud Container Registry)
- Azure Container Registry (ACR)
- AWS EventBridge (CloudEvents)

### Flags

**--acr-webhook-secret *secret***

Secret for validating Azure ACR webhooks. ACR has no built-in signing, so the
secret is sent as the `Authorization` header value, which you configure on the
webhook with `az acr webhook update --headers "Authorization=<secret>"`.

Can also be set with the `ACR_WEBHOOK_SECRET` environment variable.

**--aliyun-acr-webhook-secret *secret***

Secret for validating Aliyun ACR webhooks.

Can also be set with the `ALIYUN_ACR_WEBHOOK_SECRET` environment variable.

**-argocd-namespace *namespace***

namespace where ArgoCD runs in (current namespace by default)

**--cloudevents-webhook-secret *secret***

Secret for validating CloudEvents webhooks from AWS EventBridge and other CloudEvents sources.

Can also be set with the `CLOUDEVENTS_WEBHOOK_SECRET` environment variable.

**--disable-kube-events**

Disable kubernetes events

Can also be set with the *IMAGE_UPDATER_KUBE_EVENTS* environment variable.

**--disable-tls**

Disable TLS and run the webhook server with plain HTTP. By default, the server
starts with TLS enabled.

Can also be set with the `DISABLE_TLS` environment variable.

**--docker-webhook-secret *secret***

Secret for validating Docker Hub webhooks.

Can also be set with the `DOCKER_WEBHOOK_SECRET` environment variable.

**--enable-http2**

Enable HTTP/2 for the standalone webhook server. Disabled by default.

**--ghcr-webhook-secret *secret***

Secret for validating GitHub container registry secrets.

Can also be set with the `GHCR_WEBHOOK_SECRET` environment variable.

**--git-commit-email *email***

E-Mail address to use for Git commits (default "noreply@argoproj.io")

Can also be set using the *GIT_COMMIT_EMAIL* environment variable.

**--git-commit-message-path *path*** 

Path to a template to use for Git commit messages (default "/app/config/commit.template")

**--git-commit-sign-off**

Whether to sign-off git commits

**--git-commit-signing-key *key***

GnuPG key ID or path to Private SSH Key used to sign the commits

Can also be set using the *GIT_COMMIT_SIGNING_KEY* environment variable.

**--git-commit-signing-method *method***

Method used to sign Git commits ('openpgp' or 'ssh') (default "openpgp")

Can also be set using the *GIT_COMMIT_SIGNING_METHOD* environment variable.

**--git-commit-user *user***

Username to use for Git commits (default "argocd-image-updater")

Can also be set using the *GIT_COMMIT_USER* environment variable.

**--harbor-webhook-secret *secret***

Secret for validating Harbor webhooks

Can also be set with the `HARBOR_WEBHOOK_SECRET` environment variable.

**-h, --help**

help for run

**--kubeconfig *path***

Specify the Kubernetes client config file to use when running outside a
Kubernetes cluster, i.e. `~/.kube/config`. When specified, Argo CD Image
Updater will use the currently active context in the configuration to connect
to the Kubernetes cluster.

**--loglevel *level***

Set the log level to *level*, where *level* can be one of `trace`, `debug`,
`info`, `warn` or `error`.

Can also be set using the *IMAGE_UPDATER_LOGLEVEL* environment variable.

**--max-concurrent-apps *number***

Process a maximum of *number* applications concurrently. To disable concurrent
application processing, specify a number of `1`.

Can also be set using the *MAX_CONCURRENT_APPS* environment variable.

**--max-concurrent-updaters *number***

Process a maximum of *number* ImageUpdater custom resources concurrently.
To disable concurrent processing, specify a number of 1.
Higher values may improve throughput but could increase resource usage and API load.

Can also be set using the *MAX_CONCURRENT_UPDATERS* environment variable.

**--quay-webhook-secret *secret***

Secret for validating Quay webhooks

Can also be set with the `QUAY_WEBHOOK_SECRET` environment variable.

**--registries-conf-path *path***

Load the registry configuration from file at *path*. Defaults to the path
`/app/config/registries.conf`. If no configuration should be loaded, and the
default configuration should be used instead, specify the empty string, i.e.
`--registries-conf-path=""`.

**--tlsciphers *suites***

Colon-separated list of TLS cipher suite names to allow (e.g.
`TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384:TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256`).
Only applies to TLS 1.1 and 1.2 connections; TLS 1.3 cipher suites are not configurable.
Defaults to the Go standard library's secure defaults.

Can also be set with the `TLS_CIPHERS` environment variable.

**--tlsmaxversion *version***

Maximum TLS version to accept. Valid values are `1.1`, `1.2`, and `1.3`. Defaults to `1.3`.

Can also be set with the `TLS_MAX_VERSION` environment variable.

**--tlsminversion *version***

Minimum TLS version to accept. Valid values are `1.1`, `1.2`, and `1.3`. Defaults to `1.3`.
TLS 1.0 is not supported.

Can also be set with the `TLS_MIN_VERSION` environment variable.

**--webhook-port *int***

Port to listen on for webhook events (default 8080)

Can also be set with the `WEBHOOK_PORT` environment variable.

**--webhook-ratelimit-allowed *numRequests***

The number of allowed requests in an hour for webhook rate limiting, setting to 0
means that the rate limiting is disabled.

Can also be set with the `WEBHOOK_RATELIMIT_ALLOWED` environment variable.

**--webhook-require-secret *bool***

When set to `true` (the default), only registry webhook handlers that have a
secret configured are registered. Requests arriving for a registry with no
secret will be rejected. Set to `false` to register all handlers regardless
of whether a secret is present — this disables authentication on those
endpoints and is strongly discouraged in production.

Can also be set with the `WEBHOOK_REQUIRE_SECRET` environment variable.

!!!warning
    Setting `--webhook-require-secret=false` means the webhook endpoint will
    accept unauthenticated requests from any source for registries that have no
    secret configured. Only use this during local development or in a fully
    network-isolated environment.

[label selector syntax]: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors
