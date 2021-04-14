# Application configuration

## Marking an application for being updateable

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

## Configuring the write-back method

The Argo CD Image Updater supports two distinct methods on how to update images
of an application:

* *imperative*, via Argo CD API
* *declarative*, by pushing changes to a Git repository

Depending on your setup and requirements, you can chose the write-back method
per Application, but not per image. As a rule of thumb, if you are managing
`Application` in Git (i.e. in an *app-of-apps* setup), you most likely want
to chose the Git write-back method.

The write-back method is configured via an annotation on the `Application`
resource:

```yaml
argocd-image-updater.argoproj.io/write-back-method: <method>
```

Where `<method>` must be one of `argocd` (imperative) or `git` (declarative).

The default used by Argo CD Image Updater is `argocd`.

### Using the Argo CD API write-back method

When using the Argo CD API to write back changes, Argo CD Image Updater will
perform a similar action as `argocd app set --parameter ...` to instruct
Argo CD to re-render the manifests using those parameters.

This method is pseudo-persistent. If you delete the `Application` resource
from the cluster and re-create it, changes made by Image Updater will be gone.
The same is true if you manage your `Application` resources using Git, and
the version stored in Git is synced over the resource in the cluster. This
method is most suitable for Applications also created imperatively, i.e.
using the Web UI or CLI.

This method is the default and requires no further configuration.

### Using the Git write-back method

!!!warning "Compatibility with Argo CD"
    The Git write-back method requires a feature in Argo CD that has been
    introduced with Argo CD v2.0. Git write-back will not work with earlier
    versions of Argo CD.

The `git` write-back method uses Git to permanently store its parameter
overrides along with the Application's resource manifests. This will enable
persistent storage of the parameters in Git.

By default, Argo CD Image Updater will store the parameter in a file named
`.argocd-source-<appName>.yaml` in the path used by the Application to source
its manifests from. This will allow Argo CD to pick up parameters in this
file, when rendering manifests for the Application named `<appName>`. Using
this approach will also minimize the possibility of merge conflicts, as long
as no other party in your CI will modify this file.

!!!note "A note on the application's target revision"
    Due to the nature of how Git write-back works, your application really
    should track a *branch* instead of a revision. If you track `HEAD`, a tag
    or a certain revision with your application, you **must** override the
    branch in an annotation (see below). But in order for Argo CD to pick up
    the change after Image Updater has committed & pushed the change, you
    really want to set it up so it tracks a branch.

To use the Git write-back method, annotate your `Application` with the right
write-back method:

```yaml
argocd-image-updater.argoproj.io/write-back-method: git
```

In order to better decide whether this method is suitable for your use-case,
this is the workflow how Argo CD Image Updater performs change to Git:

* Fetch the remote repository from location specified by `.spec.source.repoURL`
  in the Argo CD Application manifest, using credentials specified as annotation
  (see below)
* Check-out the target branch on the local copy. The target branch is either
  taken from an annotation (see below), or if no annotation is set, taken from
  `.spec.source.targetRevision` in the Application manifest
* Create or update `.argocd-source-<appName>.yaml` in the local repository
* Commit the changed file to the local repository
* Push the commit to the remote repository, using credentials specified as
  annotation (see below)

The important pieces to this workflow are:

* Credentials configured in Argo CD will not be re-used, you have to supply a
  dedicated set of credentials

* Write-back is a commit to the tracking branch of the Application. Currently,
  Image Updater does not support creating a new branch or creating pull or
  merge requests

* If `.spec.source.targetRevision` does not reference a *branch*, you will have
  to specify the branch to use manually (see below)

#### General configuration

Configuration for the Git write-back method comes from two sources:

* The Argo CD `Application` manifest is used to define the repository and the
  path where the `.argocd-source-<appName>.yaml` should be written to. These
  are defined in `.spec.source.repoURL` and `.spec.source.path` fields,
  respectively. Additionally, `.spec.source.targetRevision` is used to define
  the branch to commit and push the changes to. The branch to use can be
  overridden by an annotation, see below.

* A set of annotations on the `Application` manifest, see below

#### Specifying Git credentials

By default Argo CD Image Updater re-uses the credentials you have configured
in Argo CD for accessing the repository.

If you don't want to use credentials configured for Argo CD you can use other credentials stored in a Kubernetes secret,
which needs to be accessible by the Argo CD Image Updater's Service Account. The secret should be specified in 
`argocd-image-updater.argoproj.io/write-back-method` annotation using `git:<credref>` format. Where `<credref>` might
take one of following values:

* `repocreds` (default) - Git repository credentials configured in Argo CD settings
* `secret:<namespace>/<secret>` - namespace and secret name.

Example:

```yaml
argocd-image-updater.argoproj.io/write-back-method: git:secret:argocd-image-updater/git-creds
```

If the repository is accessed using HTTPS, the secret must contain two fields:
`username` which holds the Git username, and `password` which holds the user's
password or a private access token (PAT) with write access to the repository.
You can generate such a secret using `kubectl`, e.g.:

```bash
kubectl -n argocd-image-updater secret create generic git-creds \
  --from-literal=username=someuser \
  --from-literal=password=somepassword
```

If the repository is accessed using SSH, the secret must contain the field
`sshPrivateKey`, which holds a SSH private key in OpenSSH-compatible PEM
format. To create such a secret from an existing private key, you can use
`kubectl`, for example:

```bash
kubectl -n argocd-image-updater secret create generic git-creds \
  --from-file=sshPrivateKey=~/.ssh/id_rsa
```

#### Specifying a branch to commit to

By default, Argo CD Image Updater will use the value found in the Application
spec at `.spec.source.targetRevision` as Git branch to checkout, commit to
and push back the changes it made. In some scenarios, this might not be what
is desired, and you can (and maybe have to) override the branch to use by
specifying the annotation `argocd-image-updater.argoproj.io/git-branch` on the
Application manifest.

The value of this annotation will define the Git branch to use, for example the
following would use GitHub's default `main` branch:

```yaml
argocd-image-updater.argoproj.io/git-branch: main
```

#### Specifying the user and email address for commits

Each Git commit is associated with an author's name and email address. If not
configured, commits performed by Argo CD Image Updater will use
`argocd-image-updater <noreply@argoproj.io>`
as the author. You can override the author using the
`--git-commit-user` and `--git-commit-email` command line switches or set
`git.user` and `git.email`
in the `argocd-image-updater-config` ConfigMap.

#### Changing the Git commit message

You can change the default commit message used by Argo CD Image Updater to some
message that best suites your processes and regulations. For this, a simple
template can be created (evaluating using the `text/template` Golang package)
and made available through setting the key `git.commit-message-template` in the
`argocd-image-updater-config` ConfigMap to the template's contents, e.g.

```yaml
data:
  git.commit-message-template: |
    build: automatic update of {{ .AppName }}

    {{ range .AppChanges -}}
    updates image {{ .Image }} tag '{{ .OldTag }}' to '{{ .NewTag }}'
    {{ end -}}
```

Two top-level variables are provided to the template:

* `.AppName` is the name of the application that is being updated
* `.AppChanges` is a list of changes that were performed by the update. Each
  entry in this list is a struct providing the following information for
  each change:
  * `.Image` holds the full name of the image that was updated
  * `.OldTag` holds the tag name or SHA digest previous to the update
  * `.NewTag` holds the tag name or SHA digest that was updated to

In order to test a template before configuring it for use in Image Updater,
you can store the template you want to use in a temporary file, and then use
the `argocd-image-updater template /path/to/file` command to render the
template using pre-defined data and see its outcome on the terminal.
