# --- build the server from source code
FROM golang:1.23-alpine AS builder

ENV ESM_SH_REPO https://github.com/esm-dev/esm.sh
ENV ESM_SH_VERSION v136

RUN apk update && apk add --no-cache git
RUN git clone --branch $ESM_SH_VERSION --depth 1 $ESM_SH_REPO /tmp/esm.sh

WORKDIR /tmp/esm.sh
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o esmd main.go
# ---

FROM alpine:latest AS server

COPY --from=builder /tmp/esm.sh/esmd /bin/esmd

# deno desn't provider musl build yet, the hack makes the gnu build working
# see https://github.com/denoland/deno_docker/blob/main/alpine.dockerfile
COPY --from=gcr.io/distroless/cc --chown=root:root --chmod=755 /lib/*-linux-gnu/* /usr/local/lib/
COPY --from=gcr.io/distroless/cc --chown=root:root --chmod=755 /lib/ld-linux-* /lib/
COPY --from=denoland/deno:bin-2.1.4 /deno /esmd/bin/deno
RUN mkdir /lib64 && ln -s /usr/local/lib/ld-linux-* /lib64/
ENV LD_LIBRARY_PATH="/usr/local/lib"

# for server to install package from github
RUN apk update && \
    apk add --no-cache git git-lfs && \
    git lfs install

# don't run as root
RUN addgroup -g 1000 esm && \
    adduser -u 1000 -G esm -D esm && \
    chown -R esm:esm /esmd

ENV SERVER_PORT 8080
ENV SERVER_WORKDIR /esmd

USER esm
WORKDIR /tmp
EXPOSE 8080
CMD ["esmd"]
