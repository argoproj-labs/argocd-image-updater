# Getting Started

## Installation methods

The Argo CD Image Updater controller **must** be run in the same Kubernetes cluster where your Argo CD `Application` resources are managed. The current controller architecture does not support connecting to a remote Kubernetes cluster to manage applications.

While the `argocd-image-updater` binary can be run locally from your workstation for one-time updates (see `Running locally` section), the standard and supported installation for continuous, automated updates is as a controller inside your cluster.

### Multi-Cluster Environments

In a multi-cluster setup where Argo CD, running on a central control plane cluster (let's call it cluster A), manages `Application` resources that are deployed to another cluster (cluster B), there is one important restriction:

*   The Image Updater controller **must** be installed on cluster A.
*   All `ImageUpdater` custom resources (CRs) must also be created on cluster A.

The controller cannot discover or process `ImageUpdater` CRs created on cluster B.

In short: The Image Updater controller and its `ImageUpdater` CRs must reside with your Argo CD `Application` resources, not with the deployed application workloads.

### Choosing an installation namespace

You have two options for where to install the Argo CD Image Updater:

#### Option 1: Install into the Argo CD namespace (Recommended)

The simplest approach is to install the image updater into the same namespace as your Argo CD installation. This requires minimal configuration.

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

!!! warning
    The default installation manifests assume Argo CD is in the `argocd` namespace. If your Argo CD runs in a different namespace (e.g., `my-argocd`), you must download the `install.yaml` manifest and manually update the `namespace` field in the `subjects` section of all `ClusterRoleBinding` resources from `argocd` to your target namespace.

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

```yaml
...
      env:
      - name: ARGOCD_NAMESPACE
        value: <argocd_namespace>
...
```

or

```yaml
...
      data:
        argocd.namespace: <argocd_namespace>
...
```

Alternatively, you can add the `--argocd-namespace=<argocd_namespace>` flag to the container's `command` arguments in the deployment manifest.

3. **Adjust ClusterRoleBinding**

The installation manifest contains `ClusterRoleBinding` resources that grant the controller's `ServiceAccount` cluster-wide permissions. You must update these bindings to reference the namespace where the image updater is installed (`<updater_namespace>`).

To do this, download `install.yaml` and manually change the `namespace` in the `subjects` section of all `ClusterRoleBinding` resources from `argocd` to `<updater_namespace>` before applying the manifest.

4. **Grant Permissions in the Argo CD Namespace**

The image updater needs to read resources from the Argo CD namespace, such as `Secrets` containing repository credentials. The default installation does not grant these cross-namespace permissions. You must create a `Role` and `RoleBinding` in the Argo CD namespace to allow the image updater's ServiceAccount (from `<updater_namespace>`) to access these resources.

Create and apply the following manifest in the `<argocd_namespace>`:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: argocd-image-updater-cross-namespace-reader
  namespace: <argocd_namespace>
rules:
- apiGroups: [""]
  resources: ["secrets", "configmaps"]
  verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: argocd-image-updater-cross-namespace-reader
  namespace: <argocd_namespace>
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: argocd-image-updater-cross-namespace-reader
subjects:
- kind: ServiceAccount
  name: argocd-image-updater-controller
  namespace: <updater_namespace>
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

The following metric is currently available and populated with data:

*   `argocd_image_updater_applications_watched_total` - A gauge that shows the
    number of applications watched per `ImageUpdater` CR.

!!! note "Other Defined Metrics"
    The metrics listed below are also defined within the application. However,
    for various reasons, they are either not populated with data or have been
    temporarily disabled. They may not appear on the `/metrics` endpoint or may
    always report a value of `0`.

*   `argocd_image_updater_images_watched_total`
*   `argocd_image_updater_images_updated_total`
*   `argocd_image_updater_images_errors_total`
*   `argocd_image_updater_k8s_api_requests_total`
*   `argocd_image_updater_k8s_api_errors_total`
*   `argocd_image_updater_registry_requests_total`
*   `argocd_image_updater_registry_requests_failed_total`

A (very) rudimentary example dashboard definition for Grafana is provided
[here](https://github.com/argoproj-labs/argocd-image-updater/tree/master/config)
