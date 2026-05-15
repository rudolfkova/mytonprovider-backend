# Agent VPS test deploy

This stack runs:
- `agent` (gRPC + ADNL)
- `prometheus`
- `grafana`
- `loki`

For low-disk VPS, prefer Docker Hub mode (no local image build): `docker-compose.hub.yml` / `docker-compose.hub.stack.yml`.

## 1) Prepare

From repository root:

```bash
task agent:deploy:init
```

Then edit env:

```bash
nano agent/deploy/.env
```

Place TLS files:
- `agent/deploy/secrets/server.crt`
- `agent/deploy/secrets/server.key`

## 2) Start

```bash
task agent:deploy:up
```

## 3) Smoke checks

- Containers:
  ```bash
  task agent:deploy:ps
  ```
- Agent health:
  ```bash
  grpc_health_probe -addr=127.0.0.1:${AGENT_GRPC_PORT:-8443} -tls -tls-no-verify
  ```
- Prometheus UI: `http://<vps-ip>:${PROMETHEUS_PORT}`
- Grafana UI: `http://<vps-ip>:${GRAFANA_PORT}`

## 4) Logs / stop

```bash
task agent:deploy:logs
task agent:deploy:down
```

## Docker Hub mode (no build on VPS)

You can run either:
- `agent` only (`docker-compose.hub.yml`)
- `agent + prometheus + grafana + loki` (`docker-compose.hub.stack.yml`)

### A) Build and push image from local machine

```bash
docker login
AGENT_IMAGE=<docker-user>/mytonprovider-agent:latest task agent:image:build:push
```

### B) Start on VPS

```bash
task agent:hub:init
nano agent/deploy/.env.hub
task agent:hub:up
```

In `.env.hub` set:
- `AGENT_IMAGE=<docker-user>/mytonprovider-agent:latest`
- `AGENT_AUTH_TOKEN=...`
- keep TLS paths `/run/secrets/server.crt` and `/run/secrets/server.key`
- keep `AGENT_METRICS_LISTEN_ADDR=:9090` and `AGENT_LOKI_URL=http://loki:3100` for metrics/logs

### C) Full observability stack on VPS (still no build)

```bash
task agent:hub:stack:up
task agent:hub:stack:ps
```

UIs:
- Prometheus: `http://<vps-ip>:${PROMETHEUS_PORT}`
- Grafana: `http://<vps-ip>:${GRAFANA_PORT}`

Stop:

```bash
task agent:hub:stack:down
```
