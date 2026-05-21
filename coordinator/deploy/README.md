# Coordinator VPS test deploy

**[Русская версия](README.ru.md)**

This stack runs:
- `coordinator`
- `postgres` (schema init from `coordinator/db/init.sql`)
- `prometheus`
- `grafana`

## Disk usage on VPS

`task coordinator:deploy:up` runs **`docker compose ... --build`**: the coordinator image is **compiled on the VPS** (see `build:` for the `coordinator` service in [docker-compose.yml](docker-compose.yml)). Expect extra disk use for the Go builder image and build cache — heavier than the agent **Docker Hub** flow.

For **agents** on the same or other VPS nodes, use the recommended hub path: [agent/deploy/README.md](agent/deploy/README.md) (build image locally, pull on VPS).

**Advanced (manual):** build and push the coordinator image on a dev machine, then on the VPS edit compose to use `image:` instead of `build:` (not automated in Taskfile):

```bash
# Dev machine, from repo root
docker build -f coordinator/Dockerfile -t <user>/mytonprovider-coordinator:latest .
docker push <user>/mytonprovider-coordinator:latest
```

On the VPS, replace the `coordinator` service `build:` block with `image: <user>/mytonprovider-coordinator:latest` and drop `--build` from compose invocations.

## 1) Prepare

From repository root:

```bash
task coordinator:deploy:init
```

Then edit env:

```bash
nano coordinator/deploy/.env
```

Place secret files:
- `coordinator/deploy/secrets/agents-ca.crt` — CA used to verify agent certs
- `coordinator/deploy/secrets/metrics.token` — token for Prometheus to scrape `/metrics`

`metrics.token` content must be included in `SYSTEM_ACCESS_TOKENS` in `.env`.

## 2) Start

```bash
task coordinator:deploy:up
```

## 3) Smoke checks

- Containers:
  ```bash
  task coordinator:deploy:ps
  ```
- Coordinator health:
  ```bash
  curl -fsS "http://127.0.0.1:${COORDINATOR_PORT:-8080}/health"
  ```
- Prometheus UI: `http://<vps-ip>:${PROMETHEUS_PORT}`
- Grafana UI: `http://<vps-ip>:${GRAFANA_PORT}`

## 4) Logs / stop

```bash
task coordinator:deploy:logs
task coordinator:deploy:down
```
