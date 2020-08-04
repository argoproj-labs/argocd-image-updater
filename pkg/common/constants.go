package common

// This file contains a list of constants required by other packages

// The annotation on the application resources to indicate the list of images
// allowed for updates.
const ImageUpdaterAnnotation = "argocd-image-updater.argoproj.io/image-list"

const HelmParamImageNameAnnotation = "argocd-image-update.argoproj.io/%s.image-name"
const HelmParamImageTagAnnotation = "argocd-image-update.argoproj.io/%s.image-tag"
const HelmParamImageSpecAnnotation = "argocd-image-update.argoproj.io/%s.image-spec"

// gcr.io=secret:argocd/mysecret,docker.io=env:FOOBAR
const SecretListAnnotation = "argocd-image-updater.argoproj.io/pullsecrets"
