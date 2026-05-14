# Prometheus + Grafana + Loki (локальный стек)

## Метрики агента (Prometheus)

Контейнер Prometheus ходит на хост через **`host.docker.internal`**. Агент должен слушать метрики на **`0.0.0.0:9090`**, иначе target будет DOWN.

```bash
export AGENT_METRICS_LISTEN_ADDR=0.0.0.0:9090
```

## Запуск compose

Из этой директории:

```bash
task up
# из корня: task -t observability/prometheus-grafana/Taskfile.yml up
```

- **Prometheus:** http://127.0.0.1:9091  
- **Grafana:** http://127.0.0.1:3000 — `admin` / `admin`  
- **Loki:** http://127.0.0.1:3100 — приём **push** от агента (`POST /loki/api/v1/push`)

Datasources **Prometheus** и **Loki** подключаются из `grafana/provisioning`.

## RunChecks: таблицы в Grafana (без плагинов)

После каждого **RunChecks** агент может отправлять в Loki компактные JSON-строки (одна на джобу + по одной на каждый **storage IP**).

1. Подними compose (`task up`).
2. Запусти агента с **`AGENT_LOKI_URL=http://127.0.0.1:3100`** (если Loki на том же хосте; без trailing slash). Корневой **`task agent:run:test`** уже задаёт этот URL.
3. В Grafana: **Dashboards → Agent → RunChecks jobs (Loki)** — таблица джоб и таблица по IP (переменная **job_id**, значение **All** = все джобы).

Поля в JSON включают `valid`, `invalid`, `total`, `duration_ms`, `finished_unix` и счётчики **`n_<REASON_CODE>`** (числа, нули для отсутствующих кодов).

Если **`AGENT_LOKI_URL` пустой**, push не выполняется.

## Дашборды

- **Mytonprovider agent** — метрики Prometheus (gRPC, RunChecks counters и т.д.).
- **RunChecks jobs (Loki)** — таблицы по push-событиям.

## Остановка

```bash
task down
task down:clean   # удалит volumes Prometheus / Grafana / Loki
```

## Примеры в Explore

**Prometheus:** `agent_grpc_requests_total`, `rate(agent_grpc_requests_total[1m])`

**Loki:** `{job="runchecks", event="job"}`, `{job="runchecks", event="ip"}`
