#######################################
#
# Build
#
#######################################
FROM golang:1.19-alpine AS build-stage

RUN apk add --no-cache git
RUN git clone --branch main https://github.com/esm-dev/esm.sh /tmp/esm.sh

WORKDIR /tmp/esm.sh
RUN CGO_ENABLED=0 GOOS=linux go build -o esmd main.go

#######################################
#
# Release
#
#######################################
FROM node:18-alpine AS release-stage

RUN apk update && apk add --no-cache libcap-utils
RUN npm i -g pnpm

COPY --from=build-stage /tmp/esm.sh/esmd /bin/esmd
RUN setcap cap_net_bind_service=ep /bin/esmd
RUN chown node:node /bin/esmd

USER node
WORKDIR /
EXPOSE 8080
CMD ["esmd"]
