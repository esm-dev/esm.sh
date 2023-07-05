#######################################
#
# Build
#
#######################################
FROM golang:1.19-alpine AS build-stage

ENV ESM_SH_GIT_URL https://github.com/esm-dev/esm.sh

RUN apk add --no-cache git

RUN git clone --branch main $ESM_SH_GIT_URL /tmp/esm.sh

WORKDIR /tmp/esm.sh

RUN CGO_ENABLED=0 GOOS=linux go build -o esmd main.go

#######################################
#
# Release
#
#######################################
FROM node:18-alpine AS release-stage

COPY --from=build-stage /tmp/esm.sh/esmd /bin/esmd
RUN apk update && apk add --no-cache libcap-utils

RUN setcap cap_net_bind_service=ep /bin/esmd
RUN chown node:node /bin/esmd

RUN npm i -g pnpm

USER node
WORKDIR /

EXPOSE 8080
CMD ["/bin/esmd"]
