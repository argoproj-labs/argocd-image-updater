# Getting Started

## Installation methods

It is recommended to run Argo CD Image Updater in the same Kubernetes namespace
cluster that Argo CD is running in, however, this is not a requirement. In fact,
it is not even a requirement to run Argo CD Image Updater within a Kubernetes
cluster or with access to any Kubernetes cluster at all.

However, some features might not work without accessing Kubernetes, and it is
strongly advised to use the first installation method.

## <a name="install-kubernetes"></a>Method 1: Installing as Kubernetes workload in Argo CD namespace

The most straightforward way to run the image updater is to install it as a Kubernetes workload into the namespace where
Argo CD is running. Don't worry, without any configuration, it will not start messing with your workloads yet.

!!!note
    We also provide a Kustomize base in addition to the plain Kubernetes YAML
    manifests. You can use it as remote base and create overlays with your
    configuration on top of it. The remote base's URL is
    `https://github.com/argoproj-labs/argocd-image-updater/manifests/base`. 
    You can view the manifests [here](https://github.com/argoproj-labs/argocd-image-updater/tree/stable/manifests/base)

### Apply the installation manifests

```shell
kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj-labs/argocd-image-updater/stable/manifests/install.yaml
```

!!! warning
    The installation manifests include `ClusterRoleBinding` resources that reference `argocd` namespace. If you are installing Argo CD into a different
    namespace then make sure to update the namespace reference.

!!!note "A word on high availability"
    It is not advised to run multiple replicas of the same Argo CD Image Updater
    instance. Just leave the number of replicas at 1, otherwise weird side
    effects could occur.

### Configure the desired log level

While this step is optional, we recommend to set the log level explicitly.
During your first steps with the Argo CD Image Updater, a more verbose logging
can help greatly in troubleshooting things.

Edit the `argocd-image-updater-config` ConfigMap and add the following keys
(the values are dependent upon your environment)

```yaml
data:
  # log.level can be one of trace, debug, info, warn or error
  log.level: debug
```

If you omit the `log.level` setting, the default `info` level will be used.

## Method 2: Connect using Argo CD API Server

If you are unable to install Argo CD Image Updater into the same Kubernetes
cluster you can configure it to use the API of your Argo CD installation.

If you chose to install the Argo CD Image Updater outside of the cluster where
Argo CD is running in, the API must be exposed externally (i.e. using Ingress).
If you have network policies in place, make sure that Argo CD Image Updater will
be allowed to communicate with the Argo CD API, which is usually the service
`argocd-server` in namespace `argocd` on port 443 and port 80.

### Apply the manifests

First, create a namespace and apply the manifests to your cluster

```shell
kubectl create namespace argocd-image-updater
kubectl apply -n argocd-image-updater -f https://raw.githubusercontent.com/argoproj-labs/argocd-image-updater/stable/manifests/install.yaml
```

!!! warning
    The installation manifests include `ClusterRoleBinding` resources that reference `argocd` namespace. If you are installing Argo CD into a different
    namespace then make sure to update the namespace reference.

!!!note "A word on high availability"
    It is not advised to run multiple replicas of the same Argo CD Image Updater
    instance. Just leave the number of replicas at 1, otherwise weird side
    effects could occur.

### Create a local user within Argo CD

Argo CD Image Updater needs credential for accessing the Argo CD API. Using a
[local user](https://argoproj.github.io/argo-cd/operator-manual/user-management/)
is recommended, but a *project token* will work as well (although, this will
limit updating to the applications of the given project obviously).

Let's use an account named `image-updater` with appropriate API permissions.

Add the following user definition to `argocd-cm`:

```yaml
data:
  # ...
  accounts.image-updater: apiKey
```

Now, you will need to create an access token for this user, which can be either
done using the CLI or the Web UI. The following CLI command will create a named
token for the user and print it to the console:

```shell
argocd account generate-token --account image-updater --id image-updater
```

Copy the token's value somewhere, you will need it later on.

### Granting RBAC permissions in Argo CD

The technical user `image-updater` we have configured in the previous step now
needs appropriate RBAC permissions within Argo CD. Argo CD Image Updater needs
the `update` and `get` permissions on the applications you want to manage.

A most basic version that grants `get` and `update` permissions on all of the
applications managed by Argo CD might look as follows:

```text
p, role:image-updater, applications, get, */*, allow
p, role:image-updater, applications, update, */*, allow
g, image-updater, role:image-updater
```

You might want to strip that down to apps in a specific project, or to specific
apps, however.

Put the RBAC permissions to Argo CD's `argocd-rbac-cm` ConfigMap and Argo CD will
pick them up automatically.

### Configure Argo CD endpoint

If you run Argo CD Image Updater in another cluster than Argo CD, or if your
Argo CD installation is not in namespace `argocd` or if you use a default or
otherwise self-signed TLS certificate for Argo CD API endpoint, you probably
need to divert from the default connection values.

Edit the `argocd-image-updater-config` ConfigMap and add the following keys
(the values are dependent upon your environment)

```yaml
data:
  applications_api: argocd
  # The address of Argo CD API endpoint - defaults to argocd-server.argocd
  argocd.server_addr: <FQDN or IP of your Argo CD server>
  # Whether to use GRPC-web protocol instead of GRPC over HTTP/2
  argocd.grpc_web: "true"
  # Whether to ignore invalid TLS cert from Argo CD API endpoint
  argocd.insecure: "false"
  # Whether to use plain text connection (http) instead of TLS (https)
  argocd.plaintext: "false"
```

After changing values in the ConfigMap, Argo CD Image Updater needs to be
restarted for the changes to take effect, i.e.

```shell
kubectl -n argocd-image-updater rollout restart deployment argocd-image-updater
```

### Configure API access token secret

When installed from the manifests into a Kubernetes cluster, the Argo CD Image
Updater reads the token required for accessing Argo CD API from an environment
variable named `ARGOCD_TOKEN`, which is set from a field named
`argocd.token` in a secret named `argocd-image-updater-secret`.

The value for `argocd.token` should be set to the *base64 encoded* value of the
access token you have generated above. As a short-cut, you can use generate
secret with `kubectl` and apply it over the existing resource:

```shell
kubectl create secret generic argocd-image-updater-secret \
  --from-literal argocd.token=$YOUR_TOKEN --dry-run -o yaml |
  kubectl -n argocd-image-updater apply -f -
```

You must restart the `argocd-image-updater` pod after such a change, i.e run

```shell
kubectl -n argocd-image-updater rollout restart deployment argocd-image-updater
```

Or alternatively, simply delete the running pod to have it recreated by
Kubernetes automatically.

## Running locally

As long as you have access to the Argo CD API and your Kubernetes cluster from
your workstation, running Argo CD Image Updater is simple. Make sure that you
have your API token noted and that your Kubernetes client configuration points
to the correct K8s cluster.

Grab the binary (it does not have any external dependencies) and run:

```bash
export ARGOCD_TOKEN=<yourtoken>
./argocd-image-updater run \
  --kubeconfig ~/.kube/config
  --once
```

or use `--applications-api` flag if you prefer to connect using Argo CD API

```bash
export ARGOCD_TOKEN=<yourtoken>
./argocd-image-updater run \
  --kubeconfig ~/.kube/config
  --applications-api argocd
  --argocd-server-addr argo-cd.example.com
  --once
```

Note: The `--once` flag disables the health server and the check interval, so
the tool will not regularly check for updates but exit after the first run.

Check `argocd-image-updater --help` for a list of valid command line flags, or
consult the appropriate section of the documentation.

## Running multiple instances

Generally, multiple instances of Argo CD Image Updater can be run within the same
Kubernetes cluster, however they should not operate on the same set of
applications. This allows for multiple application teams to manage their own set
of applications.

If opting for such an approach, you should make sure that:

* Each instance of Argo CD Image Updater runs in its own namespace
* Each instance has a dedicated user in Argo CD, with dedicated RBAC permissions
* RBAC permissions are set-up so that instances cannot interfere with each
  others managed resources

## Metrics

Starting with v0.8.0, Argo CD Image Updater exports Prometheus-compatible
metrics on a dedicated endpoint, which by default listens on TCP port 8081
and serves data from `/metrics` path. This endpoint is exposed by a service
named `argocd-image-updater` on a port named `metrics`.

The following metrics are being made available:

* Number of applications processed (i.e. those with an annotation)

    * `argocd_image_updater_applications_watched_total`

* Number of images watched for new tags

    * `argocd_image_updater_images_watched_total`

* Number of images updated (successful and failed)

    * `argocd_image_updater_images_updated_total`
    * `argocd_image_updater_images_errors_total`

* Number of requests to Argo CD API (successful and failed)

    * `argocd_image_updater_argocd_api_requests_total`
    * `argocd_image_updater_argocd_api_errors_total`

* Number of requests to K8s API (successful and failed)

    * `argocd_image_updater_k8s_api_requests_total`
    * `argocd_image_updater_k8s_api_errors_total`

* Number of requests to the container registries (successful and failed)

    * `argocd_image_updater_registry_requests_total`
    * `argocd_image_updater_registry_requests_failed_total`

### Additional metrics (performance and troubleshooting)

* Per-application timings and state
    * `argocd_image_updater_application_update_duration_seconds{application}`
    * `argocd_image_updater_application_last_attempt_timestamp{application}`
    * `argocd_image_updater_application_last_success_timestamp{application}`
    * `argocd_image_updater_images_considered_total{application}`
    * `argocd_image_updater_images_skipped_total{application}`
    * `argocd_image_updater_scheduler_skipped_total{reason}` (e.g., cooldown, per-repo-cap)

* Update cycle timing
    * `argocd_image_updater_update_cycle_duration_seconds`
    * `argocd_image_updater_update_cycle_last_end_timestamp`

* Registry request health
    * `argocd_image_updater_registry_in_flight_requests{registry}`
    * `argocd_image_updater_registry_request_duration_seconds{registry}`
    * `argocd_image_updater_registry_http_status_total{registry,code}`
    * `argocd_image_updater_registry_request_retries_total{registry,op}` (auth|tags|manifest)
    * `argocd_image_updater_registry_errors_total{registry,kind}` (timeout, dial_error, auth_error, 429, 5xx, ctx_deadline)

* Singleflight (deduplication effectiveness)
    * `argocd_image_updater_singleflight_leaders_total{kind}` (tags|manifest)
    * `argocd_image_updater_singleflight_followers_total{kind}`

Notes:
* Metrics are exposed at `/metrics` (see `--metrics-port`, default 8081).
* Labels like `{application}`, `{registry}`, `{repo}`, `{op}`, `{kind}` enable fine-grained dashboards.

A (very) rudimentary example dashboard definition for Grafana is provided
[here](https://github.com/argoproj-labs/argocd-image-updater/tree/master/config)

## Performance flags (recommended)

For large fleets and monorepos, enable continuous scheduling and auto concurrency:

For a complete example of args and environment variables, see the run command reference. We keep one canonical example there to avoid duplication.

See: [Run command examples](./cmd/run.md#example-kubernetes-deployment-args-and-env)

Notes:
- Continuous mode preserves all shared protections (per‑registry in‑flight cap, retries, singleflight, git batching).
- Keep per‑registry rate limits tuned in `registries.conf` to match registry capacity.
- `--per-repo-cap`: maximum apps from the same Git repository processed in one pass. Prevents a single monorepo from monopolizing workers; improves fleet fairness.
- `--cooldown`: deprioritizes apps successfully updated within this duration so other apps get slots first. Reduces thrash on hot apps.
 

### Defaults enabled without flags

The following performance features are ON by default (no flags required):

- HTTP transport reuse with tuned timeouts (keep‑alive pools, sane phase timeouts)
- Per‑registry in‑flight cap (default 15 concurrent requests per registry)
- Authorizer cache per (registry, repo) for bearer/JWT reuse
- Singleflight deduplication for tags, manifests, and /jwt/auth
- Retries: tags/manifests (3 attempts with jitter)
- JWT auth retries (defaults: 7 attempts; see REGISTRY_JWT_* above to tune)
- Git retries (fetch/shallow/push) with sane defaults (env overridable)
- Batched Git writer (coalesces per‑repo writes; disable via GIT_BATCH_DISABLE=true)
- Expanded Prometheus metrics for apps, cycles, registry, JWT
