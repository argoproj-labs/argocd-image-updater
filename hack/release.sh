#!/bin/sh

# Simple script to do a release

TARGET_REMOTE=upstream
TARGET_VERSION="$1"
set -eu
set -o pipefail

if test "${TARGET_VERSION}" = ""; then
	echo "USAGE: $0 <version>" >&2
	exit 1
fi

TARGET_TAG="v${TARGET_VERSION}"

if ! echo "${TARGET_VERSION}" | egrep -q '^[0-9]+\.[0-9]+\.[0-9]+$'; then
	echo "Error: Target version '${TARGET_VERSION}' is not well-formed. Must be X.Y.Z" >&2
	exit 1
fi

echo "*** checking for current git branch"
RELEASE_BRANCH=$(git rev-parse --abbrev-ref HEAD || true)
if [[ $RELEASE_BRANCH = release-* ]]; then
	echo "***   branch is $RELEASE_BRANCH"
	IMAGE_TAG=${TARGET_TAG}
else
	echo "Error: Branch $RELEASE_BRANCH is not release branch" >&2
	exit 1
fi

if ! test -f VERSION; then
	echo "Error: You should be in repository root." >&2
	exit 1
fi

echo "${TARGET_VERSION}" > VERSION

echo "*** checking for existence of git tag ${TARGET_TAG}"
if git tag -l "${TARGET_TAG}" | grep -q "${TARGET_TAG}"; then
	echo "Error: Tag with version ${TARGET_TAG} already exists." >&2
	exit 1
fi

echo "*** generating new manifests"
export IMAGE_TAG="${TARGET_TAG}"
make manifests

echo "*** performing release commit"
git commit -S -s -m "Release ${TARGET_TAG}" VERSION manifests/
git tag ${TARGET_TAG}

echo "*** build multiarch docker image"
make multiarch-image

echo "*** build multiarch release binaries"
make release-binaries

echo
echo "*** done"
echo
echo "If everything is fine, push changes to GitHub and Docker Hub:"
echo 
echo "   git push ${TARGET_REMOTE} $RELEASE_BRANCH ${TARGET_TAG}"
echo "   make IMAGE_TAG='${TARGET_TAG}' multiarch-image-push"
echo
echo "Then, create release tag and execute upload-multiarch-release-assets.sh"
