# docker image publishing options
DOCKER_PUSH?=false
IMAGE_NAMESPACE?=
IMAGE_TAG?=latest
ifdef IMAGE_NAMESPACE
IMAGE_PREFIX=${IMAGE_NAMESPACE}/
endif

.PHONY: all
all: clean mod-tidy lint test

.PHONY: clean
clean: clean-image
	rm -rf vendor/

.PHONY: clean-image
clean-image:
	rm -rf dist/
	rm -f coverage.out

.PHONY: mod-tidy
mod-tidy:
	go mod download && go mod tidy && go mod vendor

.PHONY: test
test:
	go test -race ./... -coverprofile=coverage.out

.PHONY: lint
lint:
	golangci-lint run

.PHONY: registry-scanner-image
registry-scanner-image:
	docker build -t $(IMAGE_PREFIX)registry-scanner . -f Dockerfile
	@if [ "$(DOCKER_PUSH)" = "true" ] ; then docker push $(IMAGE_PREFIX)registry-scanner:$(IMAGE_TAG) ; fi

