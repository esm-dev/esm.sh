# Self-Hosting

[esm.sh](https://esm.sh) provides a fast, global content delivery network publicly using Cloudflare, but you also can deploy your own CDN.<br>
You will need [Go](https://golang.org/dl) 1.16+ to compile the server. The server runtime will install the nodejs (14 LTS) automatically.

## Clone code

```baseh
git clone https://github.com/ije/esm.sh
cd esm.sh
```

## Run the sever locally

```bash
go run main.go --port=8080 --dev
```

then you can import `React` from http://localhost:8080/react

## Deploy to remote server

Please ensure the [supervisor](http://supervisord.org/) installed on your host machine.

```bash
./scripts/deploy.sh
```

## Deploy with Docker

An example [Dockerfile](./Dockerfile) is found in the root of this project.
