# Getting Started

## Installation methods

The Argo CD Image Updater controller **must** be run in the same Kubernetes cluster where your Argo CD `Application` resources are managed. The current controller architecture does not support connecting to a remote Kubernetes cluster to manage applications.

While the `argocd-image-updater` binary can be run locally from your workstation for one-time updates (see `Running locally` section), the standard and supported installation for continuous, automated updates is as a controller inside your cluster.

## <a name="install-kubernetes"></a>Installing as Kubernetes workload

The most straightforward way to run the image updater is to install it as a Kubernetes workload using the provided installation manifests. These manifests will set up the controller in its own dedicated namespace (`argocd-image-updater-system` by default).
Don't worry, without creating any ImageUpdater custom resources, it will not start modifying your workloads yet.

!!!note
    We also provide a Kustomize base in addition to the plain Kubernetes YAML
    manifests. You can use it as remote base and create overlays with your
    configuration on top of it. The remote base's URL is
    `https://github.com/argoproj-labs/argocd-image-updater/config/default`.
    You can view the manifests [here](https://github.com/argoproj-labs/argocd-image-updater/tree/stable/config/default)


### Apply the installation manifests

```shell
kubectl apply -f https://raw.githubusercontent.com/argoproj-labs/argocd-image-updater/stable/config/install.yaml
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
