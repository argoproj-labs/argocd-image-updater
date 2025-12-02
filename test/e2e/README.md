# End-to-end tests

This directory contains the end-to-end tests for Argo CD Image Updater. The
tests are implemented using [kuttl](https://kuttl.dev) and require some
prerequisites.

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

2. Install CRDs into the K8s cluster specified in ~/.kube/config.

    * `make install`

3. Run `./bin/install.sh`. This will

    * Configure your Docker daemon to be able to push to the test registry in
      an insecure manner.

    * Configure K3s to be able to access the test registry

4. Create required namespaces in the cluster: 
   * `kubectl create ns argocd-image-updater-e2e`

## Pre-requisites

1. Run `make install-prereqs` to install all the pre-requisites on your local
   cluster.

## Run the test suite

1. Run `./e2e-test.sh` to run all the tests located in `./suite`.
