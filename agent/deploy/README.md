# Agent VPS test deploy

This stack runs:
- `agent` (gRPC + ADNL)
- `prometheus`
- `grafana`
- `loki`

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
