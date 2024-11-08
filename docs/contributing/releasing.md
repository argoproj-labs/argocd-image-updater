# Releasing

Argo Image Updater is released in a 2 step automated fashion using GitHub actions. The release process takes about 20 minutes, sometimes a little less, depending on the performance of GitHub Actions runners.

Releases can only be done  by people that have write/commit access on the Argo Image Updater GitHub repository.

## Introduction

First install on your workstation the following:

1. GoLang 
1. The `git` executable
1. The [GitHub CLI](https://cli.github.com/)
1. The `semver` cli with `go install github.com/davidrjonas/semver-cli@latest`

Then create a release branch:

```
git clone git@github.com:argoproj-labs/argocd-image-updater.git
git checkout -b release-0.13
git push origin release-0.13
```

The release name is just an example. You should use the next number from the [previous release](https://github.com/argoproj-labs/argocd-image-updater/releases). Make sure that the branch is named as `release-X.XX` though.

Also note that `origin` is just an example. It should be the name of your remote that holds the upstream branch of Argo Image updater.

Finally run

```
./hack/create-release-pr.sh origin
```

Again notice that the argument must match the upstream Git remote that you used before. This [command](https://github.com/argoproj-labs/argocd-image-updater/blob/master/hack/create-release-pr.sh) will automatically create a Pull Request in GitHub with the contents of the new release.

## Review the pull request

Visit the Pull Request that you just created in the GitHub UI. Validate the contents of the diff to see the suggested changes.
Once you are happy with the result, approve/merge the Pull Request.

Merging the Pull Request will start an [automated release process](https://github.com/argoproj-labs/argocd-image-updater/blob/master/.github/workflows/create-release-draft.yaml) that will build all the artifacts
and create a draft release.

## Publish the release

You should now have a draft release or Argo Image Updater with all required artifacts attached as binaries.

First, attach the release notes. [GitHub can do it automatically for you](https://docs.github.com/en/repositories/releasing-projects-on-github/automatically-generated-release-notes) by clicking the respective button
and selecting from the drop-down menu the previous release.

Finally publish the release by clicking the green button on the bottom left of the screen.

You are finished!










