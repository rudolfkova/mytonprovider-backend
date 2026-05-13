# Prometheus + Grafana (локальный стек)

Скрейпит **`/metrics`** агента на хост-машине.

## Важно про порт метрик агента

Контейнер Prometheus ходит на хост через **`host.docker.internal`**. На Linux сервис, слушающий только **`127.0.0.1:9090`**, часто **недоступен** из Docker.

Запускай агента с:

```bash
export AGENT_METRICS_LISTEN_ADDR=0.0.0.0:9090
# плюс остальные AGENT_* как обычно
```

Локально безопаснее ограничить фаерволом доступ к `9090` снаружи, если машина в открытой сети.

## Запуск

Из этой директории:

```bash
task up
# эквивалент: docker compose up -d
# из корня репозитория: task -t observability/prometheus-grafana/Taskfile.yml up
```

- **Prometheus:** http://127.0.0.1:9091 — раздел **Status → Targets**, цель `mytonprovider-agent` должна быть **UP**.
- **Grafana:** http://127.0.0.1:3000 — логин **`admin`**, пароль **`admin`** (смени после первого входа).

Источник данных **Prometheus** подключается автоматически (`grafana/provisioning`).

### Дашборд «из коробки»

После входа в Grafana: **Dashboards → папка Agent → Mytonprovider agent** — там графики по gRPC (RPS, коды, p95), RunChecks и RunStorageRates.

Если стэк уже был запущен до появления дашборда, перезапусти Grafana: `docker compose restart grafana` или `task down` → `task up`.

### Ad-hoc в Explore

**Explore** → datasource **Prometheus** — можно писать любой PromQL (см. примеры внизу).

## Остановка

```bash
task down
# полный сброс данных в volumes: task down:clean
```

Данные TSDB и Grafana сохраняются в **именованных Docker volumes** (`prometheus_tsdb`, `grafana_data`). Полный сброс: `task down:clean` (или `docker compose down -v`).

## Примеры запросов в Grafana → Explore

- `agent_grpc_requests_total`
- `rate(agent_grpc_requests_total[1m])`
- `histogram_quantile(0.95, sum(rate(agent_grpc_request_duration_seconds_bucket[5m])) by (le, grpc_method))`
