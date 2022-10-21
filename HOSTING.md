# Self-Hosting

[esm.sh](https://esm.sh) provides a fast, global content delivery network
publicly which powered by [Cloudflare](https://cloudflare.com). You also can
deploy your own esm.sh CDN.

To build and deploy your CDN, You will need [Go](https://golang.org/dl) 1.16+ to
compile the server. The server runtime will install the nodejs (16 LTS)
automatically.

## Clone code

```baseh
git clone https://github.com/ije/esm.sh
cd esm.sh
```

## Run the sever locally

```bash
go run main.go --port=8080 --dev
```

Then you can import `React` from http://localhost:8080/react

## Deploy to remote host

Please ensure the [supervisor](http://supervisord.org/) has been installed on
your host machine.

```bash
./scripts/deploy.sh
```

Server options:

- `port` - the port to listen
- `httpsPort` - the port to listen https (use
  [autocert](golang.org/x/crypto/acme/autocert))
- `etcDir` - the workding directory (default is `.esmd/`)
- `cacheUrl` - the cache url (default is `memory:main`)
- `fsUrl` - the fs (storage) url (default is `local:$etcDir/storage`)
- `dbUrl` - the database url (default is `postdb:$etcDir/esm.db`)
- `origin` - the origin of the CDN
- `npmRegistry` - the npm registry (default is https://npmjs.org/registry)
- `npmToken` - the private token for npm registry

## Deploy with Docker

An example [Dockerfile](./Dockerfile) is found in the root of this project.
