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
  "npmRegistry": "https://registry.npmjs.org/",
  "npmToken": "xxxxxx"
}
```

You can find all the server options in [config.exmaple.jsonc](./config.example.jsonc). (**Note**: the
`config.example.jsonc` is not a valid JSON file, it's a JSONC file.)

## Run the Sever Locally

You will need [Go](https://golang.org/dl) 1.18+ to compile the server.

```bash
go run main.go --config=config.json
```

Then you can import `React` from http://localhost:8080/react

## Deploy the Server to a Single Machine

Ensure the [supervisor](http://supervisord.org/) has been installed on your host machine.

```bash
# first time deploy
./scripts/deploy.sh --init
# update the server
./scripts/deploy.sh
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

- `AUTH_SECRET`: The server auth secret, default is no authrization check.
- `BASE_PATH`: The base path of CDN, default is "/".
- `DISABLE_COMPRESSION`: Disable http compression, default is false.
- `DISABLE_SOURCEMAP`: Disable generating source map for build js files, default is false.
- `LOG_LEVEL`: The log level, available values are ["debug", "info", "warn", "error"], default is "info".
- `NPM_REGISTRY`: The global NPM registry, default is "https://registry.npmjs.org/".
- `NPM_TOKEN`: The access token for the global NPM registry.
- `NPM_USER`: The access user for the global NPM registry.
- `NPM_PASSWORD`: The access password for the global NPM registry.

You can also create your own Dockerfile with `ghcr.io/esm-dev/esm.sh`:

```dockerfile
FROM ghcr.io/esm-dev/esm.sh:v135
ADD ./config.json /etc/esmd/config.json
CMD ["esmd", "--config", "/etc/esmd/config.json"]
```
