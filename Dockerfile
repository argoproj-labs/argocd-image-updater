FROM golang:1.14.13 AS builder

RUN mkdir -p /src/argocd-image-updater
WORKDIR /src/argocd-image-updater
COPY . .

RUN mkdir -p dist && \
	make controller

FROM alpine:latest

RUN apk update && apk upgrade && apk add git

RUN mkdir -p /usr/local/bin
RUN mkdir -p /app/config

COPY --from=builder /src/argocd-image-updater/dist/argocd-image-updater /usr/local/bin/
COPY hack/git-ask-pass.sh /usr/local/bin/git-ask-pass.sh

USER 1000

ENTRYPOINT ["/usr/local/bin/argocd-image-updater"]
