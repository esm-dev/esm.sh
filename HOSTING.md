# Self-Hosting

[esm.sh](https://esm.sh) provides a fast, global content delivery network pulicly, but you also can deploy your own CDN for ES Modules.<br>
You will need [Go](https://golang.org/dl) 1.16+ to compile the server. The server runtime will install the nodejs (14 LTS) automatically.

## Clone code

```baseh
git clone https://github.com/alephjs/esm.sh
cd esm.sh
```

## Run the sever locally

```bash
go run main.go --port=8080 --dev
```

then you can import `React` from http://localhost:8080/react

## Depoly to single host

Please ensure the [supervisor](http://supervisord.org/) installed on your host machine.

```bash
sh ./scripts/deploy.sh
```

## Depoly to multiple hosts

- deploy manually
- deploy automatically

_We are working on it._

## Deploying with Docker

An example [Dockerfile](./Dockerfile) is found in the root of this project.
