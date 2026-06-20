FROM alpine:3.20
ARG BUILD_TAG=dev-unknown
ARG BUILD_BUST=0
RUN echo "demo-app ${BUILD_TAG} ${BUILD_BUST}" > /version.txt
CMD ["/bin/sh", "-c", "cat /version.txt && sleep infinity"]
