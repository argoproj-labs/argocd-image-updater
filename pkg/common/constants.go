package common

// This file contains a list of constants required by other packages

// The annotation on the application resources to indicate the list of images
// allowed for updates.
const ImageUpdaterAnnotation = "argocd-image-updater.argoproj.io/image-list"

const HelmParamImageNameAnnotation = "argocd-image-updater.argoproj.io/%s.image-name"
const HelmParamImageTagAnnotation = "argocd-image-updater.argoproj.io/%s.image-tag"
const HelmParamImageSpecAnnotation = "argocd-image-updater.argoproj.io/%s.image-spec"

const MatchOptionAnnotation = "argocd-image-updater.argoproj.io/%s.match"
const SortOptionAnnotation = "argocd-image-updater.argoproj.io/%s.sort"

// gcr.io=secret:argocd/mysecret,docker.io=env:FOOBAR
const SecretListAnnotation = "argocd-image-updater.argoproj.io/pullsecrets"
