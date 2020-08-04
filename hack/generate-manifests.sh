#!/bin/sh

set -eo pipefail
set -x

SRCROOT="$( CDPATH='' cd -- "$(dirname "$0")/.." && pwd -P )"
KUSTOMIZE="kustomize"
TEMPFILE=$(mktemp /tmp/aic-manifests.XXXXXX)

IMAGE_NAMESPACE="${IMAGE_NAMESPACE:-argoprojlabs}"
IMAGE_TAG="${IMAGE_TAG:-}"

# if the tag has not been declared, and we are on a release branch, use the VERSION file.
if [ "$IMAGE_TAG" = "" ]; then
  branch=$(git rev-parse --abbrev-ref HEAD || true)
  if [[ $branch = release-* ]]; then
    pwd
    IMAGE_TAG=v$(cat $SRCROOT/VERSION)
  fi
fi
# otherwise, use latest
if [ "$IMAGE_TAG" = "" ]; then
  IMAGE_TAG=latest
fi

cd ${SRCROOT}/manifests/base && kustomize edit set image argoprojlabs/argocd-image-updater=${IMAGE_NAMESPACE}/argocd-image-updater:${IMAGE_TAG}
cd ${SRCROOT}/manifests/base && ${KUSTOMIZE} build . > ${TEMPFILE}

mv ${TEMPFILE} ${SRCROOT}/manifests/install.yaml
cd ${SRCROOT} && chmod 644 manifests/install.yaml
