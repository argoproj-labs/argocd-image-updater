#!/bin/bash

RELEASE_BRANCH="$(git rev-parse --abbrev-ref HEAD || true)"
set -eux
set -o pipefail

### look for latest on-branch tag
PREVIOUS_TAG=$(git describe --tags --abbrev=0 --match "*${RELEASE_BRANCH##release-}*" 2>/dev/null || true)

if [ -n "$PREVIOUS_TAG" ]; then
    NEW_VERSION=$(semver-cli inc patch $PREVIOUS_TAG)
else
    NEW_VERSION="${RELEASE_BRANCH##release-}.0"
fi

echo $NEW_VERSION > VERSION

IMAGE_TAG="v${NEW_VERSION}"
make manifests

git checkout -b "feat/new-version-${NEW_VERSION}"
git commit -m "Release ${NEW_VERSION}" VERSION manifests/
git push
gh label --repo $(git remote get-url origin) create --force release
gh pr --repo $(git remote get-url origin) \
    create \
    --base ${RELEASE_BRANCH} \
    --title "Release ${NEW_VERSION}" \
    --body  "Release ${NEW_VERSION}" \
    --label release
git checkout -
