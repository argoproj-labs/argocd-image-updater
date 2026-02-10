# Configuring images for update

## Image configuration in ImageUpdater CR

Images are configured in the `ImageUpdater` custom resource using the `images`
field within each `applicationRef`. Each image configuration specifies the image
to track, update strategy, and other settings.

The basic structure for image configuration looks like this:

```yaml
apiVersion: argocd-image-updater.argoproj.io/v1alpha1
kind: ImageUpdater
metadata:
  name: my-image-updater
  namespace: argocd
spec:
  applicationRefs:
    - namePattern: "my-app-*"
      images:
        - alias: "nginx"
          imageName: "nginx:1.20"
```

### Image specification format

Each image in the `images` array must have:

* **`alias`** (required) - A unique identifier for this image within the
  application reference
* **`imageName`** (required) - The full image identifier including registry,
  repository, and initial tag/version

## Allowing an image for update

The most simple form of specifying an image allowed to update would be the
following:

```yaml
images:
  - alias: "nginx"
    imageName: "nginx"
```

The above example would specify to update the image `nginx` to its most recent
version found in the container registry, without taking any version constraints
into consideration.

This is most likely not what you want, because you could pull in some breaking
changes when `nginx` releases a new major version and the image gets updated.
So you can give a version constraint along with the image specification:

```yaml
images:
  - alias: "nginx"
    imageName: "nginx:~1.26"
```

The above example would allow the `nginx` image to be updated to any patch
version within the `1.26` minor release.

More information on how to specify semantic version constraints can be found
in the
[documentation](https://github.com/Masterminds/semver#checking-version-constraints)
of the [Semver library](https://github.com/Masterminds/semver) we're using.

!!!note
If you use an
[update strategy](#update-strategies)
other than `semver` or `digest`, the `version_constraint` will not have any effect
and all tags returned from the registry will be considered for update. If
you need to further restrict the list of tags to consider, see
[filtering tags](#filtering-tags)
below.

### Forcing Image Updates

By default, Image Updater will only update an image that is actually used in your Application
(i.e., is it exported in the Status of your ArgoCD Application.)

To support custom resources and things like PodTemplates that don't actually create a container,
you may force an update:

```yaml
images:
  - alias: "myalias"
    imageName: "some/image"
    commonUpdateSettings:
      forceUpdate: true
```

## Assigning aliases to images

It's required to assign an alias name to any given image.
Alias names should consist of alphanumerical characters only, and must
be unique within the application reference.

For example, the following would assign the alias `myalias` to the
image `some/image`:

```yaml
images:
  - alias: "myalias"
    imageName: "some/image"
```

It's generally advised to use only alphanumerical characters. The
character `/` (forward-slash) can be used in the name, but must be referenced
as `_` (underscore) in the annotation. This is a limitation of Kubernetes. So for
example, if you assign the alias `argoproj/argocd` to your image, the
appropriate key in the annotation would be referenced as `argoproj_argocd`.

## Update strategies

Argo CD Image Updater can update images according to the following strategies:

| Strategy              | Description                                                                |
|-----------------------|----------------------------------------------------------------------------|
| `semver`              | Update to the tag with the highest allowed semantic version                |
| `latest/newest-build` | Update to the tag with the most recent creation date                       |
| `name/alphabetical`   | Update to the tag with the latest entry from an alphabetically sorted list |
| `digest`              | Update to the most recent pushed version of a mutable tag                  |

You can define the update strategy for each image independently by setting the
following annotation to an appropriate value:

```yaml
images:
  - alias: "myalias"
    imageName: "some/image"
    commonUpdateSettings:
      updateStrategy: <strategy>
```

If no update strategy is given, or an invalid value is used, the default
strategy `semver` will be used.

!!!warning "Renamed image update strategies"
The `latest` strategy has been renamed to `newest-build`, and `name` strategy has been renamed to `alphabetical`.
Please switch to the new convention as support for the old naming convention will be removed in future releases.

!!!warning
As of November 2020, Docker Hub has introduced pull limits for accounts on
the free plan and unauthenticated requests. The `latest/newest-build` update strategy
will perform manifest pulls for determining the most recently pushed tags,
and these will count into your pull limits. So unless you are not affected
by these pull limits, it is **not recommended** to use the `latest/newest-build` update
strategy with images hosted on Docker Hub.

## Filtering tags

You can specify an expression that is matched against each tag returned from
the registry. On a positive match, the tag will be included in the list of tags
that will be considered to update the image to. If the expression does not
match the tag, the tag will not be included in the list. This allows you to
only consider tags that you are generally interested in.

You can define a tag filter by using the following:

```yaml
images:
  - alias: "myalias"
    imageName: "some/image"
    commonUpdateSettings:
      allowTags: <match_func>
```

The following match functions are currently available:

| Function              | Description                                                        |
|-----------------------|--------------------------------------------------------------------|
| `regexp:<expression>` | Matches the tag name against the regular expression `<expression>` |
| `any`                 | Will match any tag                                                 |

If you specify an invalid match function, or the match function is misconfigured
(i.e. an invalid regular expression is supplied), no tag will be matched at all
to prevent considering (and possibly update to) the wrong tags by accident.

If the annotation is not specified, a match function `any` will be used to match
the tag names, effectively performing no filtering at all.

## Ignoring certain tags

If you want to ignore certain tags from the registry for any given image, you
can define a comma-separated list of glob-like patterns using the following:

```yaml
images:
  - alias: "myalias"
    imageName: "some/image"
    commonUpdateSettings:
      ignoreTags: <pattern1>[, <pattern2>, ...]
```

You can use glob patterns as described in this
[documentation](https://golang.org/pkg/path/filepath/#Match)

If you want to disable updating an image temporarily, without removing all
the configuration, you can do so by just ignoring all tags, effectively
preventing the image updater from considering any of the tags returned from the
registry:

```yaml
images:
  - alias: "myalias"
    imageName: "some/image"
    commonUpdateSettings:
      ignoreTags: "*"
```

Please note that regular expressions are not supported to be used for patterns.

## <a name="platforms"></a>Image platforms

By default, Argo CD Image Updater will only consider images from the registry
that are built for the same platform as the one Argo CD Image Updater is
running on. In multi-arch clusters, your workloads may be targeted to a
different platform, and you can configure the allowed platforms for a given
image.

For example, when Argo CD Image Updater is running on a `linux/amd64` node but
your application will be executed on a node with `linux/arm64` platform, you
need to let Argo CD Image Updater know:

```yaml
images:
  - alias: "myalias"
    imageName: "some/image"
    commonUpdateSettings:
      platforms: "linux/arm64"
```

You can specify multiple allowed platforms as a comma-separated list of allowed
platforms:

```yaml
images:
  - alias: "myalias"
    imageName: "some/image"
    commonUpdateSettings:
      platforms: "linux/arm64,linux/amd64"
```

The correct image to execute will be chosen by Kubernetes.

!!!note
The `platforms` field only has effect for images that use an update
strategy that fetches meta-data. Currently, these are the `latest` and
`digest` strategies. For `semver` and `name` strategies, the `platforms`
setting has no effect.

## <a name="pull-secrets"></a>Specifying pull secrets

There are generally two ways on how to specify pull secrets for Argo CD Image
Updater to use. Either you configure a secret reference globally for the
container registry (as described [here](registries.md)), or you can specify
the pull secret to use for a given image using `ImageUpdater` resource.

```yaml
images:
  - alias: "myalias"
    imageName: "some/image"
    commonUpdateSettings:
      pullSecret: <secret_ref>
```

A configuration for an image will override what is configured for the registry,
for that certain image.

The `secret_ref` can either be a reference to a secret or a reference to an
environment variable. If a secret is referenced, the secret must exist in the
cluster where Argo CD Image Updater is running (or has access to).

Valid values for `secret_ref` are:

* `secret:<namespace>/<secret_name>#<field>` - Use credentials stored in the
  field `field` from secret `secret_name` in namespace `namespace`.

* `pullsecret:<namespace>/<secret_name>` - Use credentials stored in the secret
  `secret_name` in namespace `namespace`. The secret is treated as a Docker pull
  secret, that is, it must have a valid Docker config in JSON format in the
  field `.dockerconfigjson`.

* `env:<variable_name>` - Use credentials supplied by the environment variable
  named `variable_name`. This can be a variable that is i.e. bound from a
  secret within your pod spec.

* `ext:<path_to_script>` - Use credentials generated by a script. The script
  to execute must be specified using an absolute path, and must have the
  executable bit set. The script is supposed to output the credentials to be
  used as a single line to stdout, in the format `<username>:<password>`.
  Please note that the script will be executed every time the Argo CD Image
  Updater goes to find a new version, and credentials will not be cached. If
  you want it to execute only once and cache credentials, you should configure
  this secret on the registry level instead.

In case of `secret` or `env`references, the data stored in the reference must
be in format `<username>:<password>`

## Custom images with Kustomize

In Kustomize, if you want to use an image from another registry or a completely
different image than what is specified in the manifests, you can configure this
using the `manifestTargets.kustomize` field in your ImageUpdater resource.

First, you need to specify the target image name in the
`manifestTargets.kustomize.name` field:

```yaml
images:
  - alias: <image_alias>
    imageName: <image_name>:<image_tag>
    manifestTargets:
      kustomize:
        name: <original_image_name>
```

In this case, `imageName` should be the name of the image that you want to
update to (the source image), while `manifestTargets.kustomize.name` specifies
the original image name.

Let's take Argo CD's Kustomize base as an example: The original image used by
Argo CD is `quay.io/argoproj/argocd`, pulled from Quay container registry. If
you want to follow the latest builds, as published on the GitHub registry, you
could override the image specification in Kustomize as follows:

```yaml
images:
  - alias: "argocd"
    imageName: "ghcr.io/argoproj/argocd:latest"
    manifestTargets:
      kustomize:
        name: "quay.io/argoproj/argocd"
```

Under the hood, this would be similar to the following kustomize command:

```shell
kustomize edit set image quay.io/argoproj/argocd=ghcr.io/argoproj/argocd
```

Finally, if you have not yet overridden the image name in your manifests (i.e.
there's no image `ghcr.io/argoproj/argocd` running in your application), you
may need to tell Image Updater to force the update despite no image running:

```yaml
images:
  - alias: "argocd"
    imageName: "ghcr.io/argoproj/argocd:latest"
    commonUpdateSettings:
      forceUpdate: true
    manifestTargets:
      kustomize:
        name: "quay.io/argoproj/argocd"
```

## Specifying Helm parameter names

In the case of Helm applications that contain more than one image in the manifests
or use another set of parameters than `image.name` and `image.tag` to define
which image to render in the manifests, you need to configure the `manifestTargets.helm`
field to specify the appropriate parameter names that should get set when an image
gets updated.

For example, if you have an image `quay.io/dexidp/dex` that is configured in
your helm chart using the `dex.image.name` and `dex.image.tag` Helm parameters,
you can configure the ImageUpdater resource as follows:

```yaml
images:
  - alias: "dex"
    imageName: "quay.io/dexidp/dex:latest"
    manifestTargets:
      helm:
        name: "dex.image.name"
        tag: "dex.image.tag"
```

The general syntax for the Helm configuration is:

```yaml
images:
  - alias: "<image_alias>"
    imageName: "<image_name>"
    manifestTargets:
      helm:
        name: "<name of helm parameter to set for the image name>"
        tag: "<name of helm parameter to set for the image tag>"
```

If the chart uses a parameter for the canonical name of the image (i.e. image
name and tag combined), you can use the `spec` field instead:

```yaml
images:
  - alias: "<image_alias>"
    imageName: "<image_name>"
    manifestTargets:
      helm:
        spec: "<name of helm parameter to set for the canonical name of image>"
```

If the `spec` field is set, the `name` and `tag` fields will be ignored.

If the image is in a YAML list, then the index can be specified
in the `name`, `tag`, or `spec` fields using square brackets:

```yaml
images:
  - alias: "<image_alias>"
    imageName: "<image_name>"
    manifestTargets:
      helm:
        name: "images[0].name"
        tag: "images[0].tag"
```

## Examples

### Following an image's patch branch

*Scenario:* You have deployed image `nginx:1.19.1` and want to make sure it's
always up-to-date to the latest patch level within the `1.19` branch.

*Solution:* Use standard `semver` update strategy with a constraint on the
patch level (`~`), i.e.

```yaml
images:
  - alias: "nginx"
    imageName: "nginx:~1.19"
```

### Always deploy the latest build

*Scenario:* Your CI regularly pushes images built from the latest source, using
some identifier (i.e. the hash of the Git commit) in the tag.

*Solution:*

1. Give your image a proper alias, i.e. `yourtool` and do not define a version
   constraint.

2. Use `latest` as update strategy

3. If you just want to consider a given set of tags, i.e. `v1.0.0-<hash>`, use a
   `commonUpdateSettings.allowTags` field.

```yaml
images:
  - alias: "yourtool"
    imageName: "yourorg/yourimage"
    commonUpdateSettings:
      updateStrategy: "latest"
      allowTags: "regexp:^v1.0.0-[0-9a-zA-Z]+$"
```

### Multiple images in the same Helm chart

*Scenario:* You want to update multiple images within the same Helm chart to
their latest available version according to semver.

The Helm parameters to set the image version
are `foo.image` and `foo.tag` for the first image, and `bar.image` and
`bar.tag` for the second image. The image names are `foo/bar` and `bar/foo`
for simplicity.

*Solution:*

1. Set `helm.name` and `helm.tag` to their appropriate values

```yaml
images:
  - alias: "fooalias"
    imageName: "foo/bar"
    manifestTargets:
      helm:
        name: "foo.image"
        tag: "foo.tag"
  - alias: "baralias"
    imageName: "bar/foo"
    manifestTargets:
      helm:
        name: "bar.image"
        tag: "bar.tag"
```

### Tracking an image's `latest` tag

*Scenario:* You want to track the latest build of a given tag, e.g. the `latest`
tag that many images use without having to restart your pods manually.

*Solution:*

1. Set the constraint of your image to the tag you want to track, e.g. `latest`

2. Set the update strategy for this image to `digest`

```yaml
images:
  - alias: "fooalias"
    imageName: "yourorg/yourimage:latest"
    commonUpdateSettings:
      updateStrategy: "digest"
```

When there's a new build for `yourorg/yourimage:latest` found in the registry,
Argo CD Image Updater will update your configuration to use the SHA256 sum of
the image, and Kubernetes will restart your pods automatically to have them
use the new image.

### Updating the image in the yaml list

*Scenario:* You want to automatically update the image `nginx:1.19` that is inside the yaml list, e.g.

```yaml
foo:
  - name: foo-1
    image: busybox:latest
    command: [ 'sh', '-c', 'echo "Custom container running"' ]
  - name: foo-2
    image: nginx:1.19
```

*Solution:* Use the index in square brackets of the item that needs to be updated, i.e.

```yaml
images:
  - alias: "fooalias"
    imageName: "fooimagename"
    manifestTargets:
      helm:
        spec: "foo[1].image"
```

This works for fields `manifestTargets.helm.name`, `manifestTargets.helm.tag` and `manifestTargets.helm.spec`.

## Appendix

### <a name="appendix-fields"></a>Available ImageUpdater fields

The following is a complete list of available fields in the ImageUpdater CRD to control
update strategies and set options for images.

#### Top-level ImageUpdater fields

| Field                  | Type                 | Required | Description                                                                                                                                                                                                         |
|------------------------|----------------------|----------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `namespace`            | string               | No       | **Deprecated**: Target namespace where Argo CD Applications are located. The controller now uses the ImageUpdater CR's `metadata.namespace` instead. This field is ignored and will be removed in a future release. |
| `applicationRefs`      | []ApplicationRef     | Yes      | List of application references to manage                                                                                                                                                                            |
| `commonUpdateSettings` | CommonUpdateSettings | No       | Global default settings for all applications                                                                                                                                                                        |
| `writeBackConfig`      | WriteBackConfig      | No       | Global write-back configuration                                                                                                                                                                                     |

#### ApplicationRef fields

| Field                  | Type                 | Required | Description                                                                                                    |
|------------------------|----------------------|----------|----------------------------------------------------------------------------------------------------------------|
| `namePattern`          | string               | Yes      | Glob pattern for application name selection                                                                    |
| `images`               | []ImageConfig        | No       | List of image configurations (required if `useAnnotations` is not true, ignored when `useAnnotations` is true) |
| `useAnnotations`       | bool                 | No       | When true, read image configuration from Application's legacy annotations                                      |
| `labelSelectors`       | LabelSelector        | No       | Label selectors for application selection                                                                      |
| `commonUpdateSettings` | CommonUpdateSettings | No       | Override global settings for this application group (ignored when `useAnnotations` is true)                    |
| `writeBackConfig`      | WriteBackConfig      | No       | Override global write-back config for this application group (ignored when `useAnnotations` is true)           |

#### ImageConfig fields

| Field                  | Type                 | Required | Description                                                           |
|------------------------|----------------------|----------|-----------------------------------------------------------------------|
| `alias`                | string               | Yes      | Unique identifier for the image within the application reference      |
| `imageName`            | string               | Yes      | Full image identifier including registry, repository, and initial tag |
| `commonUpdateSettings` | CommonUpdateSettings | No       | Override settings for this specific image                             |
| `manifestTargets`      | ManifestTarget       | No       | Configuration for updating image references in manifests              |

#### CommonUpdateSettings fields

| Field            | Type     | Default    | Description                                                                     |
|------------------|----------|------------|---------------------------------------------------------------------------------|
| `updateStrategy` | string   | `"semver"` | Update strategy: `semver`, `latest/newest-build`, `digest`, `name/alphabetical` |
| `forceUpdate`    | bool     | `false`    | Force updates even if image is not currently deployed                           |
| `allowTags`      | string   | *none*     | Regex pattern for tags to allow                                                 |
| `ignoreTags`     | []string | *none*     | List of glob patterns for tags to ignore                                        |
| `pullSecret`     | string   | *none*     | Reference to secret for registry credentials                                    |
| `platforms`      | []string | *none*     | List of target platforms (e.g., `linux/amd64`, `linux/arm64`)                   |

#### WriteBackConfig fields

| Field       | Type      | Default    | Description                                               |
|-------------|-----------|------------|-----------------------------------------------------------|
| `method`    | string    | `"argocd"` | Write-back method: `argocd`, `git`, or `git:<secret_ref>` |
| `gitConfig` | GitConfig | *none*     | Git configuration (can only be used when method is `git`) |

#### GitConfig fields

| Field             | Type   | Required | Description                                                  |
|-------------------|--------|----------|--------------------------------------------------------------|
| `repository`      | string | No       | Git repository URL (defaults to Application's repoURL)       |
| `branch`          | string | No       | Git branch for commits                                       |
| `writeBackTarget` | string | No       | Target file path and type (e.g., `helmvalues:./values.yaml`) |

#### ManifestTarget fields

| Field       | Type            | Required | Description                                                     |
|-------------|-----------------|----------|-----------------------------------------------------------------|
| `helm`      | HelmTarget      | No       | Helm-specific configuration (mutually exclusive with kustomize) |
| `kustomize` | KustomizeTarget | No       | Kustomize-specific configuration (mutually exclusive with helm) |

#### HelmTarget fields

| Field  | Type   | Required | Description                                                                                                                        |
|--------|--------|----------|------------------------------------------------------------------------------------------------------------------------------------|
| `name` | string | No       | Dot-separated path to Helm key for image name                                                                                      |
| `tag`  | string | No       | Dot-separated path to Helm key for image tag                                                                                       |
| `spec` | string | No       | Dot-separated path to Helm key for full image specification. If this is set, other Helm parameter-related options will be ignored. |

#### KustomizeTarget fields

| Field  | Type   | Required | Description                                    |
|--------|--------|----------|------------------------------------------------|
| `name` | string | Yes      | Image name as it appears in kustomization.yaml |

### <a name="appendix-hierarchy"></a>Configuration hierarchy

Settings can be configured at multiple levels with the following precedence (highest to lowest):

1. **ImageConfig level** - Most specific, overrides all other levels
2. **ApplicationRef level** - Overrides global settings for applications matching the pattern
3. **ImageUpdater level** - Global defaults for all applications

For example, if you set `updateStrategy: "semver"` at the global level but 
`updateStrategy: "latest"` at the image level, the image will use `"latest"`.