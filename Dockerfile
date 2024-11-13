#####################################################
#
# Build Stage
#
#####################################################
FROM golang:1.22-alpine AS build-stage

ENV ESM_SH_VERSION v135_7
ENV ESM_SH_GIT_URL https://github.com/esm-dev/esm.sh

RUN apk update && apk add --no-cache git
RUN git clone --branch $ESM_SH_VERSION --depth 1 $ESM_SH_GIT_URL /tmp/esm.sh

WORKDIR /tmp/esm.sh
RUN CGO_ENABLED=0 GOOS=linux go build -o esmd main.go

#####################################################
#
# Release Stage
#
#####################################################
FROM node:20-alpine AS release-stage

RUN apk update && apk add --no-cache git git-lfs libcap-utils
RUN git lfs install
RUN npm i -g pnpm

COPY --from=build-stage /tmp/esm.sh/esmd /bin/esmd
RUN setcap cap_net_bind_service=ep /bin/esmd
RUN chown node:node /bin/esmd

USER node
WORKDIR /
EXPOSE 8080
CMD ["esmd"]
