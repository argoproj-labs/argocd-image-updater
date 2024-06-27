#! /bin/bash


kubeconform -v >/dev/null 2>&1
if [ $? -ne 0 ]; then
  echo "Error: 'kubeconform' is not installed."
  exit 1
fi


kubeconform \
    -schema-location default \
    -schema-location 'https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/kustomize.toolkit.fluxcd.io/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json' \
    -strict \
    -verbose \
    -summary \
    ${MANIFESTS_DIRECTORY:-"manifests"}

