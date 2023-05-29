# syntax=docker/dockerfile:1
FROM golang:1.18 AS build

WORKDIR /app
COPY . .
RUN apt-get update -y && apt-get install -y xz-utils
RUN go build -o /esmd

FROM node:18-alpine3.16
ENV USER_ID=65535
ENV GROUP_ID=65535
ENV USER_NAME=esm
ENV GROUP_NAME=esm

RUN apk add --no-cache libc6-compat
RUN addgroup -g $GROUP_ID $GROUP_NAME && \
    adduser --shell /sbin/nologin --disabled-password \
    --uid $USER_ID --ingroup $GROUP_NAME $USER_NAME
RUN mkdir -p /usr/local/lib && chown -R $USER_NAME:$GROUP_NAME /usr/local

USER $USER_NAME

WORKDIR /home/esm
COPY --from=build /esmd /home/esm/esmd

RUN echo "{\"port\":80,\"workDir\":\"/home/esm/workdir\"}" >> /home/esm/config.json

ENTRYPOINT ["/home/esm/esmd", "--config", "config.json"]
