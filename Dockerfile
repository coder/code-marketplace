# syntax=docker/dockerfile:experimental

FROM scratch AS binaries
ARG TARGETARCH
COPY ./bin/code-marketplace-linux-$TARGETARCH /opt/code-marketplace

FROM alpine:latest
COPY --chmod=755 --from=binaries /opt/code-marketplace /opt
RUN ln -s /opt/code-marketplace /usr/local/bin/code-marketplace

ENTRYPOINT [ "code-marketplace", "server" ]
