FROM golang:1.17 AS build

RUN apt-get update -y && apt-get install -y xz-utils

ADD . /esm.sh
WORKDIR /esm.sh

RUN --mount=type=cache,target=/go/pkg/mod go build -o esmd main.go

RUN useradd -u 1000 -m esm
RUN chown -R esm:esm /esm.sh
USER esm

ENTRYPOINT ["/esm.sh/esmd"]
