# Examples

## Basic ImageUpdater with global defaults

```yaml
apiVersion: argocd-image-updater.argoproj.io/v1alpha1
kind: ImageUpdater
metadata:
  name: my-image-updater
  namespace: argocd
spec:
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
  namespace: argocd
spec:
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
  namespace: argocd
spec:
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
  namespace: argocd
spec:
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

## Checking ImageUpdater status

After creating an `ImageUpdater` CR, you can monitor its status:

```bash
# Quick overview of all ImageUpdater resources
kubectl get imageupdater -n argocd

# Detailed status for a specific resource
kubectl get imageupdater production-updater -n argocd -o jsonpath='{.status}' | jq .

# Check conditions
kubectl get imageupdater production-updater -n argocd -o jsonpath='{.status.conditions}' | jq .

# Check recent image updates
kubectl get imageupdater production-updater -n argocd -o jsonpath='{.status.recentUpdates}' | jq .
```

Example status output:

```yaml
status:
  observedGeneration: 3
  lastCheckedAt: "2026-03-02T22:10:00Z"
  lastUpdatedAt: "2026-03-02T22:12:35Z"
  applicationsMatched: 2
  imagesManaged: 3
  recentUpdates:
    - alias: "nginx"
      image: "nginx:1.20"
      newVersion: "1.21.0"
      applicationsUpdated: 2
      updatedAt: "2026-03-02T22:12:35Z"
      message: "Updated to latest semver version."
  conditions:
    - type: "Ready"
      status: "True"
      reason: "ReconcileSucceeded"
      message: "Reconciled 2 applications, 1 images updated."
    - type: "Reconciling"
      status: "False"
      reason: "Idle"
      message: "Last check completed. Awaiting next cycle."
    - type: "Error"
      status: "False"
      reason: "NoErrors"
      message: "No errors during last reconciliation."
```

## Using `semver` update strategy with version constraints

```yaml
apiVersion: argocd-image-updater.argoproj.io/v1alpha1
kind: ImageUpdater
metadata:
  name: prod-image-updater
  namespace: argocd
spec:
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