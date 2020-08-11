# Argo CD Image Updater

![Integration tests](https://github.com/argoproj-labs/argocd-image-updater/workflows/Integration%20tests/badge.svg?branch=master&event=push)
[![Documentation Status](https://readthedocs.org/projects/argocd-image-updater/badge/?version=latest)](https://argocd-image-updater.readthedocs.io/en/latest/?badge=latest)
[![codecov](https://codecov.io/gh/argoproj-labs/argocd-image-updater/branch/master/graph/badge.svg)](https://codecov.io/gh/argoproj-labs/argocd-image-updater)
[![Go Report Card](https://goreportcard.com/badge/github.com/argoproj-labs/argocd-image-updater)](https://goreportcard.com/report/github.com/argoproj-labs/argocd-image-updater)

## Introduction

Argo CD Image Updater is a tool to automatically update the container
images of Kubernetes workloads which are managed by Argo CD.

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

**Disclaimer: This is pre-release code. It might have bugs that will
break things in unexpected way.**

Argo CD Image Updater was born just recently, and is not suitable for
production workloads yet. You are welcome to test it in your non-critical
environments, and to contribute by filing bugs, enhancement requests or even
better, sending in pull requests.

We decided to publish the code early, so that the community can be involved
early on in the development process, too.

**Important note:** Until the first stable version (i.e. `v1.0`) is released,
breaking changes between the releases must be expected. We will do our best
to indicate all breaking changes (and how to un-break them) in the
[Changelog](CHANGELOG.md)

## Contributing

You are welcome to contribute to this project by means of raising issues for
bugs, sending & discussing enhancment ideas or by contributing code via pull
requests.

In any case, please be sure that you have read & understood the currently known
design limitations before raising issues.

Also, if you want to contribute code, please make sure that your code

* has its functionality covered by unit tests (coverage goal is 80%),
* is correctly linted,
* is well commented,
* and last but not least is compatible with our license and CLA

## License

`argocd-image-updater` is open source software, released under the
[Apache 2.0 license](https://www.apache.org/licenses/LICENSE-2.0)

## Things that are planned (roadmap)

The following things are on the roadmap until the `v1.0` release

* Extend Argo CD functionality to be able to update images for other types of
  applications.

* Extend Argo CD functionality to write back to Git

* Provide web hook support to trigger update check for a given image

* Use concurrency for updating multiple applications at once

* Improve error handling

* Support for image tags with i.e. Git commit SHAs

For more details, check out the
[v1.0.0 milestone](https://github.com/argoproj-labs/argocd-image-updater/milestone/1)

## Frequently asked questions

**Does it write back the changes to Git?**

No, and this feature is also not planned for any of the next releases. We think
it's close to impossible to get such a feature 100% correctly working, because
there are so many edge-cases to consider (i.e. possible merge conflicts) and
there's no easy way to find out where a certain resource lives in Git when
manifests are rendered through a tool. We are thinking about how to implement
this feature in the meanwhile, but don't expect it before the v1.0 release.

**How does it persist the changes then?**

The Argo CD Image Updater leverages the Argo CD API to set application paramaters,
and Argo CD will then persist the change in the application's manifest. This is
something Argo CD will not overwrite upon the next manual (or automatic) sync,
except when the overrides are explicitly set in the manifest.

**Are there plans to extend functionality beyond Kustomize or Helm?**

Not yet, since we are dependent upon what functionality Argo CD provides for
these types of applications.

**Will it ever be fully integrated with Argo CD?**

In the current form, probably not. If there is community demand for it, let's
see how we can make this happen.
