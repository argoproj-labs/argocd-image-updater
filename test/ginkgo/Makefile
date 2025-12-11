# Assisted-by: Claude AI model

CURRENT_DIR=$(shell pwd)
CONTAINER_TOOL ?= docker
# Default image for argocd-operator. Can be overridden.
ARGOCD_OPERATOR_IMAGE ?= quay.io/argoprojlabs/argocd-operator:latest

# Get version from root VERSION file
IMAGE_NAMESPACE?=quay.io/argoprojlabs
IMAGE_NAME=argocd-image-updater
ifdef IMAGE_NAMESPACE
IMAGE_PREFIX=${IMAGE_NAMESPACE}/
else
IMAGE_PREFIX=
endif

VERSION := $(shell cat ../../VERSION)
IMAGE_TAG?=v${VERSION}
# Image URL to use all building/pushing image targets
ARGOCD_IMAGE_UPDATER_IMAGE ?= ${IMAGE_PREFIX}${IMAGE_NAME}:${IMAGE_TAG}

# Define the patch template
define PATCH_TEMPLATE
apiVersion: apps/v1
kind: Deployment
metadata:
  name: controller-manager
  namespace: system
spec:
  template:
    spec:
      containers:
      - name: manager
        env:
        - name: ARGOCD_IMAGE_UPDATER_IMAGE
          value: $(ARGOCD_IMAGE_UPDATER_IMAGE)
endef
export PATCH_TEMPLATE

# Tools - assuming they are in the path or in the project's bin directory
KUSTOMIZE ?= $(CURDIR)/../../bin/kustomize
KUBECTL ?= kubectl
K3D ?= k3d

K3D_CLUSTER_NAME ?= test-e2e
LOCAL ?= true

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: deploy-argocd-operator undeploy-argocd-operator kustomize test-e2e \
   k3d-cluster-create k3d-cluster-delete k3d-image-import configure-docker-daemon \
   git-container-build git-container-push create-local-container-registry

kustomize:
	$(MAKE) -C ../../ kustomize

deploy-argocd-operator: kustomize ## Deploy argocd-operator from a stable git reference.
	@echo "Deploying Argo CD Operator..."
	@set -e; \
	TMP_DIR=$$(mktemp -d); \
	cp prereqs/kustomization.yaml $$TMP_DIR/kustomization.yaml; \
	echo "Applying argocd-operator manifests with image $(ARGOCD_OPERATOR_IMAGE)..."; \
	echo "Setting argocd-image-updater image to $(ARGOCD_IMAGE_UPDATER_IMAGE)..."; \
	echo "$$PATCH_TEMPLATE" > $$TMP_DIR/patch.yaml; \
	cd $$TMP_DIR && \
		$(KUSTOMIZE) edit set image quay.io/argoprojlabs/argocd-operator=$(ARGOCD_OPERATOR_IMAGE) && \
		$(KUSTOMIZE) edit add patch --path patch.yaml; \
	$(KUSTOMIZE) build $$TMP_DIR | $(KUBECTL) apply --server-side=true --validate=false -f -; \
	rm -rf $$TMP_DIR; \
	echo "Argo CD Operator deployment initiated.";
	echo "--- Waiting for argocd-operator to be ready ---";
	$(KUBECTL) wait --for=condition=available --timeout=300s deployment/argocd-operator-controller-manager -n argocd-operator-system

undeploy-argocd-operator: kustomize ## Undeploy argocd-operator.
	@echo "Undeploying Argo CD Operator..."
	@set -e; \
	TMP_DIR=$$(mktemp -d); \
	cp prereqs/kustomization.yaml $$TMP_DIR/kustomization.yaml; \
	$(KUSTOMIZE) build $$TMP_DIR | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -; \
	rm -rf $$TMP_DIR; \
	echo "Argo CD Operator undeployment initiated.";

k3d-cluster-create: ## Create k3d cluster for e2e tests
	@echo "--- Creating k3d cluster $(K3D_CLUSTER_NAME) ---"
	@$(K3D) cluster create $(K3D_CLUSTER_NAME) \
	--port '30000:30000@server:0' \
	--registry-config "${CURRENT_DIR}/prereqs/assets/registries.yaml"
	@bash -c '. "${CURRENT_DIR}/prereqs/assets/wait-utils.sh" && wait_for_cluster_ready 120 $(KUBECTL)' || exit 1

k3d-cluster-delete: ## Delete k3d cluster for e2e tests
	@echo "--- Deleting k3d cluster $(K3D_CLUSTER_NAME) ---"
	$(K3D) cluster delete $(K3D_CLUSTER_NAME)

configure-docker-daemon: ## Configure Docker daemon.json to allow insecure registry at 127.0.0.1:30000
	@echo "--- Configuring Docker daemon.json for insecure registry 127.0.0.1:30000 ---"
	@bash "${CURRENT_DIR}/prereqs/assets/configure-docker-daemon.sh"

k3d-image-import: ## Import local image to k3d cluster
	@echo "--- Importing image $(ARGOCD_IMAGE_UPDATER_IMAGE) to k3d cluster $(K3D_CLUSTER_NAME) ---"
	$(K3D) image import $(ARGOCD_IMAGE_UPDATER_IMAGE) -c $(K3D_CLUSTER_NAME)

# Currently run only parallel tests because we don't have sequential tests yet.
test-e2e: ## Build local image updater, deploy operator, and run parallel e2e tests.
	@echo "--- Configuring Docker daemon for insecure registry ---"
	$(MAKE) configure-docker-daemon
	@echo "--- Building local argocd-image-updater image ---"
	$(MAKE) -C ../../ docker-build
	@echo "--- Creating k3d cluster---"
	$(MAKE) k3d-cluster-create
	@echo "--- Importing image to k3d cluster ---"
	$(MAKE) k3d-image-import
	@echo "--- Deploying argocd-operator with local image-updater ---"
	$(MAKE) deploy-argocd-operator
	@echo "--- Deploying local container registry ---"
	$(MAKE) create-local-container-registry
	@echo "--- Running Parallel E2E tests ---"
	$(MAKE) -C ../../ e2e-tests-parallel-ginkgo
ifeq ($(LOCAL),true)
	@echo "--- Deleting k3d cluster ---"
	$(MAKE) k3d-cluster-delete
endif

GIT_IMAGE_REPO		?= 127.0.0.1:30000
GIT_IMAGE_NAME		?= git-http
GIT_IMAGE_TAG		?= latest

GIT_IMAGE_SLUG		:= $(GIT_IMAGE_REPO)/$(GIT_IMAGE_NAME):$(GIT_IMAGE_TAG)

git-container-build: ## Build git container image
	$(CONTAINER_TOOL) build -t $(GIT_IMAGE_SLUG) -f ./prereqs/containers/git/Dockerfile ./prereqs/containers/git

git-container-push: git-container-build ## Push git container image
	$(CONTAINER_TOOL) push $(GIT_IMAGE_SLUG)

create-local-container-registry: ## Create local container registry
	@echo "--- Generating TLS certificates and secrets for registry ---"
	@bash "${CURRENT_DIR}/prereqs/assets/generate-registry-tls-secrets.sh"
	@echo "--- Applying registry manifests ---"
	@$(KUBECTL) apply -n argocd-operator-system -f prereqs/assets/registry.yaml
	@$(KUBECTL) rollout -n argocd-operator-system status deployment e2e-registry-public
	@$(KUBECTL) wait --for=condition=available --timeout=30s deployment/e2e-registry-public -n argocd-operator-system
	@bash -c '. "${CURRENT_DIR}/prereqs/assets/wait-utils.sh" && wait_for_registry_ready 60'
	@$(MAKE) git-container-push
	@echo "--- Verifying operator deployment ---"
	@$(KUBECTL) get deployment -n argocd-operator-system || true
	@$(KUBECTL) get pods -n argocd-operator-system || true