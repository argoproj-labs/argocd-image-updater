# Updating container images

## General process overview

Argo CD Image Updater can update container images managed by one or more of
your Argo CD applications, according to how it is configured using `ImageUpdater`
custom resources.

The workflow of Argo CD Image Updater can be described as follows:

* The controller uses a reconciliation loop that monitors `ImageUpdater` custom
  resources. Each `ImageUpdater` CR defines which Argo CD applications should
  be monitored for image updates through the `applicationRefs` field, which can
  specify applications by name patterns or label selectors.

* For each `ImageUpdater` CR, the controller lists all Argo CD `Application`
  resources in the specified namespace and matches them against the
  `applicationRefs` patterns and label selectors defined in the CR.

* The controller then processes each matching application according to the
  image configurations defined in the `ImageUpdater` CR. Each image
  configuration specifies the image name, update strategy, and other
  constraints like allowed tags, ignore tags, and platform requirements.

* For each image found in the configuration, Argo CD Image Updater will first
  check if this image is actually deployed with the application. It does a
  strict check for the complete image name, including the registry the image is
  pulled from. For example, `docker.io/some/image` and `quay.io/some/image`,
  while both referring to `some/image`, are not considered equal. This strict
  behavior can be relaxed, however. See [forcing image updates](../configuration/images.md#forcing-image-updates) for
  further explanation.

* If Argo CD Image Updater considers an image from the list eligible for an
  update check, it will connect the corresponding container registry to see
  if there is a newer version of the image according to the
  [update strategy](./update-strategies.md)
  and other constraints that may have been configured for the image (e.g.
  a list of tags that are allowed to consider).

* If a newer version of an image was found, Argo CD Image Updater will try
  to update the image according to the configured
  [update method](./update-methods.md). Please note that Argo CD Image Updater will
  never update your manifests, instead it re-configures your Application
  sources to use the new image tag, and control is handed over to Argo CD.

## <a name="multi-arch"></a>Multi-arch images and clusters

As of version 0.12, Argo CD Image Updater has full support for multi-arch
images (and multi-arch clusters) by being able to inspect images with multiple
manifests (i.e. a manifest list).

Multi-arch currently only is supported for
[update strategies](./update-strategies.md)
which fetch image meta-data: `latest` and `digest`. Multi-arch will be ignored
for the update strategies that do not fetch meta-data, `semver` and `name`.

By default, Argo CD Image Updater will only consider updating to images that
have a manifest for the same platform where itself runs on. If you are on a
cluster that has nodes of multiple architectures, and are pinning certain
workloads to certain nodes, you will have to tell Argo CD Image Updater which
platforms are allowed for a certain application or an image. This can be done
by setting an appropriate
[annotation per image](../configuration/images.md#platforms)
or for all images of a given
[application as a default setting](../configuration/images.md#appendix-defaults).

Multi-arch is also implemented by the
[test command](../install/testing.md#multi-arch).

## Sync policies and image updates

As explained above, the Argo CD Image Updater will assume that Argo CD will
update the manifests in your cluster to use any new image that has been set
by the Argo CD Image Updater.

Argo CD Image Updater will work best with automatic syncing enabled for the
Argo CD applications that are being updated.

## Rollback and image updates

Currently, Argo CD Image Updater does not honor the rollback status of an
Argo CD application, and keeps updating to new images also for Applications
that are being rolled back. However, Argo CD will disable auto-sync for
such applications.

Honoring rollbacked applications correctly is on our roadmap.
