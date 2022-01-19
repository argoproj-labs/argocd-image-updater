# End-to-end tests

This directory contains the end-to-end tests for Argo CD Image Updater. The
tests are implemented using [kuttl](https://kuttl.dev) and require some
prerequisites.

**This is work-in-progress at a very early stage**. The end-to-end tests are
not yet expected to work flawlessly, and they require an opinionated setup to
run. If you are going to use the end-to-end tests, it is expected that you are
prepared to hack on them. Do not ask for support, please.

# Components

The end-to-end tests are comprised of the following components:

* A local, vanilla K8s cluster that is treated as volatile. The tests only
  support k3s as a cluster at the moment.
* A dedicated Argo CD installation. No other Argo CD must be installed to
  the test cluster.
* A Git repository, containing resources to be consumed by Argo CD.
  This will be deployed on demand to the test cluster, with test data that
  is provided by the end-to-end tests.
* A Docker registry, holding the container images we use for testing.
  This will be deployed on demand to the test cluster.

## Local cluster

### Cluster installation

1. Install a recent version of [k3s](https://k3s.io/) on your local machine.
   If you want to re-use your k3s cluster, be aware that the test suite needs
   changes to the cluster's configuration, i.e. it will set up a custom
   container registry and credentials.

2. Run `./bin/install.sh`. This will

    * Configure your Docker daemon to be able to push to the test registry in
      an insecure manner.

    * Configure K3s to be able to access the test registry

3. Create required namespace in the cluster: `kubectl create ns argocd-image-updater-e2e`

## Pre-requisites

1. Run `make install-prereqs` to install all the pre-requisites on your local
   cluster.
