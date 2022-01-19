#!/bin/sh
set -e
set -o pipefail

E2E_NAMESPACE=argocd-image-updater-e2e
E2E_TIMEOUT=120
E2E_REGISTRY_NOAUTH="10.42.0.1:30000"
E2E_REGISTRY_AUTH="10.42.0.1:30001"

SRC_DIR="$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )/../.."
export SRC_DIR

restart_registry() {
	t="$1"
	kubectl -n $E2E_NAMESPACE rollout restart deployment e2e-registry-$t
	kubectl -n $E2E_NAMESPACE rollout status deployment e2e-registry-$t
}

restart_repository() {
	kubectl -n $E2E_NAMESPACE rollout restart deployment e2e-repository
	kubectl -n $E2E_NAMESPACE rollout status deployment e2e-repository
}

build_image_if_not_exist() {
	name="$1"
	tag="$2"
	found=$(docker images --format '{{.Repository}}:{{.Tag}}' $name:$tag)
	if ! test "$found" = "$name:$tag"; then
		(
			cd images
			IMAGE_TAG=$tag make build-and-push
		)
	fi
}

prepare_registry() {
	make git-container-build
	restart_registry public
	restart_registry private
	make git-container-push
	restart_repository
	(
		cd images
		IMAGE_TAG=1.0.0 make push
		IMAGE_TAG=1.0.1 make push
		IMAGE_TAG=1.0.2 make push
		IMAGE_TAG=latest make push
	)
}

if ! kubectl kuttl version >/dev/null 2>&1; then
	echo "kuttl seems not installed; aborting" >&2
	exit 1
fi

for tag in 1.0.0 1.0.1 1.0.2 2.0.0 2.0.1 2.1.0 latest; do
	build_image_if_not_exist "10.42.0.1:30000/test-image" "$tag"
done

prepare_registry

kubectl kuttl test --namespace ${E2E_NAMESPACE} --timeout ${E2E_TIMEOUT} $*
