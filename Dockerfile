FROM golang:1.14.4 AS builder

RUN mkdir -p /src/argocd-image-controller
WORKDIR /src/argocd-image-controller
COPY . .

RUN mkdir -p dist && \
	make controller

FROM alpine:latest

RUN mkdir -p /usr/local/bi n
COPY --from=builder /src/argocd-image-controller/dist/argocd-image-controller /usr/local/bin/

USER 1000

ENTRYPOINT ["/usr/local/bin/argocd-image-controller"]
