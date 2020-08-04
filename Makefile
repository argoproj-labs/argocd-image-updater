LDFLAGS=-extldflags "-static"

IMAGE_NAMESPACE?=argoproj-labs
IMAGE_TAG?=latest
IMAGE_NAME=argocd-image-updater
ifdef IMAGE_NAMESPACE
IMAGE_PREFIX=${IMAGE_NAMESPACE}/
else
IMAGE_PREFIX=
endif

all: prereq controller

.PHONY: clean
clean: clean-image
	rm -rf vendor/

.PHONY: clean-image
clean-image:
	rm -rf dist/
	rm -f coverage.out

mod-tidy:
	go mod tidy

mod-download:
	go mod download

mod-vendor:
	go mod vendor

.PHONY: test
test:
	go test -coverprofile coverage.out `go list ./... | egrep -v '(test|mocks)'`

.PHONY: prereq
prereq:
	mkdir -p dist

.PHONY: controller
controller: 
	CGO_ENABLED=0 go build -o dist/argocd-image-controller cmd/main.go

.PHONY: image
image: clean-image mod-vendor
	docker build -t ${IMAGE_PREFIX}${IMAGE_NAME}:${IMAGE_TAG} .
	rm -rf vendor/

.PHONY: manifests
manifests:
	./hack/generate-manifests.sh

.PHONY: run-test
run-test:
	docker run -v $(HOME)/.kube:/kube --rm -it \
		-e ARGOCD_TOKEN \
		argocd-image-controller \
		--kubeconfig /kube/config \
		--argocd-server-addr $(ARGOCD_SERVER) \
		--grpc-web
