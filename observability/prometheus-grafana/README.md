# Prometheus + Grafana + Loki (local stack)

**[Русская версия](README.ru.md)**

## Agent metrics (Prometheus)

The Prometheus container reaches the host via **`host.docker.internal`**. The agent must listen on **`0.0.0.0:9090`** for metrics, otherwise the scrape target stays DOWN.

```bash
export AGENT_METRICS_LISTEN_ADDR=0.0.0.0:9090
```

## Start compose

From this directory:

```bash
task up
# from repo root: task -t observability/prometheus-grafana/Taskfile.yml up
```

- **Prometheus:** http://127.0.0.1:9091  
- **Grafana:** http://127.0.0.1:3000 — `admin` / `admin`  
- **Loki:** http://127.0.0.1:3100 — **push** from agent (`POST /loki/api/v1/push`)

Datasources **Prometheus** and **Loki** are provisioned from `grafana/provisioning`.

## RunChecks: Grafana tables (no plugins)

After each **RunChecks**, the agent can push compact JSON lines to Loki (one per job + one per **storage IP**).

1. Start compose (`task up`).
2. Run the agent with **`AGENT_LOKI_URL=http://127.0.0.1:3100`** (same host as Loki; no trailing slash). Root **`task agent:run:test`** sets this URL.
3. In Grafana: **Dashboards → Agent → RunChecks jobs (Loki)** — job table and IP table (variable **job_id**, **All** = all jobs).

JSON fields include `valid`, `invalid`, `total`, `duration_ms`, `finished_unix`, and **`n_<REASON_CODE>`** counters (numbers; missing codes are zero).

If **`AGENT_LOKI_URL` is empty**, no push is performed.

## Dashboards

- **Mytonprovider agent** — Prometheus metrics (gRPC, RunChecks counters, etc.).
- **RunChecks jobs (Loki)** — tables from push events.

## Stop

```bash
task down
task down:clean   # removes Prometheus / Grafana / Loki volumes
```

## Explore examples

**Prometheus:** `agent_grpc_requests_total`, `rate(agent_grpc_requests_total[1m])`

**Loki:** `{job="runchecks", event="job"}`, `{job="runchecks", event="ip"}`
