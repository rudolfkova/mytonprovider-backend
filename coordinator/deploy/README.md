# Coordinator VPS test deploy

This stack runs:
- `coordinator`
- `postgres` (with schema init from `coordinator/db/init.sql`)
- `prometheus`
- `grafana`

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
- `coordinator/deploy/secrets/agents-ca.crt` - CA used to verify agent certs
- `coordinator/deploy/secrets/metrics.token` - token used by Prometheus to scrape `/metrics`

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
