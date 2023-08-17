#######################################
#
# Build Stage
#
#######################################
FROM golang:1.20-alpine AS build-stage

RUN apk update && apk add --no-cache git
RUN git clone --branch main --depth 1 https://github.com/esm-dev/esm.sh /tmp/esm.sh

WORKDIR /tmp/esm.sh
RUN CGO_ENABLED=0 GOOS=linux go build -o esmd main.go

#######################################
#
# Release Stage
#
#######################################
FROM node:18-alpine AS release-stage

RUN apk update && apk add --no-cache git libcap-utils
RUN npm i -g pnpm

COPY --from=build-stage /tmp/esm.sh/esmd /bin/esmd
RUN setcap cap_net_bind_service=ep /bin/esmd
RUN chown node:node /bin/esmd

USER node
WORKDIR /
EXPOSE 8080
CMD ["esmd"]
