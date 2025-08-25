## Command "run"

### Synopsis

`argocd-image-updater run [flags]`

### Description

Runs the Argo CD Image Updater, possibly in an endless loop with a set of options. 

### Flags

**--application-namespace *namespace***

Specifies the Kubernetes namespace in which Argo CD Image Updater will manage Argo CD Applications when using the Kubernetes-based Application API. By default, applications in all namespaces are considered. This flag can be used to limit scope to a single namespace for performance, security, or organizational reasons.

**-argocd-namespace *namespace***

namespace where ArgoCD runs in (current namespace by default)

**--disable-kube-events**

Disable kubernetes events

Can also be set with the *IMAGE_UPDATER_KUBE_EVENTS* environment variable.

**--dry-run**

If this flag is set, Argo CD Image Updater won't actually perform any changes
to workloads it found in need for upgrade.

**--docker-webhook-secret *secret***

Secret for validating Docker Hub webhooks.

**--enable-webhook *enabled***

Enable webhook server for receiving registry events.

**--ghcr-webhook-secret *secret***

Secret for validating GitHub container registry webhooks.

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

**--health-port *port***

Specifies the local port to bind the health server to. The health server is
used to provide health and readiness probes when running as K8s workload.
Use value *0* for *port* to disable launching the health server.

**-h, --help**

help for run

**--interval *duration***

Sets the interval for checking whether there are new images available to
*duration*. *duration* must be given as a valid duration identifier with
a unit suffix, i.e. `2m` for 2 minutes or `30s` for 30 seconds. If no unit
is given, milliseconds will be assumed. If set to `0`, ArgoCD Image Updater
will exit after the first run, effectively disabling the interval. Default
value is `2m0s`.

Can also be set using the *IMAGE_UPDATER_INTERVAL* environment variable.
The `--interval` flag takes precedence over the `IMAGE_UPDATER_INTERVAL` environment variable.

The order of precedence for determining the update interval is as follows:

1.  **`--interval` flag:** If the `--interval` command-line flag is provided, its value will be used.
2.  **`IMAGE_UPDATER_INTERVAL` environment variable:** If the `--interval` flag is not set, the value of the `IMAGE_UPDATER_INTERVAL` environment variable will be used.
3.  **Default value:** If neither the `--interval` flag nor the `IMAGE_UPDATER_INTERVAL` environment variable is set, the default value will be used.

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

**--max-concurrent-reconciles *number***

Process a maximum of *number* ImageUpdater custom resources concurrently. 
This controls how many ImageUpdater CRs can be reconciled simultaneously by the controller. 
To disable concurrent reconciliation processing, specify a number of 1. 
Higher values may improve throughput but could increase resource usage and API load.

Can also be set using the *MAX_CONCURRENT_RECONCILES* environment variable.

**--metrics-port *port***

port to start the metrics server on, 0 to disable (default 8081)

**--once**

A shortcut for specifying `--interval 0 --health-port 0`. If given,
Argo CD Image Updater will exit after the first update cycle.

**--quay-webhook-secret *secret***

Secret for validating Quay webhooks.

**--registries-conf-path *path***

Load the registry configuration from file at *path*. Defaults to the path
`/app/config/registries.conf`. If no configuration should be loaded, and the
default configuration should be used instead, specify the empty string, i.e.
`--registries-conf-path=""`.

**--warmup-cache**

whether to perform a cache warm-up on startup (default true)

**--webhook-port *port***

Port to listen on for webhook events (default 8082)

[label selector syntax]: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors
