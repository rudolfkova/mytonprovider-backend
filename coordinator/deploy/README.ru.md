# Тестовый деплой coordinator на VPS

**[English version](README.md)**

Стек:
- `coordinator`
- `postgres` (инициализация схемы из `coordinator/db/init.sql`)
- `prometheus`
- `grafana`

## Место на диске (VPS)

`task coordinator:deploy:up` запускает **`docker compose ... --build`**: образ coordinator **собирается на VPS** (см. `build:` у сервиса `coordinator` в [docker-compose.yml](docker-compose.yml)). Ожидайте расход диска на образ Go builder и кэш сборки — это тяжелее, чем hub-режим **агента**.

Для **агентов** на тех же или других VPS используйте рекомендуемый hub-путь: [agent/deploy/README.ru.md](README.ru.md) (сборка образа локально, pull на VPS).

**Продвинутый вариант (вручную):** соберите и запушьте образ coordinator на dev-машине, на VPS в compose замените `build:` на `image:` (в Taskfile не автоматизировано):

```bash
# Dev-машина, из корня репо
docker build -f coordinator/Dockerfile -t <user>/mytonprovider-coordinator:latest .
docker push <user>/mytonprovider-coordinator:latest
```

На VPS у сервиса `coordinator` уберите блок `build:` и укажите `image: <user>/mytonprovider-coordinator:latest`, не используйте `--build` в compose.

## 1) Подготовка

Из корня репозитория:

```bash
task coordinator:deploy:init
```

Правка env:

```bash
nano coordinator/deploy/.env
```

Секреты:
- `coordinator/deploy/secrets/agents-ca.crt` — CA для проверки TLS агентов
- `coordinator/deploy/secrets/metrics.token` — токен для scrape `/metrics` в Prometheus

Содержимое `metrics.token` должно входить в `SYSTEM_ACCESS_TOKENS` в `.env`.

## 2) Запуск

```bash
task coordinator:deploy:up
```

## 3) Smoke checks

- Контейнеры:
  ```bash
  task coordinator:deploy:ps
  ```
- Health coordinator:
  ```bash
  curl -fsS "http://127.0.0.1:${COORDINATOR_PORT:-8080}/health"
  ```
- Prometheus: `http://<vps-ip>:${PROMETHEUS_PORT}`
- Grafana: `http://<vps-ip>:${GRAFANA_PORT}`

## 4) Логи / остановка

```bash
task coordinator:deploy:logs
task coordinator:deploy:down
```
