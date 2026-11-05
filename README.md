# Doughznutz Development Environment

## Description

This project contains the development environment for Doughznutz LLC. It utilizes Docker and open-source tools and can be run in either the cloud or locally. Hooks are provided for GitHub. Secrets, like API keys, are managed in cloud secrets or in .env files.

## Containers

The system ships as named containers, one tag each in the
[`doughznutz/doughznutz`](https://hub.docker.com/r/doughznutz/doughznutz) Docker Hub repository:

| Image | Built by (compose, profile `dh`) | What it is |
|---|---|---|
| `doughznutz/doughznutz:dood` | `dood-dh` | The gateway: Traefik + the gw-server console (control plane) |
| `doughznutz/doughznutz:d8z` | `deightz-dh` | Deightz: code-server IDE with the AI CLIs |

## Usage

```sh
mkdir -m 700 ~/.d8z                           # gateway config (outside every repo)
docker compose --profile root up -d           # dood only
docker compose --profile workspace up -d      # dood + deightz
```

Then open http://localhost/ (basic auth: `$USER` / `DOCKER_PASSWORD` from `.env`).
Optional services (og, gopher, dozzle) start from the console's **Gateway** tab.

To build the images yourself (e.g. after changing gw-server or the console):

```sh
docker compose --profile dh build             # tags doughznutz/doughznutz:{dood,d8z} locally
docker compose --profile root up -d           # recreate dood on the new image
```

Pushing to Docker Hub and deploying to the cloud are on the console's **Deployment**
page, driven by `~/.d8z/deployment.yaml` - see [DEPLOYMENT.md](DEPLOYMENT.md).

## Contributing

Email me: doughznutz@gmail.com

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
