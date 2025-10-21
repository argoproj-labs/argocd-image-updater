#!/bin/bash

### This script creates a new release PR
# - install gh cli and semver-cli (go install github.com/davidrjonas/semver-cli@latest)
# - create and push "release-X.Y" branch
# - checkout this branch locally
# - run this script from repo root: ./hack/create-release-pr.sh [REMOTE]
# - merge the PR
# It will trigger the release workflow that would create release draft on github

set -eux
set -o pipefail


CURRENT_BRANCH="$(git branch --show-current)"

if [[ ! "$CURRENT_BRANCH" == release-* ]]; then
	echo "!! Please checkout branch 'release-X.Y' (currently in branch: '${CURRENT_BRANCH}')" >&2
	exit 1
fi

RELEASE_BRANCH="${CURRENT_BRANCH}"

REMOTE=${1:-origin}
REMOTE_URL=$(git remote get-url ${REMOTE})

if [[ ! $(git ls-remote --exit-code ${REMOTE_URL} ${RELEASE_BRANCH}) ]]; then
    echo "!! Please make sure '${RELEASE_BRANCH}' exists in remote '${REMOTE}'" >&2
    exit 1
fi

### look for latest on-branch tag
PREVIOUS_TAG=$(git describe --tags --abbrev=0 --match "v${RELEASE_BRANCH##release-}*" 2>/dev/null || true)

if [ -n "$PREVIOUS_TAG" ]; then
    NEW_VERSION=$(semver-cli inc patch $PREVIOUS_TAG)
else
    NEW_VERSION="${RELEASE_BRANCH##release-}.0"
fi

echo $NEW_VERSION > VERSION
IMAGE_TAG="v${NEW_VERSION}"
# Update manifests
make build-installer

# Create PR for the release
git checkout -b "feat/new-version-${NEW_VERSION}"

# Commit and push the changes
git commit -m "Release ${NEW_VERSION}" VERSION config/
git push --set-upstream ${REMOTE} "feat/new-version-${NEW_VERSION}"
gh label --repo ${REMOTE_URL} create --force release
gh pr --repo ${REMOTE_URL} \
    create \
    --base ${RELEASE_BRANCH} \
    --title "Release ${NEW_VERSION}" \
    --body  "Release ${NEW_VERSION}" \
    --label release
git checkout -
