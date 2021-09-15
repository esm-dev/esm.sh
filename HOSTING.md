# Self-Hosting

You will need [Go](https://golang.org/dl) 1.16+ to compile the server, and ensure [supervisor](http://supervisord.org/) installed on your host machine.<br>
The server runtime will install the nodejs (14 LTS) automatically.

```bash
$ git clone https://github.com/postui/esm.sh
$ cd esm.sh
$ sh ./scripts/deploy.sh
```

**Deploying with Docker:** An example [Dockerfile](./Dockerfile) is found in the root of this project.
