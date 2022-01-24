FROM golang:1.17 AS build

RUN apt-get update -y && apt-get install -y xz-utils

RUN useradd -u 1000 -m esm
USER esm

ADD . /esm
WORKDIR /esm

RUN --mount=type=cache,target=/go/pkg/mod go build -o bin/esmd main.go

ENTRYPOINT ["/esm/bin/esmd", "--etc-dir", "/esm"]
