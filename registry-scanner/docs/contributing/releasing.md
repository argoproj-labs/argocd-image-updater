# Releasing

Registry Scanner is released in a 1 step automated fashion. The release process takes about 5 minutes.

Releases can only be done by people that have write/commit access on the Argo Image Updater GitHub repository.

## Introduction

First install on your workstation the following:

1. GoLang 
1. The `git` executable
1. The [GitHub CLI](https://cli.github.com/)
1. The `semver` cli with `go install github.com/davidrjonas/semver-cli@latest`

Then create a release branch and cd to the submodule:

```bash
git clone git@github.com:argoproj-labs/argocd-image-updater.git
git checkout -b registry-scanner/release-0.13
git push origin registry-scanner/release-0.13
```

The release name is just an example. You should use the next number from the [previous release](https://github.com/argoproj-labs/argocd-image-updater/releases). Make sure that the branch is named as `registry-scanner/release-X.XX` though.

!!!Note:
`TARGET_VERSION` is the version we want to release for registry-scanner module.

Also note that `origin` is just an example. It should be the name of your remote that holds the upstream branch of Argo Image updater repository.

Finally run

```bash
cd registry-scanner
./hack/create-release-pr.sh ${TARGET_VERSION} ${REMOTE}
```

e.g.:
```bash
cd registry-scanner
./hack/create-release-pr.sh 0.1.0 origin
```

You are finished!










