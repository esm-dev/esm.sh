# build >>>
FROM golang:1.27-alpine AS builder

WORKDIR /tmp/esm.sh
COPY internal/ ./internal/
COPY server/ ./server/
COPY go.mod go.sum ./
RUN go mod download

ARG TARGETARCH
RUN set -eu; \
    case "$TARGETARCH" in \
        amd64) arch=x86_64 ;; \
        arm64) arch=aarch64 ;; \
        *) echo "Unsupported architecture: $TARGETARCH" >&2; exit 1 ;; \
    esac; \
    version=$(sed -n 's/^var cjsModuleLexerVersion = "\([^"]*\)"$/\1/p' server/cjs_module_lexer.go); \
    test -n "$version"; \
    wget -O "cjs-module-lexer-$version.gz" "https://github.com/esm-dev/cjs-module-lexer/releases/download/v$version/cjs-module-lexer-$arch-unknown-linux-gnu.gz"; \
    gzip -d "cjs-module-lexer-$version.gz"

ARG SERVER_VERSION="main"
RUN CGO_ENABLED=0 go build -ldflags="-s -w -X 'github.com/esm-dev/esm.sh/server.VERSION=${SERVER_VERSION}'" -o esmd server/esmd/main.go
# <<<

FROM alpine:latest

RUN apk add --no-cache git
RUN addgroup -g 1000 esm && adduser -u 1000 --home=/esm -G esm -D esm

COPY --from=builder /tmp/esm.sh/esmd /bin/esmd
COPY --from=builder --chown=esm:esm --chmod=755 /tmp/esm.sh/cjs-module-lexer-* /esm/bin/
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
RUN --network=none git --version && /esm/bin/deno --version \
    && echo 'exports.offline = true' > /tmp/lexer-test.cjs \
    && test "$(/esm/bin/cjs-module-lexer-* /tmp/lexer-test.cjs)" = "offline" \
    && rm /tmp/lexer-test.cjs
CMD ["esmd"]
