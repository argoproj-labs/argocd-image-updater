#!/bin/sh

set -eu
set -o pipefail

if ! test -f VERSION; then
	echo "Error: VERSION file not found" >&2
    exit 1
fi

TARGET_VERSION="v$(cat VERSION)"

if ! test -d dist/release; then
    echo "Error: dist/release directory does not exist" >&2
    exit 1
fi

cd dist/release
rm -f release-${TARGET_VERSION}.sha256 release-${TARGET_VERSION}.sha256.asc
BINARIES=
echo "*** Generating SHA256 checksums for binaries"
for bin in argocd-image-updater-*; do
    sha256sum $bin >> release-${TARGET_VERSION}.sha256
    BINARIES="${BINARIES} $bin"
done

echo "*** Signing checksum file with GPG key"
gpg -a --detach-sign release-${TARGET_VERSION}.sha256

echo "*** Reverse verify signature"
gpg -a --verify release-${TARGET_VERSION}.sha256.asc

echo "*** Uploading release assets"
for asset in ${BINARIES} release-${TARGET_VERSION}.sha256 release-${TARGET_VERSION}.sha256.asc; do
    echo "     -> $asset"
    gh release upload ${TARGET_VERSION} ${asset}
done

echo "Done."
