# Self-Hosting

[esm.sh](https://esm.sh) provides a fast, global content delivery network
publicly which powered by [Cloudflare](https://cloudflare.com). You may also want to
host esm.sh by yourself.

To serve esm.sh, You will need [Go](https://golang.org/dl) 1.18+ to
run and compile the server. The server runtime will install the nodejs (16 LTS)
automatically.

## Clone code

```baseh
git clone https://github.com/esm-dev/esm.sh
cd esm.sh
```

## Configration

To configure the server, you need to create a `config.json` file then pass it to the server bootstrap command. For example:

```jsonc
// config.json
{
  "port": 8080,
  "tlsPort": 443,
  "workDir": "/var/www/esmd",
  "storage": "local:/var/www/esmd/storage",
  "origin": "https://esm.sh",
  "npmRegistry": "https://npmjs.org/registry",
  "npmToken": "xxxxxx"
}
```

You can find all the server options in [config.exmaple.jsonc](./config.example.jsonc). (**Note**: the `config.example.jsonc` is not a valid JSON file, it's a JSONC file.)

## Run the sever locally

```bash
go run main.go --config=config.json --dev
```

Then you can import `React` from http://localhost:8080/react

## Deploy to remote host with the quick deploy script

Please ensure the [supervisor](http://supervisord.org/) has been installed on
your host machine.

```bash
./scripts/deploy.sh --init
```

## Deploy with Docker

An example [Dockerfile](./Dockerfile) is found in the root of this project.
