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

### Flags

**--application-namespace *namespace***

Specifies the Kubernetes namespace in which Argo CD Image Updater will manage Argo CD Applications when using the Kubernetes-based Application API. By default, applications in all namespaces are considered. This flag can be used to limit scope to a single namespace for performance, security, or organizational reasons.

**-argocd-namespace *namespace***

namespace where ArgoCD runs in (current namespace by default)

**--disable-kube-events**

Disable kubernetes events

Can also be set with the *IMAGE_UPDATER_KUBE_EVENTS* environment variable.

**--disable-kubernetes**

If running locally, and you do not have a working connection to any Kubernetes
cluster, this flag will prevent Argo CD Image Updater from creating a client
to interact with Kubernetes. When Kubernetes access is disabled, pull secrets
for images can only be specified from an environment variable.

**--docker-webhook-secret *secret***

Secret for validating Docker Hub webhooks.

**--ghcr-webhook-secret *secret***

Secret for validating GitHub container registry secrets.

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

**--quay-webhook-secret *secret***

Secret for validating Quay webhooks

**--registries-conf-path *path***

Load the registry configuration from file at *path*. Defaults to the path
`/app/config/registries.conf`. If no configuration should be loaded, and the
default configuration should be used instead, specify the empty string, i.e.
`--registries-conf-path=""`.

**--webhook-port *int***

Port to listen on for webhook events (default 8080)

[label selector syntax]: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors
