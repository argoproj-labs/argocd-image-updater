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

K3D_CLUSTER_NAME ?= test-e2e-local

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: deploy-argocd-operator undeploy-argocd-operator kustomize test-e2e-local test-e2e-ci k3d-cluster-create k3d-cluster-delete k3d-image-import
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
	$(KUSTOMIZE) build $$TMP_DIR | $(KUBECTL) apply --server-side=true -f -; \
	rm -rf $$TMP_DIR; \
	echo "Argo CD Operator deployment initiated.";

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
	$(K3D) cluster create $(K3D_CLUSTER_NAME)

k3d-cluster-delete: ## Delete k3d cluster for e2e tests
	@echo "--- Deleting k3d cluster $(K3D_CLUSTER_NAME) ---"
	$(K3D) cluster delete $(K3D_CLUSTER_NAME)

k3d-image-import: ## Import local image to k3d cluster
	@echo "--- Importing image $(ARGOCD_IMAGE_UPDATER_IMAGE) to k3d cluster $(K3D_CLUSTER_NAME) ---"
	$(K3D) image import $(ARGOCD_IMAGE_UPDATER_IMAGE) -c $(K3D_CLUSTER_NAME)

# Currently run only parallel tests because we don't have sequential tests yet.
test-e2e-local: ## Build local image updater, deploy operator, and run parallel e2e tests.
	@echo "--- Creating k3d cluster---"
	$(MAKE) k3d-cluster-create
	@echo "--- Building local argocd-image-updater image ---"
	$(MAKE) -C ../../ docker-build
	@echo "--- Importing image to k3d cluster ---"
	$(MAKE) k3d-image-import
	@echo "--- Deploying argocd-operator with local image-updater ---"
	$(MAKE) deploy-argocd-operator
	@echo "--- Waiting for argocd-operator to be ready ---"
	$(KUBECTL) wait --for=condition=available --timeout=300s deployment/argocd-operator-controller-manager -n argocd-operator-system
	@echo "--- Running Parallel E2E tests ---"
	$(MAKE) -C ../../ e2e-tests-parallel-ginkgo
	@echo "--- Deleting k3d cluster ---"
	$(MAKE) k3d-cluster-delete

test-e2e-ci: ## Build local image updater, deploy operator.
	@echo "--- Building local argocd-image-updater image ---"
	$(MAKE) -C ../../ docker-build
	@echo "--- Importing image to k3d cluster ---"
	$(MAKE) k3d-image-import
	@echo "--- Deploying argocd-operator with local image-updater ---"
	$(MAKE) deploy-argocd-operator
	@echo "--- Waiting for argocd-operator to be ready ---"
	$(KUBECTL) wait --for=condition=available --timeout=300s deployment/argocd-operator-controller-manager -n argocd-operator-system
