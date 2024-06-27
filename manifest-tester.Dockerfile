FROM alpine:3.20

RUN apk add bash wget

WORKDIR /tmp
RUN wget https://github.com/yannh/kubeconform/releases/download/v0.6.6/kubeconform-linux-amd64.tar.gz && \
    tar -xvf kubeconform-linux-amd64.tar.gz -C /usr/bin

WORKDIR /home

ENTRYPOINT [ "/bin/bash", "-c" ]

