#!/bin/sh

set -eu
set -o pipefail

if ! test -f VERSION; then
	echo "Error: VERSION file not found" >&2
fi

TARGET_VERSION="v$(cat VERSION)"

echo "*** extracting release assets for $TARGET_VERSION"

TEMP_DIR=$(mktemp -d /tmp/argocd-image-updater.${TARGET_VERSION}.XXXXXXX)
TARGET_BIN=argocd-image-updater_${TARGET_VERSION}_linux-amd64

cid=$(docker create argoprojlabs/argocd-image-updater:${TARGET_VERSION})
docker cp $cid:/usr/local/bin/argocd-image-updater ${TEMP_DIR}/${TARGET_BIN}
docker rm -v ${cid}

echo "*** uploading release assets"
echo "***    ${TARGET_BIN}"
gh release upload ${TARGET_VERSION} ${TEMP_DIR}/${TARGET_BIN}

echo "*** deleting temp directory"
rm -rf "${TEMP_DIR}"
