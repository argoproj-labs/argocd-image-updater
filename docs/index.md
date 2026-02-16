# Argo CD Image Updater

A tool to automatically update the container images of Kubernetes workloads
that are managed by
[Argo CD](https://github.com/argoproj/argo-cd).

!!!warning "Breaking Change: spec.namespace Field Deprecated"
    The `spec.namespace` field is now optional and deprecated. It will be removed in a future release. The controller now uses the ImageUpdater CR's `metadata.namespace` to determine which namespace to search for applications. This is a breaking change for configurations where `spec.namespace` differs from `metadata.namespace`. For details and migration examples, see [Namespace Field Deprecation](#namespace-field-deprecation) below.

!!!warning "A note on the current status"
    There has been a major transition from an annotation-based configuration to a
    CRD-based configuration with the v1.0.0 release. This documentation covers
    the modern, CRD-based configuration for versions v1.x. For the legacy,
    annotation-based configuration for versions v0.x (e.g. `0.17.0`), please
    consult the documentation for the respective version.

    Argo CD Image Updater is under active development.

    You are welcome to test it out on non-critical environments, and of
    course to
    [contribute](./contributing/start.md) by many means.

    We will do our
    best to indicate any breaking change and how to un-break it in the
    respective
    [release notes](https://github.com/argoproj-labs/argocd-image-updater/releases).

## Overview

The Argo CD Image Updater can check for new versions of the container images
that are deployed with your Kubernetes workloads and automatically update them
to their latest allowed version using Argo CD. It works by setting appropriate
application parameters for Argo CD applications, i.e. similar to
`argocd app set --helm-set image.tag=v1.0.1` - but in a fully automated
manner.

Usage is simple: You create `ImageUpdater` custom resources that define which
Argo CD applications should be monitored for image updates, along with the images
to be considered for update and version constraints to restrict the maximum
allowed new version for each image. Argo CD Image Updater then uses a
reconciliation loop to monitor the configured applications from Argo CD and
queries the corresponding container registry for possible new versions. If a new
version of the image is found in the registry, and the version constraint is met,
Argo CD Image Updater instructs Argo CD to update the application with the new
image.

Applications to update are considered only in the namespace that matches the
ImageUpdater CR's `metadata.namespace` field.

Depending on your Automatic Sync Policy for the Application, Argo CD will either
automatically deploy the new image version or mark the Application as Out Of
Sync, and you can trigger the image update manually by syncing the Application.
Due to the tight integration with Argo CD, advanced features like Sync Windows,
RBAC authorization on Application resources etc. are fully supported.

## Features

!!!warning "Renamed image update strategies"
    The `latest` strategy has been renamed to `newest-build`, and `name` strategy has been renamed to `alphabetical`. 
    Please switch to the new convention as support for the old naming convention will be removed in future releases.

* Updates images of apps that are managed by Argo CD and are either generated
  from *Helm* or *Kustomize* tooling
* Update app images according to different
  [update strategies](./basics/update-strategies.md)
    * `semver`: update to highest allowed version according to given image
    constraint,
    * `latest/newest-build`: update to the most recently created image tag,
    * `name/alphabetical`: update to the last tag in an alphabetically sorted list
    * `digest`: update to the most recent pushed version of a mutable tag
* Support for 
  [widely used container registries](./configuration/registries.md#supported-registries)
* Support for private container registries via 
  [configuration](./configuration/registries.md#custom-registries)
* Can write changes
  [back to Git](./basics/update-methods.md#method-git)
* Ability to filter list of tags returned by a registry using matcher functions
* Support for custom, per-image 
  [pull secrets](./basics/authentication.md#auth-registries) (using generic K8s
  secrets, K8s pull secrets, environment variables or external scripts)
* Runs in a 
  [Kubernetes cluster](./install/installation.md#install-kubernetes) or can be
  used stand-alone from the command line
* Ability to perform parallel update of applications
* Webhook server to receive registry events and trigger immediate image updates
  for supported registries (Docker Hub, GitHub Container Registry, Quay, Harbor)

## Limitations

The two most important limitations first. These will most likely not change
anywhere in the near future, because they are limitations by design.

Please make sure to understand these limitations, and do not send enhancement
requests or bug reports related to the following:

* The applications you want container images to be updated **must** be managed
  using Argo CD. There is no support for workloads not managed using Argo CD.

* Argo CD Image Updater can only update container images for applications whose
  manifests are rendered using either *Kustomize* or *Helm* and - especially
  in the case of Helm - the templates need to support specifying the image's
  tag (and possibly name) using a parameter (i.e. `image.tag`).

Otherwise, current known limitations are:

* Image pull secrets must exist in the same Kubernetes cluster where Argo CD
  Image Updater is running in (or has access to). It is currently not possible
  to fetch those secrets from other clusters.

## Questions, help and support

If you have any questions, need some help in setting things up or just want to
discuss something, feel free to

* open an issue on our GitHub issue tracker or

* join us in the `#argo-cd-image-updater` channel on the
  [CNCF slack](https://argoproj.github.io/community/join-slack/)

## Namespace Field Deprecation

The `spec.namespace` field in `ImageUpdater` resources is now **deprecated and optional**. The controller now uses the ImageUpdater CR's `metadata.namespace` field to determine which namespace to search for Argo CD Applications.

### Impact

**Breaking Change**: This affects configurations where `spec.namespace` differs from `metadata.namespace`. In such cases, the ImageUpdater CR will no longer update Applications in the namespace specified by `spec.namespace`.

**No Impact**: If your configuration already uses `metadata.namespace == spec.namespace`, your setup will continue to work without changes.

### Examples

#### Broken Configuration (Will Stop Working)

This configuration will **stop working** because the CR is in `argocd` namespace but tries to update Applications in `production` namespace:

```yaml
apiVersion: argocd-image-updater.argoproj.io/v1alpha1
kind: ImageUpdater
metadata:
  name: my-updater
  namespace: argocd  # CR is in argocd namespace
spec:
  namespace: production  # Trying to update apps in different namespace
  applicationRefs:
    - namePattern: "my-app"
      images:
        - alias: "nginx"
          imageName: "nginx:1.20"
```

#### Correct Configuration (Option 1: Same Namespace)

Move the ImageUpdater CR to the same namespace as your Applications:

```yaml
apiVersion: argocd-image-updater.argoproj.io/v1alpha1
kind: ImageUpdater
metadata:
  name: my-updater
  namespace: production  # CR is in same namespace as Applications
spec:
  # spec.namespace can be omitted or set to same value
  applicationRefs:
    - namePattern: "my-app"
      images:
        - alias: "nginx"
          imageName: "nginx:1.20"
```

#### Correct Configuration (Option 2: Remove spec.namespace)

If your CR and Applications are already in the same namespace, simply remove `spec.namespace`:

```yaml
apiVersion: argocd-image-updater.argoproj.io/v1alpha1
kind: ImageUpdater
metadata:
  name: my-updater
  namespace: argocd  # CR and Applications in same namespace
spec:
  # spec.namespace removed - controller uses metadata.namespace
  applicationRefs:
    - namePattern: "my-app"
      images:
        - alias: "nginx"
          imageName: "nginx:1.20"
```

### Migration Steps

1. **Identify affected configurations**: Find ImageUpdater CRs where `spec.namespace` differs from `metadata.namespace`
2. **Move CRs to target namespace**: Create new ImageUpdater CRs in the namespace where your Applications are located
3. **Remove spec.namespace**: Delete the `spec.namespace` field from existing CRs (it will be ignored anyway)
4. **Verify**: Ensure Applications are found and updated correctly after migration
