# Application configuration

Argo CD Image Updater uses `ImageUpdater` custom resources to configure which
Argo CD applications should be monitored for image updates and how those updates
should be applied.

This section explains how to configure `ImageUpdater` resources to manage your
Argo CD applications.

## <a name="imageupdater-cr"></a>Creating an ImageUpdater custom resource

To configure Argo CD Image Updater, you create an `ImageUpdater` custom resource
that defines which applications to monitor and how to update their images.

The basic structure of an `ImageUpdater` resource looks like this:

```yaml
apiVersion: argocd-image-updater.argoproj.io/v1alpha1
kind: ImageUpdater
metadata:
  name: my-image-updater
  namespace: argocd
spec:
  applicationRefs:
    - namePattern: "my-app-*"  # Select applications matching this pattern
      images:
        - alias: "nginx"
          imageName: "nginx:1.20"
```

## <a name="application-selection"></a>Selecting applications

Applications are selected using the `applicationRefs` field, which supports
multiple selection criteria:

### Name pattern matching

Use the `namePattern` field to select applications by name using glob patterns:

```yaml
spec:
  applicationRefs:
    - namePattern: "frontend-*"  # Matches frontend-app, frontend-web, etc.
      images:
        - alias: "app"
          imageName: "myregistry/frontend:latest"
    - namePattern: "backend"     # Exact match for backend application
      images:
        - alias: "api"
          imageName: "myregistry/backend:v1.0"
```

### NamePattern Specificity Rules

When multiple `namePattern` rules could match the same application, the
ImageUpdater uses a specificity-based selection algorithm to choose the
most specific rule. This ensures predictable and deterministic behavior.

#### How Specificity Works

The specificity algorithm calculates a score for each `namePattern` based on
several factors:

1. **Exact matches** get the highest priority
2. **Literal characters** in the pattern add to the score
3. **Label selectors** add significant bonus points
4. **Complex label selectors** get additional points

#### Specificity Examples

Given an application named `app-1`, here's how different patterns would be
ranked:

| Pattern         | Specificity Score | Reason                               |
|-----------------|-------------------|--------------------------------------|
| `app-1`         | 1,000,000+        | Exact match (highest priority)       |
| `app-prod-*`    | ~9 points         | More literal characters than `app-*` |
| `app-*`         | ~4 points         | Fewer literal characters             |
| `app-?`         | ~4 points         | Single wildcard character            |
| `app-[1234567]` | ~4 points         | Character set wildcard               |

#### Label Selector Impact

Label selectors significantly increase specificity:

```yaml
# This pattern will be more specific than a simple name pattern
- namePattern: "app-*"
  labelSelectors:
    matchLabels:
      environment: production
      tier: frontend
  images:
    - alias: "nginx"
      imageName: "nginx:1.20"
```

#### Complete Example

```yaml
apiVersion: argocd-image-updater.argoproj.io/v1alpha1
kind: ImageUpdater
metadata:
  name: my-image-updater
  namespace: argocd
spec:
  applicationRefs:
    # This will be used for app-1 (most specific - exact match)
    - namePattern: "app-1"
      images:
        - alias: "test1"
          imageName: "test:1.1.0"

    # This will be used for app-prod-* applications (more specific than app-*)
    - namePattern: "app-prod-*"
      images:
        - alias: "nginx"
          imageName: "nginx:1.20"

    # This will be used for other app-* applications (least specific)
    - namePattern: "app-*"
      images:
        - alias: "redis"
          imageName: "redis:6.2"
```

#### Application Selection Process

1. **List all applications** in the ImageUpdater CR's namespace (performed once per
   reconciliation)
2. **Sort applicationRefs** by specificity (most specific first)
3. **For each application**, find the first matching rule in the sorted list
4. **Stop at the first match** - this ensures the most specific rule is used

!!!warning "Multiple ImageUpdater Resources Conflict"
    **Important**: The current implementation does not handle conflicts between
    multiple `ImageUpdater` CRs that target the same application. If multiple CRs
    have `namePattern` rules that match the same application, they will
    continuously overwrite each other's changes, causing the application to
    "thrash" between different image versions.

    For example, if you have two CRs:

    - **CR-A** with `namePattern: "app-1"` and `imageName: "nginx:1.20"`
    - **CR-B** with `namePattern: "app-*"` and `imageName: "nginx:1.21"`

    Both CRs will try to manage `app-1`, causing the application to flip between
    `nginx:1.20` and `nginx:1.21` on every reconciliation cycle.

    **Workaround**: Ensure that each application is only targeted by one
    `ImageUpdater` CR, or use more specific `namePattern` rules to avoid overlaps.

    A solution for handling conflicts between multiple CRs will be implemented
    in future versions.

### Label-based selection

Use `labelSelectors` to select applications based on their labels:

```yaml
spec:
  applicationRefs:
    - namePattern: "*"  # Match all applications
      labelSelectors:
        matchLabels:
          app.kubernetes.io/part-of: "my-project"
        matchExpressions:
          - key: "environment"
            operator: In
            values: [ "production", "staging" ]
      images:
        - alias: "app"
          imageName: "myregistry/myapp:stable"
```

### Combining selection criteria

You can combine name patterns and label selectors for precise application
selection:

```yaml
spec:
  applicationRefs:
    - namePattern: "web-*"
      labelSelectors:
        matchLabels:
          tier: "frontend"
      images:
        - alias: "webapp"
          imageName: "myregistry/webapp:latest"
```

## <a name="hierarchical-configuration"></a>Hierarchical configuration

Configuration can be specified at multiple levels, with more specific levels
overriding more general ones:

1. **Global level** (`spec.commonUpdateSettings`, `spec.writeBackConfig`)
2. **Application level** (`spec.applicationRefs[].commonUpdateSettings`,
   `spec.applicationRefs[].writeBackConfig`)
3. **Image level** (`spec.applicationRefs[].images[].commonUpdateSettings`)

### Global configuration

Set defaults that apply to all applications unless overridden:

```yaml
spec:
  commonUpdateSettings:
    updateStrategy: "semver"        # Default: "semver"
    forceUpdate: false              # Default: false
    allowTags: "regexp:^v[0-9]+\\.[0-9]+\\.[0-9]+$"  # Default: all tags
    ignoreTags: [ "latest", "dev" ]   # Default: no tags ignored
    pullSecret: ""                  # Default: no pull secret
    platforms: [ ]                   # Default: no platform restrictions
  writeBackConfig:
    method: "argocd"                # Default: "argocd"
  applicationRefs:
    - namePattern: "*"
      images:
        - alias: "app"
          imageName: "myregistry/myapp:1.0"
```

### Application-level overrides

Override global settings for specific applications:

```yaml
spec:
  commonUpdateSettings:
    updateStrategy: "semver"
  applicationRefs:
    - namePattern: "production-*"
      commonUpdateSettings:
        updateStrategy: "latest"  # Override for production apps
        forceUpdate: true
      images:
        - alias: "app"
          imageName: "myregistry/myapp:stable"
    - namePattern: "development-*"
      images:
        - alias: "app"
          imageName: "myregistry/myapp:dev"
```

### Image-level overrides

Override settings for specific images:

```yaml
spec:
  commonUpdateSettings:
    updateStrategy: "semver"
  applicationRefs:
    - namePattern: "*"
      images:
        - alias: "app"
          imageName: "myregistry/myapp:1.0"
          commonUpdateSettings:
            updateStrategy: "latest"  # Override for this specific image
        - alias: "database"
          imageName: "postgres:13"
          # Uses global semver strategy
```

## <a name="application-requirements"></a>Application requirements

For Argo CD Image Updater to manage an application, the following criteria
must be met:

* The application must be of type `Helm` or `Kustomize`
* The application must be located in the `metadata.namespace` of the ImageUpdater CR
* The application must match at least one `applicationRef` criteria

## <a name="configure-write-back"></a>Configuring the write-back method

The Argo CD Image Updater supports two distinct methods on how to update images
of an application:

* *imperative*, via Argo CD API
* *declarative*, by pushing changes to a Git repository

Depending on your setup and requirements, you can choose the write-back method
per Application, but not per image. As a rule of thumb, if you are managing
`Application` in Git (i.e. in an *app-of-apps* setup), you most likely want
to choose the Git write-back method.

The write-back method is configured via an `ImageUpdater` resource:

```yaml
spec:
  writeBackConfig:
    method: "<method>"
```

Where `<method>` must be one of `argocd` (imperative) or `git` (declarative).

The default used by Argo CD Image Updater is `argocd`.

## <a name="complete-example"></a>Complete example

Here's a complete example that demonstrates various configuration options:

```yaml
apiVersion: argocd-image-updater.argoproj.io/v1alpha1
kind: ImageUpdater
metadata:
  name: production-updater
  namespace: argocd
spec:
  commonUpdateSettings:
    updateStrategy: "semver"
    forceUpdate: false
    allowTags: "regexp:^v[0-9]+\\.[0-9]+\\.[0-9]+$"
    ignoreTags: [ "latest", "dev", "test" ]
  writeBackConfig:
    method: "argocd"
  applicationRefs:
    - namePattern: "frontend-*"
      labelSelectors:
        matchLabels:
          environment: "production"
      commonUpdateSettings:
        updateStrategy: "latest"
        forceUpdate: true
      writeBackConfig:
        method: "git"
        gitConfig:
          repository: "git@github.com:myorg/frontend-config.git"
          branch: "main"
          writeBackTarget: "helmvalues:/values.yaml"
      images:
        - alias: "frontend"
          imageName: "myregistry/frontend:v1.0"
          commonUpdateSettings:
            allowTags: "regexp:^v[0-9]+\\.[0-9]+\\.[0-9]+-prod$"
        - alias: "nginx"
          imageName: "nginx:1.20"
    - namePattern: "backend"
      images:
        - alias: "api"
          imageName: "myregistry/backend:v2.0"
          commonUpdateSettings:
            updateStrategy: "digest"
        - alias: "database"
          imageName: "postgres:13"
```

This configuration:

- Sets global defaults for semver updates
- Overrides frontend applications to use latest strategy and Git write-back
- Uses specific update strategies for individual images
- Combines name patterns with label selectors for precise targeting

For more details on configuring images and update strategies, see the
[Images Configuration](images.md) and [Update Strategies](../basics/update-strategies.md)
documentation.