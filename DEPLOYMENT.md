# Deployment: Docker Hub images and cloud gateways

Everything here is configured on the root console's **Deployment** tab. As on the
Workspaces tab, each section has a form: the Docker Hub account, repository and token
(with a check for whether the repository exists and a button to create it), the images
(tag → compose build service), the GCP project, zone and service-account key, and the
cloud targets. Tokens and keys are write-only: you can set, replace or clear them, but
they're never shown again.

The settings are saved to **`~/.d8z/deployment.yaml`**, next to `workspace.yaml`.
That directory is mounted into the root gateway at `/etc/d8z` and lives outside every
repository. The GCP key is saved beside the file (mode 600).

A workspace gateway's Deployment tab shows the **root's** settings and can edit them.
Its calls are relayed to the root over NATS on `ctl.<id>.deploy`, encrypted with that
gateway's key in both directions, the same way its workspace assignment travels. Docker
Hub and cloud jobs started there currently run on the root.

The reference below describes the file the console writes; you shouldn't need to edit it by hand.

## deployment.yaml

```yaml
docker_hub:                     # one repository, one tag per named container
  username: doughznutz          # default: DOCKER_HUB_USERNAME (.env)
  repository: doughznutz        # default: DOCKER_HUB_REPO (.env)
  token_env: DOCKER_HUB_TOKEN   # read from .env (default) - or `token: dckr_pat_...`
  images:                       # tag -> the docker-compose.yaml service (profile dh) that builds it
    - { name: dood, service: dood-dh }   # doughznutz/doughznutz:dood
    - { name: d8z,     service: deightz-dh }   # doughznutz/doughznutz:d8z

gcp:
  project: my-gcp-project       # default: CLOUD_PROJECT_ID (.env)
  zone: us-west1-b              # default: COMPUTE_ZONE (.env)
  credentials: gcp-sa.json      # service-account key, a file in ~/.d8z (see below)

targets:                        # cloud gateways
  - name: cloud1                # its NATS id (GW_ID) and VM name (d8z-cloud1); a-z0-9, max 16
    profile: root               # root (dood) | workspace (dood + deightz)
    image: dood              # which docker_hub image is the gateway (default dood)
    machine_type: e2-small      # default e2-small
    disk_gb: 20                 # default 20
    allow_from: [203.0.113.7/32]  # REQUIRED: who may reach :80 (console) and :7422 (NATS leaf)
    forward_env: []             # extra .env keys to give the VM, e.g. [GEMINI_API_KEY]
```

With no file at all, `docker_hub` defaults from `.env` and there are no targets.

## Docker Hub jobs

| Job | Does |
|---|---|
| `build` | `docker compose --profile dh build <service>` from the project's **live checkout** (not the copy baked into the gateway image), tagging `<username>/<repository>:<name>` |
| `push` | logs in with a throwaway docker config and pushes the image(s) |
| `build_and_push` | both |
| `pull` | pulls the image(s) to this daemon |
| `list_tags` / `check_repo` / `create_repo` / `list_repos` | Hub API |

`image` is `all` or one name from `images`. The runtime `dood` and `deightz` services in
`docker-compose.yaml` use exactly these references, so a local `build` is immediately
what `docker compose --profile root up -d` runs.

## Cloud gateways (GCP)

All GCP work runs natively in gw-server through the Compute Engine REST API.
It authenticates as the service account in `gcp.credentials`, so you don't need gcloud.

`deploy` on a target:

1. Creates or updates the firewall rule `d8z-gw-<name>`: tcp 80 and 7422 from `allow_from`, applied only to that VM (by network tag).
2. Creates the VM `d8z-<name>` (Container-Optimized OS). If it already exists, it gets fresh metadata and a restart, which makes it pull the latest images.
3. On every boot, the VM's startup script (`dev/dood/server/cloud-startup.sh`) pulls the gateway image and runs `compose --profile <profile> up -d` from the `docker-compose.yaml` baked into it. The VM's `.env` arrives in instance metadata.
4. Waits for the console to answer on `http://<ip>/`.
5. Connects the local root's NATS to the VM's leaf port and pings it.

The cloud gateway's `.env` contains `USER`, `PROJECT`, `DOCKER_PASSWORD` (the console's
basic-auth password), `CODE_SERVER_PASSWORD`, the Docker Hub names, its `GW_ID`, and its
NATS leaf credentials, plus any `forward_env` keys. **Anyone with Compute viewer access to
the GCP project can read instance metadata**, so forward only what that VM needs.

Other jobs: `status` (refresh state and ping), `start`, `stop` (releases the ephemeral IP), `delete` (VM, its disk, and its firewall rule), `list`.
The VM's address is recorded in `~/.d8z/deployment.state.json`.

### NATS to the cloud

The local root dials each running cloud gateway as a **leaf remote**, in the same direction
it already dials its local children: `nats-leaf://d8z:<password>@<ip>:7422`.
The password is derived from `~/.d8z/.gw-master-key` (as workspace gateway keys are), so nothing extra is stored.
The cloud gateway's leaf listener requires those credentials.

A message published to `gw.<name>.inbox` lands in the cloud gateway's own JetStream inbox.
`ctl.<name>.ping` is answered by its gw-server.
On the Deployment page, each running target has a **NATS** box: it sends to the inbox, pings the gateway, and shows the reply.

> Port 80 (basic auth) and the leaf link are plain TCP, not TLS - same as the local console.
> `allow_from` should be your own IP/32. TLS for both is a natural next step.

### GCP service account

1. In the [Cloud Console](https://console.cloud.google.com/iam-admin/serviceaccounts), pick your project and **Create service account** (for example `d8z-deployer`).
2. Grant it **Compute Admin** (`roles/compute.admin`); it creates VMs and firewall rules. The VMs themselves run with no service account: they only pull public images from Docker Hub.
3. Under **Keys**, select **Add key → Create new key → JSON**.
4. On the Deployment tab, open **Google Cloud → Edit**, pick the downloaded file (or paste it), and save.

The project needs billing and the Compute Engine API enabled.
