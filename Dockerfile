FROM golang:1.24 AS builder

RUN mkdir -p /src/argocd-image-updater
WORKDIR /src/argocd-image-updater
# copy the entire repo first so local replaces (./registry-scanner) exist in context
COPY . .
# cache dependencies as a layer for faster rebuilds
RUN go mod download

RUN mkdir -p dist && \
	make controller

FROM alpine:3.22

RUN apk update && \
    apk upgrade && \
    apk add ca-certificates git openssh-client aws-cli tini gpg gpg-agent && \
    rm -rf /var/cache/apk/*

RUN mkdir -p /usr/local/bin
RUN mkdir -p /app/config
RUN adduser --home "/app" --disabled-password --uid 1000 argocd

COPY --from=builder /src/argocd-image-updater/dist/argocd-image-updater /usr/local/bin/
COPY hack/git-ask-pass.sh /usr/local/bin/git-ask-pass.sh

USER 1000

ENTRYPOINT ["/sbin/tini", "--", "/usr/local/bin/argocd-image-updater"]
