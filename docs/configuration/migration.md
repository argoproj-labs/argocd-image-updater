# Migration Guide: From Annotations to CRD

This guide helps you migrate from the legacy annotation-based configuration to the new CRD-based approach in Argo CD Image Updater.

## Overview

The new CRD-based approach provides several advantages over the annotation-based system:

- **Better structure**: Configuration is now organized in a clear, hierarchical structure
- **Type safety**: CRD validation ensures correct field types and required values
- **Easier management**: No need to manage complex annotation strings
- **Better tooling support**: IDEs and YAML editors provide better autocomplete and validation
- **Scalability**: Easier to manage multiple applications and images

## Migration Steps

### Step 1: Create an ImageUpdater Resource

Instead of annotating individual Argo CD Applications, you now create dedicated `ImageUpdater` resources.

**Before (Annotations):**
```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: my-app
  annotations:
    argocd-image-updater.argoproj.io/image-list: nginx=nginx:1.20
    argocd-image-updater.argoproj.io/nginx.update-strategy: semver
    argocd-image-updater.argoproj.io/nginx.allow-tags: regexp:^[0-9]+\.[0-9]+$
```

**After (CRD):**
```yaml
apiVersion: argocd-image-updater.argoproj.io/v1alpha1
kind: ImageUpdater
metadata:
  name: my-image-updater
  namespace: argocd
spec:
  applicationRefs:
    - namePattern: "my-app"
      images:
        - alias: "nginx"
          imageName: "nginx:1.20"
          commonUpdateSettings:
            updateStrategy: "semver"
            allowTags: "regexp:^[0-9]+\\.[0-9]+$"
```

### Step 2: Map Application Selection

The `namePattern` field replaces the need to annotate individual applications.

**Before:**
- Each application or application set needed individual annotations

**After:**
- Use glob patterns to select multiple applications: `"my-app-*"`, `"production-*"`
- Add label selectors for more complex selection criteria
- Group related applications under a single ImageUpdater resource

### Step 3: Convert Image Configurations

#### Basic Image List

**Before:**
```yaml
argocd-image-updater.argoproj.io/image-list: nginx=nginx:1.20,redis=redis:6.2
```

**After:**
```yaml
images:
  - alias: "nginx"
    imageName: "nginx:1.20"
  - alias: "redis"
    imageName: "redis:6.2"
```

#### Update Strategies

**Before:**
```yaml
argocd-image-updater.argoproj.io/nginx.update-strategy: semver
argocd-image-updater.argoproj.io/redis.update-strategy: latest
```

**After:**
```yaml
images:
  - alias: "nginx"
    imageName: "nginx:1.20"
    commonUpdateSettings:
      updateStrategy: "semver"
  - alias: "redis"
    imageName: "redis:6.2"
    commonUpdateSettings:
      updateStrategy: "newest-build"
```

#### Tag Filtering

**Before:**
```yaml
argocd-image-updater.argoproj.io/nginx.allow-tags: regexp:^[0-9]+\.[0-9]+$
argocd-image-updater.argoproj.io/nginx.ignore-tags: latest,dev
```

**After:**
```yaml
images:
  - alias: "nginx"
    imageName: "nginx:1.20"
    commonUpdateSettings:
      allowTags: "regexp:^[0-9]+\\.[0-9]+$"
      ignoreTags: ["latest", "dev"]
```

#### Pull Secrets

**Before:**
```yaml
argocd-image-updater.argoproj.io/nginx.pull-secret: secret:my-namespace/my-secret#username
```

**After:**
```yaml
images:
  - alias: "nginx"
    imageName: "nginx:1.20"
    commonUpdateSettings:
      pullSecret: "secret:my-namespace/my-secret#username"
```

#### Platform Specifications

**Before:**
```yaml
argocd-image-updater.argoproj.io/nginx.platforms: linux/amd64,linux/arm64
```

**After:**
```yaml
images:
  - alias: "nginx"
    imageName: "nginx:1.20"
    commonUpdateSettings:
      platforms: ["linux/amd64", "linux/arm64"]
```

### Step 4: Convert Helm Configurations

**Before:**
```yaml
argocd-image-updater.argoproj.io/dex.helm.image-name: dex.image.name
argocd-image-updater.argoproj.io/dex.helm.image-tag: dex.image.tag
```

**After:**
```yaml
images:
  - alias: "dex"
    imageName: "quay.io/dexidp/dex:latest"
    manifestTargets:
      helm:
        name: "dex.image.name"
        tag: "dex.image.tag"
```

### Step 5: Convert Kustomize Configurations

**Before:**
```yaml
argocd-image-updater.argoproj.io/argocd.kustomize.image-name: quay.io/argoproj/argocd
```

**After:**
```yaml
images:
  - alias: "argocd"
    imageName: "ghcr.io/argoproj/argocd:latest"
    manifestTargets:
      kustomize:
        name: "quay.io/argoproj/argocd"
```

### Step 6: Convert Write-Back Configurations

**Before:**
```yaml
argocd-image-updater.argoproj.io/write-back-method: git
argocd-image-updater.argoproj.io/git-repository: git@github.com:myorg/myrepo.git
argocd-image-updater.argoproj.io/git-branch: main
argocd-image-updater.argoproj.io/write-back-target: helmvalues:./values.yaml
```

**After:**
```yaml
spec:
  writeBackConfig:
    method: "git"
    gitConfig:
      repository: "git@github.com:myorg/myrepo.git"
      branch: "main"
      writeBackTarget: "helmvalues:./values.yaml"
```

## Complete Migration Example

Here's a complete example showing the migration from annotations to CRD:

### Before (Annotation-based)

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: production-app
  annotations:
    argocd-image-updater.argoproj.io/image-list: nginx=nginx:1.20,redis=redis:6.2
    argocd-image-updater.argoproj.io/write-back-method: git
    argocd-image-updater.argoproj.io/git-repository: git@github.com:myorg/myrepo.git
    argocd-image-updater.argoproj.io/git-branch: main
    argocd-image-updater.argoproj.io/write-back-target: helmvalues:./values.yaml
    argocd-image-updater.argoproj.io/update-strategy: semver
    argocd-image-updater.argoproj.io/nginx.allow-tags: regexp:^[0-9]+\.[0-9]+$
    argocd-image-updater.argoproj.io/nginx.ignore-tags: latest,dev
    argocd-image-updater.argoproj.io/redis.update-strategy: latest
    argocd-image-updater.argoproj.io/redis.pull-secret: secret:my-namespace/redis-secret#username
---
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: staging-app
  annotations:
    argocd-image-updater.argoproj.io/write-back-method: git
    argocd-image-updater.argoproj.io/git-repository: git@github.com:myorg/myrepo.git
    argocd-image-updater.argoproj.io/git-branch: develop
    argocd-image-updater.argoproj.io/write-back-target: helmvalues:./values.yaml
    argocd-image-updater.argoproj.io/image-list: nginx=nginx:latest
    argocd-image-updater.argoproj.io/nginx.update-strategy: latest
```

### After (CRD-based)

```yaml
apiVersion: argocd-image-updater.argoproj.io/v1alpha1
kind: ImageUpdater
metadata:
  name: multi-app-image-updater
  namespace: argocd
spec:
  writeBackConfig:
    method: "git"
    gitConfig:
      repository: "git@github.com:myorg/myrepo.git"
      branch: "main"
      writeBackTarget: "helmvalues:./values.yaml"
  applicationRefs:
    - namePattern: "production-app"
      images:
        - alias: "nginx"
          imageName: "nginx:1.20"
          commonUpdateSettings:
            allowTags: "regexp:^[0-9]+\\.[0-9]+$"
            ignoreTags: ["latest", "dev"]
        - alias: "redis"
          imageName: "redis:6.2"
          commonUpdateSettings:
            updateStrategy: "newest-build"
            pullSecret: "secret:my-namespace/redis-secret#username"
    - namePattern: "staging-app"
      writeBackConfig:
        method: "git"
        gitConfig:
          branch: "develop"
      images:
        - alias: "nginx"
          imageName: "nginx"
          commonUpdateSettings:
            updateStrategy: "newest-build"
```

## Common Pitfalls

### 1. Namespace Specification
The target namespace is determined by the ImageUpdater CR's `metadata.namespace` field, not per application.

### 2. Array vs String
Some fields (`platforms`, `ignoreTags`) that were comma-separated strings are now arrays:

**Before:** `ignore-tags: latest,dev`
**After:** `ignoreTags: ["latest", "dev"]`

For additional help, please refer to the [Configuration documentation](images.md) or open an issue on the [GitHub repository](https://github.com/argoproj-labs/argocd-image-updater).