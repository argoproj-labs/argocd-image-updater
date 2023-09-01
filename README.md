# Argo CD Image Updater

![Integration tests](https://github.com/argoproj-labs/argocd-image-updater/workflows/Integration%20tests/badge.svg?branch=master&event=push)
[![Documentation Status](https://readthedocs.org/projects/argocd-image-updater/badge/?version=latest)](https://argocd-image-updater.readthedocs.io/en/latest/?badge=latest)
[![codecov](https://codecov.io/gh/argoproj-labs/argocd-image-updater/branch/master/graph/badge.svg)](https://codecov.io/gh/argoproj-labs/argocd-image-updater)
[![Go Report Card](https://goreportcard.com/badge/github.com/argoproj-labs/argocd-image-updater)](https://goreportcard.com/report/github.com/argoproj-labs/argocd-image-updater)

## Introduction

Argo CD Image Updater is a tool to automatically update the container
images of Kubernetes workloads which are managed by Argo CD. In a nutshell,
it will track image versions specified by annotations on the Argo CD
Application resources and update them by setting parameter overrides using
the Argo CD API.

Currently it will only work with applications that are built using *Kustomize*
or *Helm* tooling. Applications built from plain YAML or custom tools are not
supported yet (and maybe never will). 

## Documentation

Read
[the documentation](https://argocd-image-updater.readthedocs.io/en/stable/)
for more information on how to setup and run Argo CD Image Updater and to get
known to it's features and limitations.

Above URL points to the documentation for the current release. If you are
interested in documentation of upcoming features, check out the
[the latest documentation](https://argocd-image-updater.readthedocs.io/en/latest/)
which is up-to-date with the master branch.

## Current status

Argo CD Image Updater is under active development. We would not recommend it
yet for *critical* production workloads, but feel free to give it a spin.

We're very interested in feedback on usability and the user experience as well
as in bug discoveries and enhancement requests.

**Important note:** Until the first stable version (i.e. `v1.0`) is released,
breaking changes between the releases must be expected. We will do our best
to indicate all breaking changes (and how to un-break them) in the
[Changelog](CHANGELOG.md)

## Contributing

You are welcome to contribute to this project by means of raising issues for
bugs, sending & discussing enhancement ideas or by contributing code via pull
requests.

In any case, please be sure that you have read & understood the currently known
design limitations before raising issues.

Also, if you want to contribute code, please make sure that your code

* has its functionality covered by unit tests (coverage goal is 80%),
* is correctly linted,
* is well commented,
* and last but not least is compatible with our license and CLA

Please note that in the current early phase of development, the code base is
a fast moving target and lots of refactoring will happen constantly.

## License

`argocd-image-updater` is open source software, released under the
[Apache 2.0 license](https://www.apache.org/licenses/LICENSE-2.0)

## Things that are planned (roadmap)

The following things are on the roadmap until the `v1.0` release

* [ ] Extend Argo CD functionality to be able to update images for other types
  of applications.

* [x] Extend Argo CD functionality to write back to Git

* [ ] Provide web hook support to trigger update check for a given image

* [x] Use concurrency for updating multiple applications at once

* [x] Improve error handling

* [x] Support for image tags with i.e. Git commit SHAs

For more details, check out the
[v1.0.0 milestone](https://github.com/argoproj-labs/argocd-image-updater/milestone/1)

## Frequently asked questions

**Does it write back the changes to Git?**

We're happy to announce that as of `v0.9.0` and Argo CD `v1.9.0`, Argo CD
Image Updater is able to commit changes to Git. It will not modify your
application's manifests, but instead writes
[Parameter Overrides](https://argoproj.github.io/argo-cd/user-guide/parameters/#store-overrides-in-git)
to the repository.

We think that this is a good compromise between functionality (have everything
in Git) and ease-of-use (minimize conflicts).

**Are there plans to extend functionality beyond Kustomize or Helm?**

Not yet, since we are dependent upon what functionality Argo CD provides for
these types of applications.

**Will it ever be fully integrated with Argo CD?**

In the current form, probably not. If there is community demand for it, let's
see how we can make this happen.

There is [an open proposal](https://github.com/argoproj/argo-cd/issues/7385) to migrate this project into the `argoproj` org (out
of the `argoproj-labs` org) and include it in the installation of Argo CD.
