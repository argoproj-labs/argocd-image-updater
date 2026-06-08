# docker image publishing options
DOCKER_PUSH?=false
IMAGE_NAMESPACE?=
IMAGE_TAG?=latest
ifdef IMAGE_NAMESPACE
IMAGE_PREFIX=${IMAGE_NAMESPACE}/
endif

# Version of github.com/distribution/distribution/v3 whose internal/ packages
# are vendored into pkg/registry/internal/. Override on the command line when
# upgrading: make get-distribution-internal DISTRIBUTION_VERSION=v3.1.2
DISTRIBUTION_VERSION ?= v3.1.1
DISTRIBUTION_INTERNAL_DST := pkg/registry/internal

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

.PHONY: get-distribution-internal
get-distribution-internal: ## Refresh $(DISTRIBUTION_INTERNAL_DST) from upstream distribution/distribution at DISTRIBUTION_VERSION. Usage: make get-distribution-internal DISTRIBUTION_VERSION=v3.1.2
	@echo "Fetching github.com/distribution/distribution $(DISTRIBUTION_VERSION) internal/ ..."
	@tmpdir=$$(mktemp -d); \
	 curl -sSL https://github.com/distribution/distribution/archive/refs/tags/$(DISTRIBUTION_VERSION).tar.gz \
	   | tar -xz -C "$$tmpdir"; \
	 cp $(DISTRIBUTION_INTERNAL_DST)/README.md "$$tmpdir/README.md.bak"; \
	 rm -rf $(DISTRIBUTION_INTERNAL_DST); \
	 cp -r "$$tmpdir/distribution-$(subst v,,$(DISTRIBUTION_VERSION))/internal" $(DISTRIBUTION_INTERNAL_DST); \
	 mv "$$tmpdir/README.md.bak" $(DISTRIBUTION_INTERNAL_DST)/README.md; \
	 rm -rf "$$tmpdir"; \
	 echo ""; \
	 echo "Done. Next steps:"; \
	 echo "  1. The copied files contain upstream import paths that must be fixed manually:"; \
	 echo "       FROM: github.com/distribution/distribution/v3/internal/"; \
	 echo "       TO:   github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/registry/internal/"; \
	 echo "     Revert any import changes shown by git diff that deviate from the TO path above."; \
	 echo "  2. Run 'git diff $(DISTRIBUTION_INTERNAL_DST)' to review all other changes."


