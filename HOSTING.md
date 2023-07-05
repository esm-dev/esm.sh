# Self-Hosting

[esm.sh](https://esm.sh) provides a fast, global content delivery network
publicly which powered by [Cloudflare](https://cloudflare.com). You may also
want to host esm.sh by yourself.

To serve esm.sh, You will need [Go](https://golang.org/dl) 1.18+ to run and
compile the server. The server will install
[Node.js](https://nodejs.org/en/download/) runtime automatically if it's not
found on your host machine.

## Recommended Host Machine (Single Server)

- Linux system with git installed
- 4x CPU cores or more
- 8GB RAM or more
- 100GB disk space or more

## Clone code

```baseh
git clone https://github.com/esm-dev/esm.sh
cd esm.sh
```

## Configration

To configure the server, create a `config.json` file then pass it to the server
bootstrap command. For example:

```jsonc
// config.json
{
  "port": 8080,
  "workDir": "/var/www/esmd",
  "storage": "local:/var/www/esmd/storage",
  "origin": "https://esm.sh",
  "npmRegistry": "https://npmjs.org/registry",
  "npmToken": "xxxxxx"
}
```

You can find all the server options in
[config.exmaple.jsonc](./config.example.jsonc). (**Note**: the
`config.example.jsonc` is not a valid JSON file, it's a JSONC file.)

## Run the Sever Locally

```bash
go run main.go --config=config.json --dev
```

Then you can import `React` from http://localhost:8080/react

## Deploy to Single Machine with the Quick Deploy Script

Please ensure the [supervisor](http://supervisord.org/) has been installed on
your host machine.

```bash
./scripts/deploy.sh --init
```

## Deploy with Docker

esm.sh provides an official docker image for deployment. You can pull the container image from https://ghcr.io/esm-dev/esm.sh:

```bash
docker pull ghcr.io/esm-dev/esm.sh:v127   # specific version
docker pull ghcr.io/esm-dev/esm.sh:latest # latest stable version
docker pull ghcr.io/esm-dev/esm.sh:dev    # latest dev version
```

Then run the container:

```bash
docker run -p 8080:8080 \
  -e NPM_REGISTRY=https://npmjs.org/registry \
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
- `SERVER_AUTH_SECRET`: The server auth secret, default is no auth.

You can also create your own Dockerfile with `ghcr.io/esm-dev/esm.sh`:

```dockerfile
FROM ghcr.io/esm-dev/esm.sh
ADD ./config.json /etc/esmd/config.json
CMD ["esmd", "--config", "/etc/esmd/config.json"]
```

## Deploy with Cloudflare Workers

We use [Cloudflare Workers](https://workers.cloudflare.com/) as the CDN layer to
handle and cache esm.sh requests at edge(earth). We open sourced the code, you
can use it to build your own esm CDN without deploying the server easily.

More details check [esm-worker](./packages/esm-worker/README.md).

## Deploy with Deno

We also provide a server for [Deno](https://deno.land) which is powered by the [esm-worker](./packages/esm-worker/README.md).

```bash
deno run -A https://esm.sh/v127/server --port=8080
```
