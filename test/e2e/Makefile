IMAGE_REPO		?= 10.42.0.1:30000
IMAGE_NAME		?= git-http
IMAGE_TAG		?= latest

IMAGE_SLUG		:= $(IMAGE_REPO)/$(IMAGE_NAME):$(IMAGE_TAG)

git-container-build: copy-testdata
	docker build -t $(IMAGE_SLUG) -f ./containers/git/Dockerfile ./containers/git

git-container-push: git-container-build
	docker push $(IMAGE_SLUG)

remove-docker-images:
	docker rmi $$(docker images | grep $(IMAGE_REPO) | awk '{print $$3}')

copy-testdata:
	cp -r testdata containers/git

install-prereqs: 
	kustomize build prereqs/argocd | kubectl -n argocd-image-updater-e2e apply -f -
	kubectl apply -n argocd-image-updater-e2e -f prereqs/registry/registry.yaml
	kubectl rollout -n argocd-image-updater-e2e status deployment e2e-registry-public
	sleep 5
	make git-container-push
	kubectl apply -n argocd-image-updater-e2e -f prereqs/repo/install.yaml

delete-prereqs: 
	kubectl delete -n argocd-image-updater-e2e -f prereqs/repo/install.yaml
	kubectl delete -n argocd-image-updater-e2e -f prereqs/registry/registry.yaml
	kustomize build prereqs/argocd | kubectl -n argocd-image-updater-e2e delete -f -
	make remove-docker-images

.PHONY: git-container-build
.PHONY: git-container-push
.PHONY: copy-testdata
