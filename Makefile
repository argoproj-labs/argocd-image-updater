IMAGE_NAMESPACE?=quay.io/argoprojlabs
IMAGE_NAME=argocd-image-updater
IMAGE_TAG?=latest
ifdef IMAGE_NAMESPACE
IMAGE_PREFIX=${IMAGE_NAMESPACE}/
else
IMAGE_PREFIX=
endif

CURRENT_DIR=$(shell pwd)
VERSION=$(shell cat ${CURRENT_DIR}/VERSION)
GIT_COMMIT=$(shell git rev-parse HEAD)
BUILD_DATE=$(shell date -u +'%Y-%m-%dT%H:%M:%SZ')

LDFLAGS=

VERSION_PACKAGE=github.com/argoproj-labs/argocd-image-updater/pkg/version

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

.PHONY: prereq
prereq:
	mkdir -p dist

.PHONY: controller
controller: 
	CGO_ENABLED=0 go build -ldflags '${LDFLAGS}' -o dist/argocd-image-updater cmd/*.go

.PHONY: image
image: clean-image mod-vendor
	docker build -t ${IMAGE_PREFIX}${IMAGE_NAME}:${IMAGE_TAG} .
	rm -rf vendor/

.PHONY: image-push
image-push: image
	docker push ${IMAGE_PREFIX}${IMAGE_NAME}:${IMAGE_TAG}

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
