# Configuring images for update

## Annotation format

You can specify one or more image(s) for each application that should be
considered for updates. To specify those images, the following annotation
is used:

```yaml
argocd-image-updater.argoproj.io/image-list: <image spec list>
```

The `<image spec list>` is a comma separated list of image specifications. Each
image specification is composed of mandatory and optional information, and is
used to specify the image, its version constraint and a few meta data.

An image specification could be formally described as:

```text
[<image_name>=]<image_path>[:<version_constraint>][#<secret ref>]
```

Specifying the fields denoted in square brackets is optional and can be left
out.

## Allowing an image for update

The most simple form of specifying an image allowed to update would be the
following:

```yaml
argocd-image-updater.argoproj.io/image-list: nginx
```

The above example would specify to update the image `nginx` to it's most recent
version found in the container registry, without taking any version constraints
into cosideration.

This is most likely not what you want, because you could pull in some breaking
changes when `nginx` releases a new major version and the image gets updated.
So you can give a version constraint along with the image specification:

```yaml
argocd-image-updater.argoproj.io/image-list: nginx:~1.26
```

The above example would allow the `nginx` image to be updated to any patch
version within the `1.26` minor release.

More information on how to specify semantic version constraints can be found
in the
[documentation](https://github.com/Masterminds/semver#checking-version-constraints)
of the [Semver library](https://github.com/Masterminds/semver) we're using.

## Naming images

Giving a name to an image is necessary in these scenarios:

* If you want to use custom images with Kustomize. In this case, the name must
  match to what is defined in your Kustomize base.

* If you need to specify the Helm parameters used for rendering the image name
  and version using Helm and the parameter names do not equal `image.name` and
  `image.tag`. In this case, the name is just symbolic.

### Custom images with Kustomize

In Kustomize, if you want to use an image from another registry or a completely
different image than what is specified in the manifests, you can give the image
specification as follows:

```text
<image_name>=<image_path>:<image_tag>
```

`<image_name>` will be the original image name, as used in your manifests, and
`<image_path>:<image_path>` will be the value used when rendering the
manifests.

Let's take Argo CD's Kustomize base as an example: The original image used by
Argo CD is `argoproj/argocd`, pulled from the Docker Hub container registry. If
you are about to follow the latest builds, as published on the GitHub registry,
you could override the image specification in Kustomize as follows:

```text
argoproj/argocd=docker.pkg.github.com/argoproj/argo-cd/argocd:1.7.0-a6399e59
```

### Specifying Helm parameter names

!!!note
    Image names should not be too complex. In case of Helm, they must only
    consist of letters and numbers because the names will be reused in
    Kubernetes annotation names, and thus, must fit in the overall naming
    convention of Kubernetes annotation names.

In case of Helm applications which contain more than one image in the manifests
or use another set of parameters than `image.name` and `image.tag` to define
which image to render in the manifests, you can use the `<name>` parameter in
the image specification to define a (symbolic) name for that image. Then, you
can use another set of annotations to specify the appropriate parameter names
that should get set if an image gets updated.

For example, if you have an image `quay.io/dexidp/dex` that is configured in
your helm chart using the `dex.image.name` and `dex.image.tag` Helm parameters,
you can set the following annotations on your `Application` resource so that
Argo CD Image Updater will know which Helm parameters to set:

```yaml
argocd-image-updater.argoproj.io/image-list: dex=quay.io/dexidp/dex
argocd-image-updater.argoproj.io/dex.image-name: dex.image.name
argocd-image-updater.argoproj.io/dex.image-tag: dex.image.tag

```

The general syntax for the two Helm specific annotations is:

```yaml
argocd-image-updater.argoproj.io/<name>.image-name: <name of helm parameter to set>
```