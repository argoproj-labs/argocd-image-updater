# Configuring images for update

## Annotation format

You can specify one or more image(s) for each application that should be
considered for updates. To specify those images, the following annotation
is used:

```yaml
argocd-image-updater.argoproj.io/image-list: <image_spec_list>
```

The `<image_spec_list>` is a comma separated list of image specifications. Each
image specification is composed of mandatory and optional information, and is
used to specify the image, its version constraint and a few meta data.

An image specification could be formally described as:

```text
[<alias_name>=]<image_path>[:<version_constraint>]
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

## Assigning aliases to images

It's possible (and sometimes necessary) to assign an alias name to any given
image. Alias names should consist of alphanumerical characters only, and must
be unique within the same application. Re-using an alias name across different
applications is allowed.

An alias name is assigned during image specification in the `image-list`
annotation, for example the following would assign the alias `myalias` to the
image `some/image`:

```yaml
argocd-image-updater.argproj.io/image-list: myalias=some/image
```

Assigning an alias name to an image is necessary in these scenarios:

* If you want to use custom images with Kustomize. In this case, the name must
  match to what is defined in your Kustomize base.

* If you need to specify the Helm parameters used for rendering the image name
  and version using Helm and the parameter names do not equal `image.name` and
  `image.tag`. In this case, the name is just symbolic.

* If you want to set custom options for a given image's update strategy, or
  require referencing unique pull secrets for each image

The alias you assign to any image will be reused as a key in the annotations
used to define further options, so a little care should be taken when defining
such a name. It's generally advised to use only alpha-numerical characters. The
character `/` (forward-slash) can be used in the name, but must be referenced
as `_` (underscore) in the annotation. This is a limit of Kubernetes. So for
example, if you assign the alias `argoproj/argocd` to your image, the
appropriate key in the annotation would be referenced as `argoproj_argocd`.

## Update strategies

Argo CD Image Updater can update images according to the following strategies:

|Strategy|Description|
|--------|-----------|
|`semver`| Update to the tag with the highest allowed semantic version|
|`latest`| Update to the tag with the most recent creation date|
|`name`  | Update to the tag with the latest entry from an alphabetically sorted list|

You can define the update strategy for each image independently by setting the
following annotation to an appropriate value:

```yaml
argocd-image-updater.argoproj.io/<image_name>.update-strategy: <strategy>
```

If no update strategy is given, or an invalid value was used, the default
strategy `semver` will be used.

## Custom images with Kustomize

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

## Specifying Helm parameter names

In case of Helm applications which contain more than one image in the manifests
or use another set of parameters than `image.name` and `image.tag` to define
which image to render in the manifests, you need to set an `<image_alias>`
in the image specification to define an alias for that image, and then
use another set of annotations to specify the appropriate parameter names
that should get set if an image gets updated.

For example, if you have an image `quay.io/dexidp/dex` that is configured in
your helm chart using the `dex.image.name` and `dex.image.tag` Helm parameters,
you can set the following annotations on your `Application` resource so that
Argo CD Image Updater will know which Helm parameters to set:

```yaml
argocd-image-updater.argoproj.io/image-list: dex=quay.io/dexidp/dex
argocd-image-updater.argoproj.io/dex.helm.image-name: dex.image.name
argocd-image-updater.argoproj.io/dex.helm.image-tag: dex.image.tag

```

The general syntax for the two Helm specific annotations is:

```yaml
argocd-image-updater.argoproj.io/<image_alias>.helm.image-name: <name of helm parameter to set for the image name>
argocd-image-updater.argoproj.io/<image_alias>.helm.image-tag: <name of helm parameter to set for the image tag>
```

If the chart uses a parameter for the canonical name of the image (i.e. image
name and tag combined), a third option can be used:

```yaml
argocd-image-updater.argoproj.io/<image_alias>.helm.image-spec: <name of helm parameter to set for canonical name of image>
```

If the `<image_alias>.helm.image-spec` annotation is set, the two other
annotations `<image_alias>.helm.image-name` and `<image_alias>.helm.image-tag`
will be ignored.

## Examples

### Following an image's patch branch

*Scenario:* You have deployed image `nginx:1.19.1` and want to make sure it's
always up-to-date to the latest patch level within the `1.19` branch.

*Solution:* Use standard `semver` update strategy with a constraint on the
patch level (`~`), i.e.

```yaml
argocd-image-updater.argoproj.io/image-list: nginx:~1.19
```

### Always deploy the latest build

*Scenario:* Your CI regulary pushes images built from the latest source, using
some identifier (i.e. the hash of the Git commit) in the tag.

*Solution:*

1. Make sure that the image tags follow semantic versioning and use the Git
   commit hash as pre-release identifier, i.e. `v1.0.0-<githash>`

2. Define an alias for your image when configuring it for update, and match
   against pre-release in the version constraint by prepending `-0`.

3. Use `latest` as update strategy

```yaml
argocd-image-updater.argoproj.io/image-list: yourtool=yourorg/yourimage:v1.0.0-0
argocd-image-updater.argoproj.io/yourtool.update-strategy: latest
```

## Appendix

### Available annotations

The following is a complete list of available annotations to control the
update strategy and set options for images. Please note, all annotations
must be prefixed with `argocd-image-updater.argoproj.io`.

|Annotation name|Default value|Description|
|---------------|-------|-----------|
|`image-list`|*none*|Comma separated list of images to consider for update|
|`<image_alias>.update-strategy`|`semver`|The update strategy to be used for the image|
|`<image_alias>.helm.image-spec`|*none*|Name of the Helm parameter to specify the canonical name of the image, i.e. holds `image/name:1.0`. If this is set, other Helm parameter related options will be ignored.|
|`<image_alias>.helm.image-name`|`image.name`|Name of the Helm parameter used for specifying the image name, i.e. holds `image/name`|
|`<image_alias>.helm.image-tag`|`image.tag`|Name of the Helm parameter used for specifying the image tag, i.e. holds `1.0`|
|`<image_alias>.kustomize.image-name`|*original name of image*|Name of Kustomize image parameter to set during updates|
