#!/bin/bash

### This script creates a new release Tag for Registry Scanner
# - install gh cli and semver-cli (go install github.com/davidrjonas/semver-cli@latest)
# - create and push "registry-scanner/release-X.Y" branch
# - checkout this branch locally
# - run this script from repo registry-scanner module: ./hack/create-release.sh [TARGET_VERSION] [REMOTE]
# - merge the PR

TARGET_VERSION="$1"
set -eu
set -o pipefail

if test "${TARGET_VERSION}" = ""; then
	echo "USAGE: $0 <version> <remote>" >&2
	exit 1
fi

CURRENT_BRANCH="$(git branch --show-current)"
SUBMODULE_NAME="registry-scanner"

if [[ ! "${CURRENT_BRANCH}" == registry-scanner/release-* ]]; then
	echo "!! Please checkout branch 'registry-scanner/release-X.Y' (currently in branch: '${CURRENT_BRANCH}')" >&2
	exit 1
fi

RELEASE_BRANCH="${CURRENT_BRANCH}"

REMOTE=${2:-origin}
REMOTE_URL=$(git remote get-url "${REMOTE}")

if [[ ! $(git ls-remote --exit-code ${REMOTE_URL} ${RELEASE_BRANCH}) ]]; then
    echo "!! Please make sure '${RELEASE_BRANCH}' exists in remote '${REMOTE}'" >&2
    exit 1
fi

NEW_TAG="${SUBMODULE_NAME}/v${TARGET_VERSION}"

### look for latest on-branch tag to check if it matches the NEW_TAG
PREVIOUS_TAG=$(git describe --tags --abbrev=0 --match "${SUBMODULE_NAME}/*" 2>/dev/null || true)

if [[ "${PREVIOUS_TAG}" == "${NEW_TAG}" ]]; then
    echo "!! Tag ${NEW_TAG} already exists" >&2
    exit 1
fi

echo "Creating tag ${NEW_TAG}"
echo "${TARGET_VERSION}" > VERSION

# Create tag for registry-scanner
git tag "${NEW_TAG}"
git push "${REMOTE}" tag "${NEW_TAG}"

