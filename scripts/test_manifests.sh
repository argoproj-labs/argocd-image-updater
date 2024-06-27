#! /bin/bash

set -e

kubeconform -schema-location default -schema-location 'https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/kustomize.toolkit.fluxcd.io/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json' -verbose /home/manifests

