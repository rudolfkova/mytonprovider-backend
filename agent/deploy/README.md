# Agent VPS test deploy

**[Русская версия](README.ru.md)**

This stack can run:
- `agent` (gRPC + ADNL)
- `prometheus`
- `grafana`
- `loki`

## Production / VPS (Docker Hub) — recommended

Use this on VPS with limited disk: **build the agent image on your dev machine**, push to Docker Hub (or another registry), then on the VPS only **pull** the image — no `golang` build cache or source compile on the server.

| Step | Where | Action |
|------|--------|--------|
| 1 | Dev machine | Clone repo, `docker login` |
| 2 | Dev machine | `AGENT_IMAGE=<user>/mytonprovider-agent:<tag> task agent:image:build:push` |
| 3 | VPS | Repo clone (or copy `agent/deploy/` + secrets only), `task`, Docker |
| 4 | VPS | `task agent:hub:init` → edit `agent/deploy/.env.hub` → `task agent:hub:up` or `task agent:hub:stack:up` |

**Why:** `task agent:deploy:up` runs `docker compose ... --build`, which pulls `golang:bookworm` and compiles on the VPS. Hub compose files (`docker-compose.hub.yml`, `docker-compose.hub.stack.yml`) use `image: ${AGENT_IMAGE}` with `pull_policy: always` — no agent build on VPS.

### A) Build and push (dev machine)

```bash
docker login
AGENT_IMAGE=<docker-user>/mytonprovider-agent:latest task agent:image:build:push
```

Separate steps: `task agent:image:build`, `task agent:image:push` (both require `AGENT_IMAGE`).

### B) Agent only on VPS

```bash
task agent:hub:init
nano agent/deploy/.env.hub
task agent:hub:up
```

Place TLS files before `up`:
- `agent/deploy/secrets/server.crt`
- `agent/deploy/secrets/server.key`

In `.env.hub` set:
- `AGENT_IMAGE=<docker-user>/mytonprovider-agent:latest`
- `AGENT_AUTH_TOKEN=...`
- TLS paths `/run/secrets/server.crt` and `/run/secrets/server.key`
- `AGENT_METRICS_LISTEN_ADDR=:9090` and `AGENT_LOKI_URL=http://loki:3100` if you use the full stack later

### C) Agent + monitoring on VPS (still no build)

```bash
task agent:hub:stack:up
task agent:hub:stack:ps
```

UIs:
- Prometheus: `http://<vps-ip>:${PROMETHEUS_PORT}`
- Grafana: `http://<vps-ip>:${GRAFANA_PORT}`

Stop: `task agent:hub:stack:down` or `task agent:hub:down`.

### Hub Task reference

| Task | Description |
|------|-------------|
| `agent:image:build` | Build image locally (`AGENT_IMAGE` required) |
| `agent:image:push` | Push existing tag |
| `agent:image:build:push` | Build + push |
| `agent:hub:init` | Create `.env.hub` from example |
| `agent:hub:up` / `down` / `ps` / `logs` | Agent only, pull image |
| `agent:hub:stack:up` / `down` / `ps` / `logs` | Agent + Prometheus + Grafana + Loki |

### Smoke checks (hub)

```bash
task agent:hub:ps
grpc_health_probe -addr=127.0.0.1:${AGENT_GRPC_PORT:-8443} -tls -tls-no-verify
```

---

## Build on VPS (dev / debugging)

Heavier on disk and CPU: compiles the agent image on the server via `docker compose ... --build`.

Uses `agent/deploy/docker-compose.yml` and `agent/deploy/.env`.

### 1) Prepare

From repository root:

```bash
task agent:deploy:init
nano agent/deploy/.env
```

TLS files:
- `agent/deploy/secrets/server.crt`
- `agent/deploy/secrets/server.key`

### 2) Start

```bash
task agent:deploy:up
```

### 3) Smoke checks

```bash
task agent:deploy:ps
grpc_health_probe -addr=127.0.0.1:${AGENT_GRPC_PORT:-8443} -tls -tls-no-verify
```

- Prometheus: `http://<vps-ip>:${PROMETHEUS_PORT}`
- Grafana: `http://<vps-ip>:${GRAFANA_PORT}`

### 4) Logs / stop

```bash
task agent:deploy:logs
task agent:deploy:down
```
