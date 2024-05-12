# Update methods

## Overview

Argo CD Image Updater supports several methods to propagate new versions of the
images to Argo CD. These methods are also referred to as *write back methods*.

Currently, the following methods are supported:

* [argocd](../configuration/applications.md#method-argocd)
  directly modifies the Argo CD *Application* resource, either using Kubernetes
  or via Argo CD API, depending on Argo CD Image Updater's configuration.

* [git](../configuration/applications.md#method-git)
  will create a Git commit in your Application's Git repository that holds the
  information about the image to update to.

Depending on the write back method, further configuration may be possible.

The write back method and its configuration is specified per Application.

## <a name="method-argocd"></a>`argocd` write-back method

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

## <a name="method-git"></a>`git` write-back method

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

* Credentials configured in Argo CD will be re-used, unless you override with a
  dedicated set of credentials

* Write-back is a commit to the tracking branch of the Application.

* If `.spec.source.targetRevision` does not reference a *branch*, you will have
  to specify the branch to use manually (see below)

### <a name="method-git-general-config"></a>General configuration

Configuration for the Git write-back method comes from two sources:

* The Argo CD `Application` manifest is used to define the repository and the
  path where the `.argocd-source-<appName>.yaml` should be written to. These
  are defined in `.spec.source.repoURL` and `.spec.source.path` fields,
  respectively. Additionally, `.spec.source.targetRevision` is used to define
  the branch to commit and push the changes to. The branch to use can be
  overridden by an annotation, see below.

* A set of annotations on the `Application` manifest, see below

### <a name="method-git-credentials"></a>Specifying Git credentials

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

If the repository is accessed using HTTPS, the secret must contain either user credentials or GitHub app credentials.

If the repository is accessed using user credentials, the secret requires two fields
`username` which holds the Git username, and `password` which holds the user's
password or a private access token (PAT) with write access to the repository.
You can generate such a secret using `kubectl`, e.g.:

```bash
kubectl -n argocd-image-updater create secret generic git-creds \
  --from-literal=username=someuser \
  --from-literal=password=somepassword
```

If the repository is accessed using GitHub app credentials, the secret requires three fields `githubAppID` which holds the GitHub Application ID, `githubAppInstallationID` which holds the GitHub Organization Installation ID, and `githubAppPrivateKey` which holds the GitHub Application private key. The GitHub Application must be installed into the target repository with write access.
You can generate such a secret using `kubectl`, e.g.:

```bash
kubectl -n argocd-image-updater create secret generic git-creds \
  --from-literal=githubAppID=applicationid \
  --from-literal=githubAppInstallationID=installationid \
  --from-literal=githubAppPrivateKey='-----BEGIN RSA PRIVATE KEY-----PRIVATEKEYDATA-----END RSA PRIVATE KEY-----'
```

If the repository is accessed using SSH, the secret must contain the field
`sshPrivateKey`, which holds a SSH private key in OpenSSH-compatible PEM
format. To create such a secret from an existing private key, you can use
`kubectl`, for example:

```bash
kubectl -n argocd-image-updater create secret generic git-creds \
  --from-file=sshPrivateKey=~/.ssh/id_rsa \
  --from-file=sshPublicKey=~/.ssh/id_rsa.pub \
```

### <a name="method-git-repository"></a>Specifying a repository when using a Helm repository in repoURL

By default, Argo CD Image Updater will use the value found in the Application
spec at `.spec.source.repoURL` as Git repository to checkout. But when using
a Helm repository as `.spec.source.repoURL` GIT will simply fail. To manually
specify the repository to push the changes, specify the 
annotation `argocd-image-updater.argoproj.io/git-repository` on the Application
manifest.

The value of this annotation will define the Git repository to use, for example the
following would use a GitHub's repository:

```yaml
argocd-image-updater.argoproj.io/git-repository: git@github.com:example/example.git
```

### <a name="method-git-branch"></a>Specifying a branch to commit to

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

### <a name="method-git-base-commit-branch"></a>Specifying a separate base and commit branch

By default, Argo CD Image Updater will checkout, commit, and push back to the
same branch specified above. There are many scenarios where this is not
desired or possible, such as when the default branch is protected. You can
add a separate write-branch by modifying `argocd-image-updater.argoproj.io/git-branch`
with additional data, which will create a new branch from the base branch, and
push to this new branch instead:

```yaml
argocd-image-updater.argoproj.io/git-branch: base:target
```

If you want to specify a write-branch but continue to use the target revision from the application
specification, just omit the base branch name:

```yaml
argocd-image-updater.argoproj.io/git-branch: :target
```

A static branch name may not be desired for this value, so a simple template
can be created (evaluating using the `text/template` Golang package) within
the annotation. For example, the following would create a branch named
`image-updater-foo/bar-1.1` based on `main` in the event an image with
the name `foo/bar` was updated to the new tag `1.1`.

```yaml
argocd-image-updater.argoproj.io/git-branch: main:image-updater{{range .Images}}-{{.Name}}-{{.NewTag}}{{end}}
```

Alternatively, to assure unique branch names you could use the SHA1 representation of the changes:

```yaml
argocd-image-updater.argoproj.io/git-branch: main:image-updater-{{.SHA256}}
```

The following variables are provided for this template:

* `.Images` is a list of changes that were performed by the update. Each
  entry in this list is a struct providing the following information for
  each change:
  * `.Name` holds the full name of the image that was updated
  * `.Alias` holds the alias of the image that was updated
  * `.OldTag` holds the tag name or SHA digest previous to the update
  * `.NewTag` holds the tag name or SHA digest that was updated to
* `.SHA256` is a unique SHA256 has representing these changes

Please note that if the output of the template exceeds 255 characters (git branch name limit) it will be truncated.

### <a name="method-git-commit-user"></a>Specifying the user and email address for commits

Each Git commit is associated with an author's name and email address. If not
configured, commits performed by Argo CD Image Updater will use
`argocd-image-updater <noreply@argoproj.io>`
as the author. You can override the author using the
`--git-commit-user` and `--git-commit-email` command line switches or set
`git.user` and `git.email`
in the `argocd-image-updater-config` ConfigMap.

### <a name="method-git-commit-signing"></a>Enabling commit signature verification using an SSH or GPG key
Commit signing requires the repository be accessed using HTTPS or SSH with a user account.
Repositories accessed using a GitHub App can not be verified when using the git command line at this time.

Each Git commit associated with an author's name and email address can be signed via a public SSH key or GPG key.
Commit signing requires a bot account with a GPG or SSH key and the username and email address configured to match the bot account.

Your preferred signing key must be associated with your bot account. See provider documentation for further details:
* [GitHub](https://docs.github.com/en/authentication/managing-commit-signature-verification/about-commit-signature-verification)
* [GitLab](https://docs.gitlab.com/ee/user/project/repository/signed_commits/)
* [Bitbucket](https://confluence.atlassian.com/bitbucketserver/controlling-access-to-code-776639770.html)

Commit Sign Off can be enabled by setting `git.commit-sign-off: "true"`

**SSH:**

Both private and public keys must be mounted and accessible on the `argocd-image-updater` pod.

Set `git.commit-signing-key` `argocd-image-updater-config` ConfigMap to the path of your public key:

```yaml
data:
  git.commit-sign-off: "true"
  git.commit-signing-key: /app/.ssh/id_rsa.pub
```

The matching private key must be available in the same location.

Create a new SSH secret or add the public key to your existing SSH secret:
```bash
kubectl -n argocd-image-updater create secret generic ssh-git-creds \
  --from-file=sshPrivateKey=~/.ssh/id_rsa \
  --from-file=sshPublicKey=~/.ssh/id_rsa.pub
```

**GPG:**

The GPG private key must be installed and available in the `argocd-image-updater` pod.
Set `git.commit-signing-key` in the `argocd-image-updater-config` ConfigMap to the GPG key ID you want to use:

```yaml
data:
  git.commit-sign-off: "true"
  git.commit-signing-key: 3AA5C34371567BD2
```

### <a name="method-git-commit-message"></a>Changing the Git commit message

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

### <a name="method-git-target"></a>Git Write-Back Target

By default, git write-back will create or update `.argocd-source-<appName>.yaml`.

If you are using Kustomize and want the image updates available for normal use with `kustomize`,
you may set the `write-back-target` to `kustomization`. This method commits changes to the Kustomization
file back to git as though you ran `kustomize edit set image`.

```yaml
argocd-image-updater.argoproj.io/write-back-method: git  # all git options are supported
argocd-image-updater.argoproj.io/write-back-target: kustomization
```

You may also specify which kustomization to update with either a path relative to the project source path...

```yaml
argocd-image-updater.argoproj.io/write-back-target: "kustomization:../../base"
# if the Application spec.source.path = config/overlays/foo, this would update the kustomization in config/base 
```

...or absolute with respect to the repository:

```yaml
# absolute paths start with /
argocd-image-updater.argoproj.io/write-back-target: "kustomization:/config/overlays/bar"
```

Note that the Kustomization directory needs to be specified, not a file, like when using Kustomize.

If you are using Helm and want the image updates parameters available in your values files,
you may set the `write-back-target` to `helmvalues:<full path to your values file>`. This method commits changes to the values
file back that is used to render the Helm template.

```yaml
argocd-image-updater.argoproj.io/write-back-method: git  # all git options are supported
argocd-image-updater.argoproj.io/write-back-target: helmvalues
```

You may also specify which helmvalues to update with either a path relative to the project source path...

```yaml
argocd-image-updater.argoproj.io/write-back-target: "helmvalues:../../values.yaml"
# if the Application spec.source.path = config/overlays/foo, this would update the helmvalues in config/base 
```

...or absolute with respect to the repository:

```yaml
# absolute paths start with /
argocd-image-updater.argoproj.io/write-back-target: "helmvalues:/helm/config/test-values.yaml"
```

Note that using the helmvalues option needs the Helm values filename to be specified in the
write-back-target annotation.
