# mytonprovider-backend

**[English version](README.md)**

Backend для [mytonprovider.org](https://mytonprovider.org) — мониторинг и проверки здоровья провайдеров TON Storage.

## Архитектура

Два Go-сервиса и общие protobuf-контракты в `contracts/`:

| Компонент | Назначение |
|-----------|------------|
| **coordinator** | HTTP API для фронтенда, Postgres, фоновые воркеры (жизненный цикл провайдеров, телеметрия, очистка), вызов проверок по gRPC к агентам |
| **agent** | gRPC (`RunChecks`, `RunStorageRates`), ADNL к провайдерам, опционально Prometheus и push в Loki |

```mermaid
flowchart LR
  Frontend --> Coordinator
  Coordinator -->|gRPC TLS| Agent
  Agent -->|ADNL| Providers[TON Storage providers]
  Coordinator --> Postgres
```

## Структура репозитория

```
├── agent/              # gRPC-агент проверок
├── coordinator/        # API, воркеры, db/init.sql
├── contracts/          # proto и сгенерированный Go
├── observability/      # локальный Prometheus + Grafana + Loki (dev)
├── Taskfile.yml        # proto, тесты, деплой
└── go.work
```

## Разработка

**Нужно:** Go 1.26+ ([go.work](go.work)), [Task](https://taskfile.dev), `grpcurl`, `openssl`, `protoc` (для `task proto:gen`).

```bash
# Перегенерация protobuf (после правок contracts/proto)
task proto:gen
task proto:check

# Локальный агент с тестовым TLS и токеном
task agent:run:test
```

В другом терминале — gRPC smoke (агент уже должен быть запущен):

```bash
task agent:test:smoke
```

Живые проверки с реальными данными TON: [agent/tests/grpc/README.md](agent/tests/grpc/README.md) · [RU](agent/tests/grpc/README.ru.md).

**TLS для продакшн-агентов:** [agent/README.md](agent/README.md) · [RU](agent/README.ru.md)

### VS Code

Пример `launch.json`:

```json
{
  "version": "0.2.0",
  "configurations": [
    {
      "name": "Coordinator",
      "type": "go",
      "request": "launch",
      "mode": "auto",
      "program": "${workspaceFolder}/coordinator/cmd",
      "buildFlags": "-tags=debug"
    },
    {
      "name": "Agent",
      "type": "go",
      "request": "launch",
      "mode": "auto",
      "program": "${workspaceFolder}/agent/cmd/agent"
    }
  ]
}
```

Переменные `env` задайте по `coordinator/deploy/.env.example` или по тестовым значениям из `Taskfile.yml` (`task agent:run:test`).

## Docker-деплой (VPS)

| Стек | Документация | Заметка |
|------|----------------|---------|
| Agent (+ опционально мониторинг) | [EN](agent/deploy/README.md) · [RU](agent/deploy/README.ru.md) | **VPS: лучше Docker Hub** — сборка образа локально, pull на сервере |
| Coordinator (+ Postgres + мониторинг) | [EN](coordinator/deploy/README.md) · [RU](coordinator/deploy/README.ru.md) | Образ coordinator **собирается на VPS** (`deploy:up`) |

**Агент на VPS (рекомендуется):** сборка и push на dev-машине, затем hub — см. [agent/deploy/README.ru.md](agent/deploy/README.ru.md).

```bash
# Dev-машина
AGENT_IMAGE=<user>/mytonprovider-agent:latest task agent:image:build:push
# VPS
task agent:hub:init && nano agent/deploy/.env.hub && task agent:hub:stack:up
```

**Coordinator — быстрый старт** (сборка на VPS):

```bash
task coordinator:deploy:init
nano coordinator/deploy/.env
task coordinator:deploy:up
```

**Агент со сборкой на VPS** (только dev / отладка):

```bash
task agent:deploy:init
nano agent/deploy/.env
task agent:deploy:up
```

## Локальная observability

Prometheus + Grafana + Loki для разработки: [EN](observability/prometheus-grafana/README.md) · [RU](observability/prometheus-grafana/README.ru.md).

## API и воркеры coordinator

REST (Fiber): телеметрия, список/фильтры провайдеров, метрики.

Фоновые воркеры:

- **Providers Master** — жизненный цикл провайдеров, health checks, gRPC к агентам
- **Telemetry Worker** — телеметрия от провайдеров
- **Cleaner Worker** — очистка устаревших данных в БД

## Лицензия

Apache-2.0 — см. [LICENSE](LICENSE).

Проект создан по заказу участника сообщества TON Foundation.
