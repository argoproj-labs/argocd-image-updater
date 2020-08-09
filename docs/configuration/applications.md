# Application configuration

In order for Argo CD Image Updater to know which applications it should inspect
for updating the workloads' container images, the corresponding Kubernetes
resource needs to be correctly annotated. Argo CD Image Updater will inspect
only resources of kind `application.argoproj.io`, that is, your Argo CD
`Application` resources. Annotations on other kinds of resources will have no
effect and will not be considered.

For its annotations, Argo CD Image Updater uses the following prefix:

```yaml
argocd-image-updater.argoproj.io
```

As explained earlier, your Argo CD applications must be of either `Kustomize`
or `Helm` type. Other types of applications will be ignored.

So, in order for Argo CD Image Updater to consider your application for the
update of its images, at least the following criteria must be met:

* Your `Application` resource is annotated with the mandatory annotation of
  `argocd-image-updater.argoproj.io/image-list`, which contains at least one
  valid image specification (see [Images Configuration](images.md)).

* Your `Application` resource is of type `Helm` or `Kustomize`

An example of a correctly annotated `Application` resources might look like:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  annotations:
    argocd-image-updater.argoproj.io/image-list: gcr.io/heptio-images/ks-guestbook-demo:^0.1
  name: guestbook
  namespace: argocd
spec:
  destination:
    namespace: guestbook
    server: https://kubernetes.default.svc
  project: default
  source:
    path: helm-guestbook
    repoURL: https://github.com/argocd-example-apps/argocd-example-apps
    targetRevision: HEAD
```
