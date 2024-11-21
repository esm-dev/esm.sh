# Self-Hosting

[esm.sh](https://esm.sh) provides a global fast CDN publicly which is powered by [Cloudflare](https://cloudflare.com).
You can also host esm.sh service by yourself. Please follow the instructions below.

## Clone the Source Code

```bash
git clone https://github.com/esm-dev/esm.sh
cd esm.sh
```

## Configuration

To configure the server, create a `config.json` file then pass it to the server bootstrap command. For example:

```jsonc
// config.json
{
  "port": 8080,
  "npmRegistry": "https://registry.npmjs.org/",
  "npmToken": "******"
}
```

You can find all the server options in [config.example.jsonc](./config.example.jsonc).

## Run the Server Locally

You will need [Go](https://golang.org/dl) 1.22+ to compile and run the server.

```bash
go run main.go --config=config.json
```

Then you can import `React` from <http://localhost:8080/react>.

## Deploy the Server to a Single Machine

We provide a bash script to deploy the server to a single machine.

```bash
# first time deploy
./scripts/deploy.sh --init
# update the server
./scripts/deploy.sh
```

Recommended host machine requirements:

- Linux system (Debian/Ubuntu)
- 4x CPU cores or more
- 8GB RAM or more
- 100GB disk space or more

## Deploy with Docker

[![Docker Image](https://img.shields.io/github/v/tag/esm-dev/esm.sh?label=Docker&display_name=tag&sort=semver&style=flat&colorA=232323&colorB=232323&logo=docker&logoColor=eeeeee)](https://github.com/esm-dev/esm.sh/pkgs/container/esm.sh)

esm.sh provides a Docker image for deployment. You can pull the container image from <https://ghcr.io/esm-dev/esm.sh>.

```bash
docker pull ghcr.io/esm-dev/esm.sh      # latest version
docker pull ghcr.io/esm-dev/esm.sh:v135 # specific version
```

Run the container:

```bash
docker run -p 8080:8080 \
  -e NPM_REGISTRY=https://registry.npmjs.org/ \
  -e NPM_TOKEN=****** \
  ghcr.io/esm-dev/esm.sh:latest
```

Available environment variables:

- `AUTH_SECRET`: The server auth secret, default is no authorization.
- `STORAGE_TYPE`: The storage type, available values are ["fs", "s3"], default is "fs".
- `STORAGE_ENDPOINT`: The storage endpoint, default is "~/.esmd/storage".
- `STORAGE_REGION`: The region for S3 storage.
- `STORAGE_ACCESS_KEY_ID`: The access key for S3 storage.
- `STORAGE_SECRET_ACCESS_KEY`: The secret key for S3 storage.
- `COMPRESS`: Compress http responses with gzip/brotli, default is `true`.
- `MINIFY`: Minify the built JS/CSS files, default is `true`.
- `SOURCEMAP`: Generate source map for built JS/CSS files, default is `true`.
- `LOG_LEVEL`: The log level, available values are ["debug", "info", "warn", "error"], default is "info".
- `NPM_REGISTRY`: The global NPM registry, default is "https://registry.npmjs.org/".
- `NPM_TOKEN`: The access token for the global NPM registry.
- `NPM_USER`: The access user for the global NPM registry.
- `NPM_PASSWORD`: The access password for the global NPM registry.
- `NPM_QUERY_CACHE_TTL`: The cache TTL for NPM query, default is 10 minutes.

You can also create your own Dockerfile based on `ghcr.io/esm-dev/esm.sh`:

```dockerfile
FROM ghcr.io/esm-dev/esm.sh:v135_6
ADD ./config.json /etc/esmd/config.json
CMD ["esmd", "--config", "/etc/esmd/config.json"]
```
