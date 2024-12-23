# build the server from source code
FROM golang:1.23-alpine AS builder

ENV ESM_SH_REPO https://github.com/esm-dev/esm.sh
ENV ESM_SH_VERSION v136

RUN apk update && apk add --no-cache git
RUN git clone --branch $ESM_SH_VERSION --depth 1 $ESM_SH_REPO /tmp/esm.sh

WORKDIR /tmp/esm.sh
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o esmd main.go

# server
FROM alpine:latest AS server

ENV SERVER_PORT 8080
ENV SERVER_WORKDIR /esmd

RUN apk update && \
    apk add --no-cache git git-lfs && \
    git lfs install

COPY --from=builder /tmp/esm.sh/esmd /bin/esmd
COPY --from=denoland/deno:bin-2.1.4 /deno /esmd/bin/deno

# add esm(non-root) user
RUN addgroup -g 1000 esm && \
    adduser -u 1000 -G esm -D esm && \
    chown -R esm:esm /esmd

USER esm
WORKDIR /tmp
EXPOSE 8080
CMD ["esmd"]
