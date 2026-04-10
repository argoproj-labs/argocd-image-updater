# Getting Started

!!!warning "Breaking Change: Default watch scope is now namespace-scoped"
    Previously, the controller operated in cluster-wide mode by default, watching
    all namespaces using a `ClusterRole` and `ClusterRoleBinding`. The default is
    now **namespace-scoped**: the controller watches only its own namespace using a
    `Role` and `RoleBinding`. Existing installations that rely on cluster-wide
    watching must explicitly set `--watch-namespaces="*"` and switch to
    `ClusterRole`+`ClusterRoleBinding` RBAC. See
    [Choosing an installation namespace](#choosing-an-installation-namespace) for
    details.

## Installation methods

The Argo CD Image Updater controller **must** be run in the same Kubernetes cluster where your Argo CD `Application` resources are managed. The current controller architecture does not support connecting to a remote Kubernetes cluster to manage applications.

While the `argocd-image-updater` binary can be run locally from your workstation for one-time updates (see `Running locally` section), the standard and supported installation for continuous, automated updates is as a controller inside your cluster.

### Multi-Cluster Environments

In a multi-cluster setup where Argo CD, running on a central control plane cluster (let's call it cluster A), manages `Application` resources that are deployed to another cluster (cluster B), there is one important restriction:

*   The Image Updater controller **must** be installed on cluster A.
*   All `ImageUpdater` custom resources (CRs) must also be created on cluster A.

The controller cannot discover or process `ImageUpdater` CRs created on cluster B.

In short: The Image Updater controller and its `ImageUpdater` CRs must reside with your Argo CD `Application` resources, not with the deployed application workloads.

### Namespace scope

The controller's watch scope is controlled by the `--watch-namespaces` flag (or `IMAGE_UPDATER_WATCH_NAMESPACES` env var) and determines which namespaces it monitors for `ImageUpdater` CRs and Argo CD `Applications`:

| Value             | Scope                           | RBAC required                                |
|-------------------|---------------------------------|----------------------------------------------|
| Not set (default) | Controller's own namespace only | `Role` + `RoleBinding` in that namespace     |
| `"ns1,ns2,..."`   | Listed namespaces only          | `Role` + `RoleBinding` in **each** namespace |
| `"*"`             | All namespaces, cluster-wide    | `ClusterRole` + `ClusterRoleBinding`         |

The default installation (`install.yaml`) uses `Role` and `RoleBinding` scoped to the controller's namespace. For `--watch-namespaces="*"`, replace these with a `ClusterRole` and `ClusterRoleBinding` (see Option 3 below).

### Choosing an installation namespace

#### Option 1: Install into the Argo CD namespace (Recommended)

The simplest approach. The controller runs in the same namespace as Argo CD, so the default scope (own namespace) covers both `ImageUpdater` CRs and `Applications` without any extra configuration.

If Argo CD is running in the `argocd` namespace, use the following command:

!!!note
    We also provide a Kustomize base in addition to the plain Kubernetes YAML
    manifests. You can use it as remote base and create overlays with your
    configuration on top of it. The remote base's URL is
    `https://github.com/argoproj-labs/argocd-image-updater/config/default`.
    You can view the manifests [here](https://github.com/argoproj-labs/argocd-image-updater/tree/stable/config/default)

```shell
kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj-labs/argocd-image-updater/stable/config/install.yaml
```

By default, the controller watches only its own namespace. If you use Argo CD's [Applications in any namespace](https://argo-cd.readthedocs.io/en/stable/operator-manual/app-any-namespace/) feature and have `Application` resources in additional namespaces, configure `--watch-namespaces` and create a `Role` + `RoleBinding` in **each** extra namespace (see the [Namespace scope](#namespace-scope) table above).

#### Option 2: Install into a separate namespace

For better workload isolation, you can install the image updater into its own namespace. This use case requires several manual configuration steps.

Let's assume Argo CD runs in `<argocd_namespace>` and you are installing the image updater in `<updater_namespace>`.

1. **Install the Controller**

    First, create the target namespace and apply the installation manifest.

    ```shell
    kubectl create namespace <updater_namespace>
    kubectl apply -n <updater_namespace> -f https://raw.githubusercontent.com/argoproj-labs/argocd-image-updater/stable/config/install.yaml
    ```

2. **Configure the Argo CD Namespace**

    The controller needs to know where to find Argo CD resources. Edit the `argocd-image-updater-controller` deployment manifest and add the `ARGOCD_NAMESPACE` environment variable to the `argocd-image-updater-controller` container or add `argocd.namespace` key to the ConfigMap `argocd-image-updater-config`, pointing to the namespace where Argo CD is installed.

    Also set `IMAGE_UPDATER_WATCH_NAMESPACES` (or `--watch-namespaces`) to include both namespaces, since by default the controller only watches its own namespace and would not see `Applications` in `<argocd_namespace>`.

    ```yaml
    ...
          env:
          - name: ARGOCD_NAMESPACE
            value: "<argocd_namespace>"
          - name: IMAGE_UPDATER_WATCH_NAMESPACES
            value: "<updater_namespace>,<argocd_namespace>"
    ...
    ```

    or

    ```yaml
    ...
          data:
            argocd.namespace: "<argocd_namespace>"
            watch.namespaces: "<updater_namespace>,<argocd_namespace>"
    ...
    ```

    Alternatively, you can add the `--argocd-namespace=<argocd_namespace>` and `--watch-namespaces=<updater_namespace>,<argocd_namespace>` flags to the container's `command` arguments in the deployment manifest.

3. **Patch the metrics ClusterRoleBindings**

    The `install.yaml` manifest hardcodes `namespace: argocd` in the subjects of the metrics `ClusterRoleBinding` resources (`argocd-image-updater-metrics-auth-rolebinding` and `argocd-image-updater-metrics-reader-rolebinding`). Patch both to reference `<updater_namespace>` instead, otherwise the controller's ServiceAccount will not receive the `TokenReview`/`SubjectAccessReview`/metrics-reader permissions.

4. **Grant permissions in the Argo CD namespace**

    `ImageUpdater` CRs must be in the same namespace as the `Applications` they
    reference, so they belong in `<argocd_namespace>`. The controller needs a full
    `Role` and `RoleBinding` there to manage them alongside `Applications` and Argo CD
    `Secrets`/`ConfigMaps`:

    ```yaml
    apiVersion: rbac.authorization.k8s.io/v1
    kind: Role
    metadata:
      name: argocd-image-updater-argocd-ns-role
      namespace: <argocd_namespace>
    rules:
    - apiGroups: [""]
      resources: ["secrets", "configmaps"]
      verbs: ["get", "list", "watch"]
    - apiGroups: [""]
      resources: ["events"]
      verbs: ["create"]
    - apiGroups: ["argocd-image-updater.argoproj.io"]
      resources: ["imageupdaters"]
      verbs: ["create", "delete", "get", "list", "patch", "update", "watch"]
    - apiGroups: ["argocd-image-updater.argoproj.io"]
      resources: ["imageupdaters/finalizers"]
      verbs: ["update"]
    - apiGroups: ["argocd-image-updater.argoproj.io"]
      resources: ["imageupdaters/status"]
      verbs: ["get", "patch", "update"]
    - apiGroups: ["argoproj.io"]
      resources: ["applications"]
      verbs: ["get", "list", "patch", "update", "watch"]
    ---
    apiVersion: rbac.authorization.k8s.io/v1
    kind: RoleBinding
    metadata:
      name: argocd-image-updater-argocd-ns-rolebinding
      namespace: <argocd_namespace>
    roleRef:
      apiGroup: rbac.authorization.k8s.io
      kind: Role
      name: argocd-image-updater-argocd-ns-role
    subjects:
    - kind: ServiceAccount
      name: argocd-image-updater-controller
      namespace: <updater_namespace>
    ```

#### Option 3: Cluster-scoped installation

For environments where the controller must watch `ImageUpdater` CRs in any namespace, use `--watch-namespaces="*"`. This requires a `ClusterRole` and `ClusterRoleBinding`.

1. **Install the Controller**

    ```shell
    kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj-labs/argocd-image-updater/stable/config/install.yaml
    ```

2. **Replace Role with ClusterRole**

    Delete the namespace-scoped RBAC from the default install and apply cluster-scoped equivalents. The required permissions mirror `config/rbac/role.yaml`:

    ```shell
    kubectl delete role argocd-image-updater-manager-role -n argocd
    kubectl delete rolebinding argocd-image-updater-manager-rolebinding -n argocd
    ```

    ```yaml
    apiVersion: rbac.authorization.k8s.io/v1
    kind: ClusterRole
    metadata:
      name: argocd-image-updater-manager-role
    rules:
    - apiGroups: [""]
      resources: ["events"]
      verbs: ["create"]
    - apiGroups: ["argocd-image-updater.argoproj.io"]
      resources: ["imageupdaters"]
      verbs: ["create", "delete", "get", "list", "patch", "update", "watch"]
    - apiGroups: ["argocd-image-updater.argoproj.io"]
      resources: ["imageupdaters/finalizers"]
      verbs: ["update"]
    - apiGroups: ["argocd-image-updater.argoproj.io"]
      resources: ["imageupdaters/status"]
      verbs: ["get", "patch", "update"]
    - apiGroups: ["argoproj.io"]
      resources: ["applications"]
      verbs: ["get", "list", "patch", "update", "watch"]
    ---
    apiVersion: rbac.authorization.k8s.io/v1
    kind: ClusterRoleBinding
    metadata:
      name: argocd-image-updater-manager-rolebinding
    roleRef:
      apiGroup: rbac.authorization.k8s.io
      kind: ClusterRole
      name: argocd-image-updater-manager-role
    subjects:
    - kind: ServiceAccount
      name: argocd-image-updater-controller
      namespace: argocd  # Replace with your controller namespace
    ```

3. **Enable cluster-scoped watching**

    ```yaml
    env:
    - name: IMAGE_UPDATER_WATCH_NAMESPACES
      value: "*"
    ```

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

## Running locally

As long as you have access to your Kubernetes cluster from
your workstation, running Argo CD Image Updater is simple. Make sure that your
Kubernetes client configuration points to the correct K8s cluster.

Grab the binary and run:

```bash
./argocd-image-updater run \
  --kubeconfig ~/.kube/config
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
metrics. This feature is disabled by default but can be enabled using the
`--metrics-bind-address` flag to specify a listening address (e.g., `:8080`).
Metrics are then served on the `/metrics` path.

CR-scoped metrics are labeled by `image_updater_cr_name` and
`image_updater_cr_namespace`. They are populated only when the controller runs
with polling (default `run` command); in webhook-only mode (`webhook` command)
they are disabled to avoid orphaned series when CRs are deleted.

**Available metrics**

*   `argocd_image_updater_applications_watched_total` - A gauge that shows the
    number of applications watched per `ImageUpdater` CR.
*   `argocd_image_updater_images_watched_total` - A gauge that shows the number
    of images watched (considered for update) per `ImageUpdater` CR.
*   `argocd_image_updater_images_updated_total` - A counter of the number of
    images updated per `ImageUpdater` CR.
*   `argocd_image_updater_images_errors_total` - A counter of the number of
    errors during image updates per `ImageUpdater` CR.

**Sample output on the `/metrics` endpoint**

```text
# HELP argocd_image_updater_applications_watched_total The total number of applications watched by Argo CD Image Updater CR
# TYPE argocd_image_updater_applications_watched_total gauge
argocd_image_updater_applications_watched_total{image_updater_cr_name="dev",image_updater_cr_namespace="argocd"} 1
argocd_image_updater_applications_watched_total{image_updater_cr_name="prod",image_updater_cr_namespace="argocd"} 2

# HELP argocd_image_updater_images_watched_total Number of images watched by Argo CD Image Updater CR
# TYPE argocd_image_updater_images_watched_total gauge
argocd_image_updater_images_watched_total{image_updater_cr_name="dev",image_updater_cr_namespace="argocd"} 2
argocd_image_updater_images_watched_total{image_updater_cr_name="prod",image_updater_cr_namespace="argocd"} 1

# HELP argocd_image_updater_images_updated_total Number of images updated by Argo CD Image Updater CR
# TYPE argocd_image_updater_images_updated_total counter
argocd_image_updater_images_updated_total{image_updater_cr_name="dev",image_updater_cr_namespace="argocd"} 2
argocd_image_updater_images_updated_total{image_updater_cr_name="prod",image_updater_cr_namespace="argocd"} 5

# HELP argocd_image_updater_images_errors_total Number of errors reported by Argo CD Image Updater CR
# TYPE argocd_image_updater_images_errors_total counter
argocd_image_updater_images_errors_total{image_updater_cr_name="dev",image_updater_cr_namespace="argocd"} 0
argocd_image_updater_images_errors_total{image_updater_cr_name="prod",image_updater_cr_namespace="argocd"} 0
```

Here, two ImageUpdater CRs in the `argocd` namespace are tracked. Gauges
reflect the current state of the last run; counters are cumulative since
controller start.

!!! note "Other Defined Metrics"
    The metrics listed below are also defined within the application. However,
    for various reasons, they are either not populated with data or have been
    temporarily disabled. They may not appear on the `/metrics` endpoint or may
    always report a value of `0`.

    *   `argocd_image_updater_k8s_api_requests_total`
    *   `argocd_image_updater_k8s_api_errors_total`
    *   `argocd_image_updater_registry_requests_total`
    *   `argocd_image_updater_registry_requests_failed_total`

A (very) rudimentary example dashboard definition for Grafana is provided
[here](https://github.com/argoproj-labs/argocd-image-updater/tree/master/config)
