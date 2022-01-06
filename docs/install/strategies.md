# Update strategies

Argo CD Image Updater supports different update strategies for the images that
are configured to be tracked and updated.

You can configure the strategy to be used for each image individually, with the
default strategy being `semver` (see below).

Some of the strategies will require additional configuration, or can be tweaked
with additional parameters. Please have a look at the
[image configuration](../../configuration/images)
documentation for more details.

Please note that all update strategies except `digest` assume tags to be
*immutable* and that new images will be pushed with a new, unique tag. If
you want to update to *mutable* tags (e.g. the commonly used `latest` tag),
you should use the `digest` strategy.

## Update to semantic versions

This is the default strategy.

Strategy name: `semver`

```yaml
argocd-image-updater.argoproj.io/image-list: some/image[:<version_constraint>]
# Specifying update-strategy is optional, because semver is the default
argocd-image-updater.argoproj.io/<image>.update-strategy: semver
```

The `semver` strategy allows you to track & update images which use tags that
follow the
[semantic versioning scheme](https://semver.org). Tag names must contain semver
compatible identifiers in the format `X.Y.Z`, where `X`, `Y` and `Z` must be
whole numbers. An optional prefix of `v`, e.g. `vX.Y.Z` is allowed, and both
variants are treated equal (so, a constraint of `v1.x` would match a tag `1.0`
and a constraint of `1.x` also matches a tag `v1.0`).

Updating to pre-release versions (e.g. `-rc1`) is supported, but must be 
explicitly allowed (see below).

This will allow you to update to the latest version of an image within a given
patch branch or minor release, or just to the latest version that has is tagged
with a valid semantic version identifier.

To tell Argo CD Image Updater which versions are allowed, simply give a semver
version as a constraint in the `image-list` annotation. For example, to allow
updates to the latest patch release within the `1.2` minor release branch, use

```
argocd-image-updater.argoproj.io/image-list: some/image:1.2.x
```

The above example would update to any new tag pushed to the registry matching
this constraint, e.g. `1.2.5`, `1.2.12` etc, but not to a new minor version
(e.g. `1.3`).

Likewise, to allow updates to any minor release within the major version `1`,
use

```yaml
argocd-image-updater.argoproj.io/image-list: some/image:1.x
```

The above example would update to any new tag pushed to the registry matching
this constraint, e.g. `1.2.12`, `1.3.0`, `1.15.2` etc, but not to a new major
version (e.g. `2.0`).

If you also want to allow updates to pre-release versions (e.g. `v2.0-rc1`),
you need to append the suffix `-0` to the constraint, for example

```
argocd-image-updater.argoproj.io/image-list: some/image:2.x-0
```

If no version constraint is specified in the list of allowed images, Argo CD
Image Updater will pick the highest version number found in the registry.

Argo CD Image Updater will omit any tags from your registry that do not match 
a semantic version when using the `semver` update strategy.

## Update to the most recently built image

!!!warning
    As of November 2020, Docker Hub has introduced pull limits for accounts on
    the free plan and unauthenticated requests. The `latest` update strategy
    will perform manifest pulls for determining the most recently pushed tags,
    and these will count into your pull limits. So unless you are not affected
    by these pull limits, it is **not recommended** to use the `latest` update
    strategy with images hosted on Docker Hub.

Strategy name: `latest`

```yaml
argocd-image-updater.argoproj.io/image-list: <alias>=some/image
argocd-image-updater.argoproj.io/<alias>.update-strategy: latest
```

Argo CD Image Updater can update to the image that has the most recent build
date, and is tagged with an arbitrary name (e.g. a Git commit SHA, or even a
random string). 

It is important to understand, that this strategy will consider the build date
of the image, and not the date of when the image was tagged or pushed to the
registry. If you are tagging the same image with multiple tags, these tags
will have the same build date. In this case, Argo CD Image Updater will sort
the tag names lexically descending and pick the last tag name of that list.
For example, consider an image that was tagged with the `f33bacd`, `dev`
and `latest` tags. You might want to have the `f33bacd` tag set for your
application, but Image Updater will pick the `latest` tag name. In order to
prevent such a situation, you need to further restrict the tags that Image
Updater will inspect, see below.

By default, this update strategy will inspect all of the tags it found in the
image's repository. If you wish to allow only certain tags to be considered
for update, you will need additional configuration. For example,

```yaml
argocd-image-updater.argoproj.io/image-list: myimage=some/image
argocd-image-updater.argoproj.io/myimage.update-strategy: latest
argocd-image-updater.argoproj.io/myimage.allow-tags: regexp:^[0-9a-f]{7}$
```

would only consider tags that match a given regular expression for update. In
this case, the regular expression matches a 7-digit hexadecimal string that
could represent the short version of a Git commit SHA, so it would match tags
like `a5fb3d3` or `f7bb2e3`, but not `latest` or `master`.

Likewise, you can ignore a certain list of tags from your repository:

```yaml
argocd-image-updater.argoproj.io/image-list: myimage=some/image
argocd-image-updater.argoproj.io/myimage.update-strategy: latest
argocd-image-updater.argoproj.io/myimage.ignore-tags: latest, master
```

This would allow for considering all tags found but `latest` and `master`. You
can read more about filtering tags
[here](../../configuration/images/#filtering-tags).

## Update to the most recent pushed version of a tag

Strategy name: `digest`

```yaml
argocd-image-updater.argoproj.io/image-list: <alias>=some/image:<tag_name>
argocd-image-updater.argoproj.io/<alias>.update-strategy: digest
```

This update strategy inspects a single tag in the registry for changes, and
updates the image on any change to the previous state. The tag name to be
inspected must be specified as a version constraint in the image list.

Use this update strategy if you want to follow a *mutable* tag, such as the
commonly used `latest` tag. 

Argo CD Image Updater will then update the image when either

* The currently running image has a non-digest specification (e.g. uses a tag),
  or
* the currently used digest differs from what is found in the registry

!!!note "Tags and digests"
    Note that the `digest` update strategy will use image digests for updating
    the image tags in your applications, so the image running in your
    application will appear as `some/image@sha256:<somelonghashstring>` instead
    of `some/image:latest`. So in your running system, the tag information will
    be effectively lost.

For example, the following specification would always update the image for an
application on each new push of the image `some/image` with the tag `latest`:

```yaml
argocd-image-updater.argoproj.io/image-list: myimage=some/image:latest
argocd-image-updater.argoproj.io/myimage.update-strategy: digest
```

## Update according to lexical sort

Strategy name: `name`

```yaml
argocd-image-updater.argoproj.io/image-list: <alias>=some/image
argocd-image-updater.argoproj.io/<alias>.update-strategy: name
```

This update strategy sorts the tags returned by the registry in a lexical way
(by name, in descending order) and picks the last tag in the list for update.
This can be useful if the image you want to track uses `calver` versioning,
with tags in the format of e.g. `YYYY-MM-DD` or similar lexical sortable
strings.

By default, this update strategy will inspect all of the tags it found in the
image's repository. If you wish to allow only certain tags to be considered
for update, you will need additional configuration. For example,

```yaml
argocd-image-updater.argoproj.io/image-list: myimage=some/image
argocd-image-updater.argoproj.io/myimage.update-strategy: name
argocd-image-updater.argoproj.io/myimage.allow-tags: regexp:^[0-9]{4}-[0-9]{2}[0-9]{2}$
```

would only consider tags that match a given regular expression for update. In
this case, only tags matching a date specification of `YYYY-MM-DD` would be
considered for update.
