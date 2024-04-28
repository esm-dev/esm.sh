# Self-Hosting

[esm.sh](https://esm.sh) provides a global fast CDN publicly which is powered by
[Cloudflare](https://cloudflare.com). You can also host esm.sh service by yourself.

## Clone the Source Code

```baseh
git clone https://github.com/esm-dev/esm.sh
cd esm.sh
```

## Configration

To configure the server, create a `config.json` file then pass it to the server bootstrap command. For example:

```jsonc
// config.json
{
  "port": 8080,
  "workDir": "/var/www/esmd",
  "storage": "local:/var/www/esmd/storage",
  "origin": "https://esm.sh",
  "npmRegistry": "https://registry.npmjs.org/",
  "npmToken": "xxxxxx"
}
```

You can find all the server options in [config.exmaple.jsonc](./config.example.jsonc). (**Note**: the
`config.example.jsonc` is not a valid JSON file, it's a JSONC file.)

## Run the Sever Locally

You will need [Go](https://golang.org/dl) 1.18+ to compile the server.

```bash
go run main.go --config=config.json --dev
```

Then you can import `React` from http://localhost:8080/react

## Deploy the Server to a Single Machine

Ensure the [supervisor](http://supervisord.org/) has been installed on your host machine.

```bash
./scripts/deploy.sh --init
```

Recommended host machine requirements:

- Linux system with `git` and `supervisor` installed
- 4x CPU cores or more
- 8GB RAM or more
- 100GB disk space or more

## Deploy with Docker

[![Docker Image](https://img.shields.io/github/v/tag/esm-dev/esm.sh?label=Docker&display_name=tag&sort=semver&style=flat&colorA=232323&colorB=232323&logo=docker&logoColor=eeeeee)](https://github.com/esm-dev/esm.sh/pkgs/container/esm.sh)

esm.sh provides a docker image for deployment. You can pull the container image from https://ghcr.io/esm-dev/esm.sh.

```bash
docker pull ghcr.io/esm-dev/esm.sh      # latest version
docker pull ghcr.io/esm-dev/esm.sh:v135 # specific version
```

Run the container:

```bash
docker run -p 8080:8080 \
  -e NPM_REGISTRY=https://registry.npmjs.org/ \
  -e NPM_TOKEN=xxxxxx \
  ghcr.io/esm-dev/esm.sh:latest
```

Available environment variables:

- `CDN_ORIGIN`: The origin of CDN, default is using the origin of the request.
- `CDN_BASE_PATH`: The base path of CDN, default is "/".
- `NPM_REGISTRY`: The NPM registry, default is "https://registry.npmjs.org/".
- `NPM_TOKEN`: The NPM token for private packages.
- `NPM_REGISTRY_SCOPE`: The NPM registry scope, default is no scope.
- `NPM_USER`: The NPM user for private packages.
- `NPM_PASSWORD`: The NPM password for private packages.
- `AUTH_SECRET`: The server auth secret, default is no authrization check.
- `DISABLE_COMPRESSION`: Disable compression, default is false.

You can also create your own Dockerfile with `ghcr.io/esm-dev/esm.sh`:

```dockerfile
FROM ghcr.io/esm-dev/esm.sh:v135
ADD ./config.json /etc/esmd/config.json
CMD ["esmd", "--config", "/etc/esmd/config.json"]
```

## Deploy with Cloudflare Workers

We use [Cloudflare Workers](https://workers.cloudflare.com/) as the front layer to handle and cache esm.sh requests at
edge(earth). And we open sourced the code, you can use it to build your own esm.sh CDN that's running globally.

More details check [esm-worker](./packages/esm-worker/README.md).
