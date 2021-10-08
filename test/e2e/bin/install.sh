#!/bin/sh
set -e
set -o pipefail
set -x

BASE_DIR="$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )/.."

sudo mkdir -p /etc/rancher/k3s
sudo mkdir -p /etc/docker

sudo cp ${BASE_DIR}/assets/registries.yaml /etc/rancher/k3s/registry.yaml
sudo cp ${BASE_DIR}/assets/registry.crt /etc/rancher/k3s/local.crt
sudo cp ${BASE_DIR}/assets/daemon.json /etc/docker/daemon.json

sudo systemctl restart k3s docker
