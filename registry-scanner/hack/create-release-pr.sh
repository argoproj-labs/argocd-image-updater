#!/bin/bash

### This script creates a new release PR
# - install gh cli and semver-cli (go install github.com/davidrjonas/semver-cli@latest)
# - create and push "release-X.Y" branch
# - checkout this branch locally
# - run this script from repo root: ./hack/create-release-pr.sh [REMOTE]
# - merge the PR
# It will trigger the release workflow that would create release draft on github

TARGET_VERSION="$1"
set -eu
set -o pipefail

if test "${TARGET_VERSION}" = ""; then
	echo "USAGE: $0 <version>" >&2
	exit 1
fi

CURRENT_BRANCH="$(git branch --show-current)"
SUBMODULE_NAME="registry-scanner"

if [[ ! "$CURRENT_BRANCH" == release-* ]]; then
	echo "!! Please checkout branch 'release-X.Y' (currently in branch: '${CURRENT_BRANCH}')" >&2
	exit 1
fi

RELEASE_BRANCH="${CURRENT_BRANCH}"

REMOTE=${2:-origin}
REMOTE_URL=$(git remote get-url "${REMOTE}")

if [[ ! $(git ls-remote --exit-code ${REMOTE_URL} ${RELEASE_BRANCH}) ]]; then
    echo "!! Please make sure '${RELEASE_BRANCH}' exists in remote '${REMOTE}'" >&2
    exit 1
fi

NEW_TAG="registry-scanner/v${TARGET_VERSION}"

### look for latest on-branch tag to check if it matches the NEW_TAG
PREVIOUS_TAG=$(git describe --tags --abbrev=0 --match "${SUBMODULE_NAME}/*" 2>/dev/null || true)

if [ "${PREVIOUS_TAG}" == "${NEW_TAG}" ]; then
    echo "!! Tag ${NEW_TAG} already exists" >&2
    exit 1
fi

echo "Creating tag ${NEW_TAG}"
echo "${TARGET_VERSION}" > VERSION

# Create tag for registry-scanner
git tag "${NEW_TAG}"
git push "${REMOTE}" tag "${NEW_TAG}"

