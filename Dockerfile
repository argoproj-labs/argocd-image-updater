# Build the manager binary
FROM --platform=$BUILDPLATFORM golang:1.24 AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /src/argocd-image-updater
# Copy the Go Modules manifests
COPY go.mod go.sum ./
COPY registry-scanner/go.mod registry-scanner/go.sum ./registry-scanner/
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download
COPY . .

RUN mkdir -p dist && \
	OS=${TARGETOS:-linux} ARCH=${TARGETARCH} make build

FROM --platform=$TARGETPLATFORM alpine:3.22

RUN apk update && \
    apk upgrade && \
    apk add --no-cache ca-certificates git openssh-client aws-cli tini gpg gpg-agent

RUN mkdir -p /usr/local/bin && \
    mkdir -p /app/config && \
    adduser --home "/app" --disabled-password --uid 1000 argocd

COPY --from=builder /src/argocd-image-updater/dist/argocd-image-updater /manager
COPY hack/git-ask-pass.sh /usr/local/bin/git-ask-pass.sh

USER 1000
WORKDIR /app

ENTRYPOINT ["/sbin/tini", "--", "/manager"]