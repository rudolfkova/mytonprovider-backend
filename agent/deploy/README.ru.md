# Тестовый деплой агента на VPS

**[English version](README.md)**

Стек может включать:
- `agent` (gRPC + ADNL)
- `prometheus`
- `grafana`
- `loki`

## Production / VPS (Docker Hub) — рекомендуется

Для VPS с ограниченным диском: **соберите образ агента на dev-машине**, запушьте в Docker Hub (или другой registry), на VPS только **pull** — без кэша `golang` и сборки исходников на сервере.

| Шаг | Где | Действие |
|-----|-----|----------|
| 1 | Dev-машина | Клон репо, `docker login` |
| 2 | Dev-машина | `AGENT_IMAGE=<user>/mytonprovider-agent:<tag> task agent:image:build:push` |
| 3 | VPS | Клон репо (или только `agent/deploy/` + secrets), `task`, Docker |
| 4 | VPS | `task agent:hub:init` → правка `agent/deploy/.env.hub` → `task agent:hub:up` или `task agent:hub:stack:up` |

**Зачем:** `task agent:deploy:up` делает `docker compose ... --build` — на VPS тянется `golang:bookworm` и идёт компиляция. Hub compose (`docker-compose.hub.yml`, `docker-compose.hub.stack.yml`) используют `image: ${AGENT_IMAGE}` и `pull_policy: always` — без сборки агента на VPS.

### A) Сборка и push (dev-машина)

```bash
docker login
AGENT_IMAGE=<docker-user>/mytonprovider-agent:latest task agent:image:build:push
```

По отдельности: `task agent:image:build`, `task agent:image:push` (нужен `AGENT_IMAGE`).

### B) Только агент на VPS

```bash
task agent:hub:init
nano agent/deploy/.env.hub
task agent:hub:up
```

Перед `up` положите TLS:
- `agent/deploy/secrets/server.crt`
- `agent/deploy/secrets/server.key`

В `.env.hub`:
- `AGENT_IMAGE=<docker-user>/mytonprovider-agent:latest`
- `AGENT_AUTH_TOKEN=...`
- пути TLS `/run/secrets/server.crt` и `/run/secrets/server.key`
- `AGENT_METRICS_LISTEN_ADDR=:9090` и `AGENT_LOKI_URL=http://loki:3100`, если позже поднимете полный stack

### C) Агент + мониторинг на VPS (без сборки)

```bash
task agent:hub:stack:up
task agent:hub:stack:ps
```

UI:
- Prometheus: `http://<vps-ip>:${PROMETHEUS_PORT}`
- Grafana: `http://<vps-ip>:${GRAFANA_PORT}`

Остановка: `task agent:hub:stack:down` или `task agent:hub:down`.

### Задачи Task (hub)

| Task | Описание |
|------|----------|
| `agent:image:build` | Сборка образа локально (нужен `AGENT_IMAGE`) |
| `agent:image:push` | Push тега |
| `agent:image:build:push` | Сборка + push |
| `agent:hub:init` | Создать `.env.hub` из example |
| `agent:hub:up` / `down` / `ps` / `logs` | Только агент, pull образа |
| `agent:hub:stack:up` / `down` / `ps` / `logs` | Агент + Prometheus + Grafana + Loki |

### Smoke checks (hub)

```bash
task agent:hub:ps
grpc_health_probe -addr=127.0.0.1:${AGENT_GRPC_PORT:-8443} -tls -tls-no-verify
```

---

## Сборка на VPS (dev / отладка)

Тяжелее по диску и CPU: образ агента собирается на сервере через `docker compose ... --build`.

Файлы: `agent/deploy/docker-compose.yml`, `agent/deploy/.env`.

### 1) Подготовка

Из корня репозитория:

```bash
task agent:deploy:init
nano agent/deploy/.env
```

TLS:
- `agent/deploy/secrets/server.crt`
- `agent/deploy/secrets/server.key`

### 2) Запуск

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

### 4) Логи / остановка

```bash
task agent:deploy:logs
task agent:deploy:down
```
