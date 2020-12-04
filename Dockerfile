FROM golang:1.14.13 AS builder

RUN mkdir -p /src/argocd-image-updater
WORKDIR /src/argocd-image-updater
COPY . .

RUN mkdir -p dist && \
	make controller

FROM alpine:latest

RUN mkdir -p /usr/local/bin
RUN mkdir -p /app/config

COPY --from=builder /src/argocd-image-updater/dist/argocd-image-updater /usr/local/bin/

USER 1000

ENTRYPOINT ["/usr/local/bin/argocd-image-updater"]
