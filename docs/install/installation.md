# Getting Started

## Installation methods

The Argo CD Image Updater controller **must** be run in the same Kubernetes cluster where your Argo CD `Application` resources are managed. The current controller architecture does not support connecting to a remote Kubernetes cluster to manage applications.

While the `argocd-image-updater` binary can be run locally from your workstation for one-time updates (see `Running locally` section), the standard and supported installation for continuous, automated updates is as a controller inside your cluster.

## <a name="install-kubernetes"></a>Installing as Kubernetes workload

The most straightforward way to run the image updater is to install it as a Kubernetes workload using the provided installation manifests. These manifests will set up the controller in its own dedicated namespace (`argocd-image-updater-system` by default).
Don't worry, without creating any ImageUpdater custom resources, it will not start modifying your workloads yet.

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

!!!note "Under Construction"
    Please note that Prometheus metrics are not available in the initial CRD-based versions of Argo CD Image Updater. The functionality described below is planned for a future release. We are keeping this section as a reference for when metrics are re-introduced.

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

A (very) rudimentary example dashboard definition for Grafana is provided
[here](https://github.com/argoproj-labs/argocd-image-updater/tree/master/config)
