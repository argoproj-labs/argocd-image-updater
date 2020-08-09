# Developing

## Requirements

Getting started to develop Argo CD Image Updater shouldn't be too hard. All that
is required is a simple build toolchain, consisting of:

* Golang v1.14
* GNU make
* Docker (for building images, optional)
* Kustomize (for building K8s manifests, optional)

## Makefile targets

Most steps in the development process are scripted in the `Makefile`, the most
important targets are:

* `all` - this is the default target, and will build the `argocd-image-updater`
  binary.

* `lint` - this will run `golangci-lint` and ensure code is linted correctly.

* `test` - this will run all the unit tests

* `image` - this will build the Docker image

* `manifests` - this will build the installation manifests for Kubernetes from
  the Kustomize sources

## Sending Pull Requests

To send a pull request, simply fork the
[GitHub repository](https://github.com/argoproj-labs/argocd-image-updater)
to your GitHub account, create a new branch, commit & push your changes and then
send the PR over for review.

When developing new features or fixing bugs, please make sure that your code is
accompanied by appropriate unit tests. If you are fixing a bug, please also
include a unit test for that specific bug.

Also, please make sure that your code is correctly linted.
