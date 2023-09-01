# Examples

## Digest
Using `digest` sha configuration for `image-list`

```
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: dev
  annotations:
    argocd-image-updater.argoproj.io/write-back-method: argocd
    argocd-image-updater.argoproj.io/image-list: api=registry.com/vendor/api:latest,front=registry.com/vendor/front:latest
    argocd-image-updater.argoproj.io/update-strategy: digest
---
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
        - name: app
          image: registry.com/vendor/api@sha256:38089... # Initial sha
```

## semver
Using `semver` defining the `update-strategy` per `image-list`

```
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: prod
  annotations:
    argocd-image-updater.argoproj.io/write-back-method: argocd
    argocd-image-updater.argoproj.io/image-list: api=registry.com/vendor/api:1.x,front=registry.com/vendor/front:1.x
    #argocd-image-updater.argoproj.io/update-strategy: semver # to apply for all the images
    argocd-image-updater.argoproj.io/api.update-strategy: semver
    argocd-image-updater.argoproj.io/front.update-strategy: semver
---
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
          image: registry.com/vendor/api:1.0
```
