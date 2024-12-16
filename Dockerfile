# 1. build the server from source code
FROM golang:1.23-alpine AS build

ENV ESM_SH_REPO https://github.com/esm-dev/esm.sh
ENV ESM_SH_VERSION v136

RUN apk update && apk add --no-cache git
RUN git clone --branch $ESM_SH_VERSION --depth 1 $ESM_SH_REPO /tmp/esm.sh

WORKDIR /tmp/esm.sh
RUN CGO_ENABLED=0 GOOS=linux go build -o esmd main.go

# 2. run the server
FROM alpine AS server

ENV HOME /home

RUN apk update && apk add --no-cache git git-lfs libcap-utils
RUN git lfs install

RUN mkdir -p $HOME/.esmd/bin
COPY --from=build /tmp/esm.sh/esmd /bin/esmd
COPY --from=denoland/deno:bin-2.1.4 /deno $HOME/.esmd/bin/deno

## add esm(non-root) user
RUN addgroup -g 1000 esm
RUN adduser -u 1000 -G esm -D esm
RUN chown -R esm:esm $HOME/.esmd

## allow user esm(non-root) to listen 80 port
RUN setcap cap_net_bind_service=ep /bin/esmd

USER esm
WORKDIR /tmp
EXPOSE 80
CMD ["esmd"]
