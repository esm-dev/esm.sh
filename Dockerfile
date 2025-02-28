# --- build server from source code
FROM golang:1.23-alpine AS builder

ARG SERVER_VERSION="v136"

RUN apk update && apk add --no-cache git
RUN git clone --branch $SERVER_VERSION --depth 1 https://github.com/esm-dev/esm.sh /tmp/esm.sh

WORKDIR /tmp/esm.sh
RUN go build -ldflags="-s -w -X 'github.com/esm-dev/esm.sh/server.VERSION=${SERVER_VERSION}'" -o esmd server/cmd/main.go
# ---

FROM alpine:latest

# install git (use to fetch repo tags from Github)
RUN apk update && apk add --no-cache git

# add user and working directory
RUN addgroup -g 1000 esm && adduser -u 1000 -G esm -D esm && mkdir /esmd && chown -R esm:esm /esmd

# copy esmd & deno build
COPY --from=builder /tmp/esm.sh/esmd /bin/esmd
COPY --from=denoland/deno:bin-2.1.4 --chown=esm:esm /deno /esmd/bin/deno

# deno desn't provider musl build yet, the hack below makes the gnu build working in alpine
# see https://github.com/denoland/deno_docker/blob/main/alpine.dockerfile
COPY --from=gcr.io/distroless/cc --chown=root:root --chmod=755 /lib/*-linux-gnu/* /usr/local/lib/
COPY --from=gcr.io/distroless/cc --chown=root:root --chmod=755 /lib/ld-linux-* /lib/
RUN mkdir /lib64 && ln -s /usr/local/lib/ld-linux-* /lib64/
ENV LD_LIBRARY_PATH="/usr/local/lib"

# server configuration
ENV ESMPORT="8080"
ENV ESMDIR="/esmd"

# switch to non-root user
USER esm

EXPOSE 8080
WORKDIR /esmd
CMD ["esmd"]
