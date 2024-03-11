IMAGE_NAMESPACE?=registry.sima-land.ru/devops
IMAGE_NAME=argocd-image-updater
IMAGE_TAG?=v0.12.2--sl-devops-clean
ifdef IMAGE_NAMESPACE
IMAGE_PREFIX=${IMAGE_NAMESPACE}/
else
IMAGE_PREFIX=
endif
IMAGE_PUSH?=no
OS?=$(shell go env GOOS)
ARCH?=$(shell go env GOARCH)
OUTDIR?=dist
BINNAME?=argocd-image-updater

CURRENT_DIR=$(shell pwd)
VERSION=$(shell cat ${CURRENT_DIR}/VERSION)
GIT_COMMIT=$(shell git rev-parse HEAD)
BUILD_DATE=$(shell date -u +'%Y-%m-%dT%H:%M:%SZ')

LDFLAGS=

RELEASE_IMAGE_PLATFORMS?=linux/amd64,linux/arm64

VERSION_PACKAGE=github.com/argoproj-labs/argocd-image-updater/pkg/version
ifeq ($(IMAGE_PUSH), yes)
DOCKERX_PUSH=--push
else
DOCKERX_PUSH=
endif

override LDFLAGS += -extldflags "-static"
override LDFLAGS += \
	-X ${VERSION_PACKAGE}.version=${VERSION} \
	-X ${VERSION_PACKAGE}.gitCommit=${GIT_COMMIT} \
	-X ${VERSION_PACKAGE}.buildDate=${BUILD_DATE}

.PHONY: all
all: prereq controller

.PHONY: clean
clean: clean-image
	rm -rf vendor/

.PHONY: clean-image
clean-image:
	rm -rf dist/
	rm -f coverage.out

.PHONY: mod-tidy
mod-tidy:
	go mod tidy

.PHONY: mod-download
mod-download:
	go mod download

.PHONY: mod-vendor
mod-vendor:
	go mod vendor

.PHONY: test
test:
	go test -coverprofile coverage.out `go list ./... | egrep -v '(test|mocks|ext/)'`

test-race:
	go test -race -coverprofile coverage.out `go list ./... | egrep -v '(test|mocks|ext/)'`

.PHONY: prereq
prereq:
	mkdir -p dist

.PHONY: controller
controller:
	CGO_ENABLED=0 GOOS=${OS} GOARCH=${ARCH} go build -ldflags '${LDFLAGS}' -o ${OUTDIR}/${BINNAME} cmd/*.go

.PHONY: image
image: clean-image
	docker build \
		--platform linux/amd64 \
		-t ${IMAGE_PREFIX}${IMAGE_NAME}:${IMAGE_TAG} \
		--pull \
		.

.PHONY: multiarch-image
multiarch-image:
	docker buildx build \
		-t ${IMAGE_PREFIX}${IMAGE_NAME}:${IMAGE_TAG} \
		--progress plain \
		--pull \
		--platform ${RELEASE_IMAGE_PLATFORMS} ${DOCKERX_PUSH} \
		.

.PHONY: multiarch-image
multiarch-image-push:
	docker buildx build \
		-t ${IMAGE_PREFIX}${IMAGE_NAME}:${IMAGE_TAG} \
		--progress plain \
		--pull \
		--push \
		--platform ${RELEASE_IMAGE_PLATFORMS} ${DOCKERX_PUSH} \
		.

.PHONY: image-push
image-push: image
	docker push ${IMAGE_PREFIX}${IMAGE_NAME}:${IMAGE_TAG}

.PHONY: release-binaries
release-binaries:
	BINNAME=argocd-image-updater-linux_amd64 OUTDIR=dist/release OS=linux ARCH=amd64 make controller
	BINNAME=argocd-image-updater-linux_arm64 OUTDIR=dist/release OS=linux ARCH=arm64 make controller
	BINNAME=argocd-image-updater-darwin_amd64 OUTDIR=dist/release OS=darwin ARCH=amd64 make controller
	BINNAME=argocd-image-updater-darwin_arm64 OUTDIR=dist/release OS=darwin ARCH=arm64 make controller
	BINNAME=argocd-image-updater-win64.exe OUTDIR=dist/release OS=windows ARCH=amd64 make controller

.PHONY: extract-binary
extract-binary:
	docker rm argocd-image-updater-${IMAGE_TAG} || true
	docker create --name argocd-image-updater-${IMAGE_TAG} ${IMAGE_PREFIX}${IMAGE_NAME}:${IMAGE_TAG}
	docker cp argocd-image-updater-${IMAGE_TAG}:/usr/local/bin/argocd-image-updater /tmp/argocd-image-updater_${IMAGE_TAG}_linux-amd64
	docker rm argocd-image-updater-${IMAGE_TAG}

.PHONY: lint
lint:
	golangci-lint run

.PHONY: manifests
manifests:
	IMAGE_NAMESPACE=${IMAGE_NAMESPACE} ./hack/generate-manifests.sh

.PHONY: codegen
codegen: manifests

.PHONY: run-test
run-test:
	docker run -v $(HOME)/.kube:/kube --rm -it \
		-e ARGOCD_TOKEN \
		${IMAGE_PREFIX}${IMAGE_NAME}:${IMAGE_TAG} \
		--kubeconfig /kube/config \
		--argocd-server-addr $(ARGOCD_SERVER) \
		--grpc-web
