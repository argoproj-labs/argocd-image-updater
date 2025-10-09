#!/bin/bash

### This script creates a new release Tag for Registry Scanner
# - install gh cli and semver-cli (go install github.com/davidrjonas/semver-cli@latest)
# - create and push "registry-scanner/release-X.Y" branch
# - checkout this branch locally
# - run this script from repo registry-scanner module: ./hack/create-release.sh [TARGET_VERSION] [REMOTE]
# - merge the PR
# Example Uses:
# ./hack/create-release.sh 0.1.1                                Would create a new tag "registry-scanner/v0.1.1" with
#                                                               the message "Release registry-scanner/v0.1.1" and edit 
#                                                               VERSION file to be 0.1.1 which would be committed.
#
# ./hack/create-release.sh 0.1.X upstream                       Would create a new tag "registry-scanner/v0.1.X" with
#                                                               the message "Relase registry-scanner/v0.1.X" and edit
#                                                               VERSION to be 0.1.X which would be committed. The contents
#                                                               would be pushed to the remote "upstream." 

TARGET_VERSION="$1"

USAGE_ERR="   USAGE: $0 [TARGET_VERSION] [REMOTE]"

set -eu
set -o pipefail

# Validate if arguments are present
if test "${TARGET_VERSION}" = ""; then
	printf "!! TARGET_VERSION is missing\n$USAGE_ERR\n" >&2
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
MESSAGE="Release ${NEW_TAG}"

### look for latest on-branch tag to check if it matches the NEW_TAG
PREVIOUS_TAG=$(git describe --tags --abbrev=0 --match "${SUBMODULE_NAME}/*" 2>/dev/null || true)

if [[ "${PREVIOUS_TAG}" == "${NEW_TAG}" ]]; then
    echo "!! Tag ${NEW_TAG} already exists" >&2
    exit 1
fi

echo "Creating tag ${NEW_TAG}"
echo "${TARGET_VERSION}" > VERSION

# Commit updated VERSION file
git add VERSION
git commit -s -m "${MESSAGE}" 

# Create tag for registry-scanner
git tag -a "${NEW_TAG}" -m "${MESSAGE}"
git push "${REMOTE}" "${RELEASE_BRANCH}" "${NEW_TAG}"
