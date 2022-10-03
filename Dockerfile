# syntax=docker/dockerfile:experimental

FROM scratch AS binaries
ARG TARGETARCH
COPY ./bin/code-marketplace-linux-$TARGETARCH /tmp/code-marketplace

FROM alpine:latest
RUN --mount=from=binaries,src=/tmp,dst=/tmp/binaries cp /tmp/binaries/code-marketplace /opt
ENTRYPOINT [ "/opt/code-marketplace", "server" ]
