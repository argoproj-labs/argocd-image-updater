# ArgoCD Image Updater

A tool to automatically update the container images of Kubernetes workloads
that are managed by
[ArgoCD](https://github.com/argoproj/argo-cd).

!!!warning "A note on the current status"
    ArgoCD Image Updater was born just recently. It is not suitable for
    production use yet, and it might break things in unexpected ways.

    You are welcome to test it out on non-critical environments, and to 
    contribute by sending bug reports, enhancement requests and - most
    appreciated - pull requests.

    There will be (probably a lot of) breaking changes from release to
    release as development progresses until version 1.0. We will do our
    best to indicate any breaking change and how to un-break it in the
    [Changelog](https://github.com/argoproj-labs/argocd-image-updater/CHANGELOG.md)

## Overview

The ArgoCD Image Updater can check for new versions of the container images
that are deployed with your Kubernetes workloads and automatically update them
to their latest allowed version using ArgoCD. It works by setting appropriate
application parameters for ArgoCD applications, i.e. similar to
`argocd app set --helm-set image.tag=v1.0.1` - but in a fully automated
manner.

Usage is simple: You annotate your ArgoCD `Application` resources with a list
of images to be considered for update, along with a version constraint to
restrict the maximum allowed new version for each image. ArgoCD Image Updater
then regulary polls the configured applications from ArgoCD and queries the
corresponding container registry for possible new versions. If a new version of
the image is found in the registry, and the version constraint is met, ArgoCD
Image Updater instructs ArgoCD to update the application with the new image.

Depending on your Automatic Sync Policy for the Application, ArgoCD will either
automatically deploy the new image version or mark the Application as Out Of
Sync, and you can trigger the image update manually by syncing the Application.
Due to the tight integration with ArgoCD, advanced features like Sync Windows,
RBAC authorization on Application resources etc. are fully supported.

## Limitations

The three most important limitations first. These will most likely not change
anywhere in the near future, because they are limitations by design.

Please make sure to understand these limitations, and do not send enhancement
requests or bug reports related to the following:

* The applications you want container images to be updated **must** be managed
  using ArgoCD. There is no support for workloads not managed using ArgoCD.

* ArgoCD Image Updater can only update container images for applications whose
  manifests are rendered using either *Kustomize* or *Helm* and - especially
  in the case of Helm - the templates need to support specifying the image's
  tag (and possibly name) using a parameter (i.e. `image.tag`).

* Your images' tags need to follow the semantic versioning scheme. ArgoCD
  Image Updater will not be able to update images that are just made from
  arbitrary strings, or consist solely of Git SHA strings.

Otherwise, current known limitations are:

* Image pull secrets must exist in the same Kubernetes cluster where ArgoCD
  Image Updater is running in (or has accesst to). It is currently not possible
  to fetch those secrets from other clusters.
