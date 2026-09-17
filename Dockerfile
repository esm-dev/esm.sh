# build >>>
FROM golang:1.26-alpine AS builder

WORKDIR /tmp/esm.sh
COPY internal/ ./internal/
COPY server/ ./server/
COPY go.mod go.sum ./
RUN go mod download

ARG SERVER_VERSION="main"
RUN CGO_ENABLED=0 go build -ldflags="-s -w -X 'github.com/esm-dev/esm.sh/server.VERSION=${SERVER_VERSION}'" -o esmd server/esmd/main.go
# <<<

FROM alpine:latest

RUN apk add --no-cache git
RUN addgroup -g 1000 esm && adduser -u 1000 --home=/esm -G esm -D esm

COPY --from=builder /tmp/esm.sh/esmd /bin/esmd
COPY --from=denoland/deno:bin-2.7.13 --chown=esm:esm /deno /esm/bin/deno

# deno doesn't provide musl build yet, the hack below makes the gnu build working in alpine
COPY --from=gcr.io/distroless/cc --chown=root:root --chmod=755 /lib/*-linux-gnu/ /usr/local/lib/glibc/
COPY --from=gcr.io/distroless/cc --chown=root:root --chmod=755 /lib/ld-linux-* /lib/
RUN --mount=type=bind,from=debian:trixie-slim,source=/sbin/ldconfig,target=/tmp/ldconfig \
    mkdir /lib64 && ln -s /usr/local/lib/glibc/ld-linux-* /lib64/ \
    && echo /usr/local/lib/glibc > /etc/ld.so.conf \
    && LD_LIBRARY_PATH=/usr/local/lib/glibc /tmp/ldconfig

ENV DENO_USE_CGROUPS=1
ENV ESMDIR="/esm"

WORKDIR /esm
EXPOSE 80
USER esm
RUN git --version && /esm/bin/deno --version
CMD ["esmd"]
