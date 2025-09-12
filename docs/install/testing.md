# Testing outside the cluster

The `argocd-image-updater` binary provides means to test its behavior using the
`test` subcommand.

You can use this command from your workstation without any modifications to your
Argo CD installation or applications, and without having to install the image
updater in your Kubernetes cluster. The `test` command does not need to talk to
Argo CD, and only needs access to your Kubernetes cluster if you need to use
image pull secret stored in Kubernetes.

The `test` command's main purpose is to verify the behaviour of Argo CD
Image Updater on arbitrary images and to validate your configuration. For
example, the most simple form of running a test is the following:

```shell
argocd-image-updater test <image_name>
```

## Testing registry access

For example, to see what Argo CD Image Updater would consider the latest nginx
version on Docker Hub according to semantic versioning, you can run:

```shell
$ argocd-image-updater test nginx --loglevel info
INFO[0000] retrieving information about image            command=test image_name=nginx image_registry=
INFO[0000] Fetching available tags and metadata from registry  command=test image_name=nginx image_registry=
INFO[0002] Found 923 tags in registry                    command=test image_name=nginx image_registry=
INFO[0002] latest image according to constraint is nginx:1.29.1  command=test image_name=nginx image_registry=
```

## Multi-arch images

As stated in the section about
[multi-arch support](../basics/update.md#multi-arch),
Argo CD Image Updater by default only considers images in the registry for the
same platform as `argocd-image-updater` is executed on.

For the `test` command, this means it takes the platform of the system you are
running it on. If you are executing it for example on your Mac workstation, most
likely no results will yield at all - because there are simple no native images
for any of the `darwin` platforms.

You can specify the target platforms manually, using the `--platforms` command
line option, e.g. the following demonstrates no available images for the given
platform:

```shell
$ argocd-image-updater test quay.io/argoprojlabs/argocd-e2e-container --platforms darwin/amd64 --update-strateg newest-build --loglevel debug 
INFO[0000] retrieving information about image            command=test image_name=quay.io/argoprojlabs/argocd-e2e-container image_registry=quay.io
DEBU[0000] setting rate limit to 20 requests per second  command=test image_name=quay.io/argoprojlabs/argocd-e2e-container image_registry=quay.io prefix=quay.io registry="https://quay.io"
DEBU[0000] Inferred registry from prefix quay.io to use API https://quay.io  command=test image_name=quay.io/argoprojlabs/argocd-e2e-container image_registry=quay.io
INFO[0000] Fetching available tags and metadata from registry  command=test image_name=quay.io/argoprojlabs/argocd-e2e-container image_registry=quay.io
DEBU[0002] Manifest list did not contain any usable reference. Platforms requested: (darwin/amd64), platforms included: (linux/amd64,linux/arm64,linux/s390x,linux/ppc64le)  command=test image_name=quay.io/argoprojlabs/argocd-e2e-container image_registry=quay.io
DEBU[0002] No metadata found for argoprojlabs/argocd-e2e-container:0.1  command=test image_name=quay.io/argoprojlabs/argocd-e2e-container image_registry=quay.io                                                                                                                          
DEBU[0002] Manifest list did not contain any usable reference. Platforms requested: (darwin/amd64), platforms included: (linux/amd64,linux/arm64,linux/s390x,linux/ppc64le)  command=test image_name=quay.io/argoprojlabs/argocd-e2e-container image_registry=quay.io
DEBU[0002] No metadata found for argoprojlabs/argocd-e2e-container:0.2  command=test image_name=quay.io/argoprojlabs/argocd-e2e-container image_registry=quay.io                                                                                                                          
DEBU[0002] Manifest list did not contain any usable reference. Platforms requested: (darwin/amd64), platforms included: (linux/amd64,linux/arm64,linux/s390x,linux/ppc64le)  command=test image_name=quay.io/argoprojlabs/argocd-e2e-container image_registry=quay.io
DEBU[0002] No metadata found for argoprojlabs/argocd-e2e-container:0.3  command=test image_name=quay.io/argoprojlabs/argocd-e2e-container image_registry=quay.io                                                                                                                          
INFO[0002] Found 0 tags in registry                      command=test image_name=quay.io/argoprojlabs/argocd-e2e-container image_registry=quay.io
INFO[0002] no newer version of image found               command=test image_name=quay.io/argoprojlabs/argocd-e2e-container image_registry=quay.io
```

While setting the `platforms` to include `linux/amd64`, yields the following:

```shell
$ argocd-image-updater test quay.io/argoprojlabs/argocd-e2e-container --platforms linux/amd64 --update-strateg newest-build --loglevel debug 
INFO[0000] retrieving information about image            command=test image_name=quay.io/argoprojlabs/argocd-e2e-container image_registry=quay.io
DEBU[0000] setting rate limit to 20 requests per second  command=test image_name=quay.io/argoprojlabs/argocd-e2e-container image_registry=quay.io prefix=quay.io registry="https://quay.io"
DEBU[0000] Inferred registry from prefix quay.io to use API https://quay.io  command=test image_name=quay.io/argoprojlabs/argocd-e2e-container image_registry=quay.io
INFO[0000] Fetching available tags and metadata from registry  command=test image_name=quay.io/argoprojlabs/argocd-e2e-container image_registry=quay.io
INFO[0002] Found 3 tags in registry                      command=test image_name=quay.io/argoprojlabs/argocd-e2e-container image_registry=quay.io
DEBU[0002] found 3 from 3 tags eligible for consideration  command=test image_name=quay.io/argoprojlabs/argocd-e2e-container image_registry=quay.io
INFO[0002] latest image according to constraint is quay.io/argoprojlabs/argocd-e2e-container:0.3  command=test image_name=quay.io/argoprojlabs/argocd-e2e-container image_registry=quay.io
```

## Testing for semver constraints

To see what it would consider the latest patch version within the 1.17 release,
run:

```shell
$ argocd-image-updater test nginx --semver-constraint 1.17.X --loglevel info
INFO[0000] retrieving information about image            command=test image_name=nginx image_registry=
INFO[0000] Fetching available tags and metadata from registry  command=test image_name=nginx image_registry=
INFO[0001] Found 923 tags in registry                    command=test image_name=nginx image_registry=
INFO[0001] latest image according to constraint is nginx:1.17.10  command=test image_name=nginx image_registry=
```

## Testing different update strategies

You can test the result of different
[update strategies](../basics/update-strategies.md)
using the `--update-strategy` command line option, e.g.:

```shell
$ argocd-image-updater test ghcr.io/argoproj/argocd --update-strategy newest-build --loglevel info
DEBU[0000] Creating in-cluster Kubernetes client        
INFO[0000] retrieving information about image            image_alias= image_name=ghcr.io/argoproj/argocd registry_url=ghcr.io
DEBU[0000] setting rate limit to 20 requests per second  prefix=ghcr.io registry="https://ghcr.io"
DEBU[0000] Inferred registry from prefix ghcr.io to use API https://ghcr.io 
INFO[0000] Fetching available tags and metadata from registry  application=test image_alias= image_name=ghcr.io/argoproj/argocd registry_url=ghcr.io
INFO[0139] Found 864 tags in registry                    application=test image_alias= image_name=ghcr.io/argoproj/argocd registry_url=ghcr.io
DEBU[0139] found 864 from 864 tags eligible for consideration  image=ghcr.io/argoproj/argocd
INFO[0139] latest image according to constraint is ghcr.io/argoproj/argocd:2.4.0-f8390c94  application=test image_alias= image_name=ghcr.io/argoproj/argocd registry_url=ghcr.io
```

## Using credentials

If you need to specify 
[credentials](../basics/authentication.md#auth-registries),
you can do so using the `--credentials` parameter. It accepts the same values
as the corresponding
[annotation](../configuration/images.md#pull-secrets), i.e.:

```shell
$ export GITHUB_PULLSECRET="<username>:<token>"
$ argocd-image-updater test docker.pkg.github.com/argoproj/argo-cd/argocd --update-strategy latest --credentials env:GITHUB_PULLSECRET
INFO[0000] getting image                                 image_name=argoproj/argo-cd/argocd registry=docker.pkg.github.com
INFO[0000] Fetching available tags and metadata from registry  image_name=argoproj/argo-cd/argocd
INFO[0040] Found 100 tags in registry                    image_name=argoproj/argo-cd/argocd
INFO[0040] latest image according to constraint is docker.pkg.github.com/argoproj/argo-cd/argocd:2.5.0-1ae5aff2
```

For a complete list of available command line parameters, run
`argocd-image-updater test --help`.

It is recommended that you read about core updating and image concepts in the
[documentation](../../configuration/images/)
before using this command.
