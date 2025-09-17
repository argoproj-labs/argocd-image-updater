# Examples

## Basic ImageUpdater with global defaults

```yaml
apiVersion: argocd-image-updater.argoproj.io/v1alpha1
kind: ImageUpdater
metadata:
  name: my-image-updater
spec:
  namespace: argocd
  commonUpdateSettings:
    updateStrategy: "semver"
    forceUpdate: false
  applicationRefs:
    - namePattern: "my-app-*"
      images:
        - alias: "nginx"
          imageName: "nginx:1.20"
```

## ImageUpdater with application-specific overrides

```yaml
apiVersion: argocd-image-updater.argoproj.io/v1alpha1
kind: ImageUpdater
metadata:
  name: my-image-updater
spec:
  namespace: argocd
  commonUpdateSettings:
    updateStrategy: "semver"
  applicationRefs:
    - namePattern: "production-*"
      commonUpdateSettings:
        updateStrategy: "digest"  # Override for production apps
      images:
        - alias: "app"
          imageName: "myapp:latest"
    - namePattern: "staging-*"
      images:
        - alias: "app"
          imageName: "myapp:latest"
          commonUpdateSettings:
            updateStrategy: "latest"  # Override for this specific image
```

## ImageUpdater with Git write-back

```yaml
apiVersion: argocd-image-updater.argoproj.io/v1alpha1
kind: ImageUpdater
metadata:
  name: my-image-updater
spec:
  namespace: argocd
  writeBackConfig:
    method: "git"
    gitConfig:
      repository: "git@github.com:myorg/myrepo.git"
      branch: "main"
      writeBackTarget: "helmvalues:./values.yaml"
  applicationRefs:
    - namePattern: "my-app-*"
      images:
        - alias: "nginx"
          imageName: "nginx:1.20"
          manifestTargets:
            helm:
              name: "image.repository"
              tag: "image.tag"
```

## Using `digest` update strategy for tracking mutable tags

```yaml
apiVersion: argocd-image-updater.argoproj.io/v1alpha1
kind: ImageUpdater
metadata:
  name: dev-image-updater
spec:
  namespace: argocd
  commonUpdateSettings:
    updateStrategy: "digest"
  writeBackConfig:
    method: "argocd"
  applicationRefs:
    - namePattern: "dev"
      images:
        - alias: "api"
          imageName: "registry.com/vendor/api:latest"
        - alias: "front"
          imageName: "registry.com/vendor/front:latest"
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

## Using `semver` update strategy with version constraints

```yaml
apiVersion: argocd-image-updater.argoproj.io/v1alpha1
kind: ImageUpdater
metadata:
  name: prod-image-updater
spec:
  namespace: argocd
  writeBackConfig:
    method: "argocd"
  applicationRefs:
    - namePattern: "prod"
      images:
        - alias: "api"
          imageName: "registry.com/vendor/api:1.x"
          commonUpdateSettings:
            updateStrategy: "semver"
        - alias: "front"
          imageName: "registry.com/vendor/front:1.x"
          commonUpdateSettings:
            updateStrategy: "semver"
---
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
        - name: app
          image: registry.com/vendor/api:1.0
```