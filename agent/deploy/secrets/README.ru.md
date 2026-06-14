# Секреты для деплоя агента

**[English version](README.md)**

Положите TLS-файлы агента сюда перед `task agent:hub:up` / `task agent:hub:stack:up` / `task agent:deploy:up`.

## Файлы

- `server.crt` — server-сертификат (подписан CA coordinator’а)
- `server.key` — приватный ключ

Монтируются в контейнер как `/run/secrets/server.crt` и `/run/secrets/server.key` (см. `AGENT_TLS_*` в `.env.hub`).

## Права на файлы (обязательно)

Образ агента работает от пользователя **`app`** (не root). Если `server.key` с правами `600` и владельцем root, в логах будет:

`create TLS credentials: open /run/secrets/server.key: permission denied`

и контейнер уйдёт в restart loop, порт `8443` не слушается.

После копирования сертификатов:

```bash
chmod 644 agent/deploy/secrets/server.crt agent/deploy/secrets/server.key
```

Проверка:

```bash
docker exec -u app deploy-agent-1 test -r /run/secrets/server.key && echo OK
```

(имя контейнера может быть `agent-1` при `docker-compose.hub.yml` без stack — см. `docker ps`.)

## SAN в сертификате

IP в `subjectAltName` должен совпадать с адресом, который coordinator указывает в `AGENT_ENDPOINTS` (часто **Tailscale** `100.x.x.x`, не публичный IP VPS).

```bash
openssl x509 -in agent/deploy/secrets/server.crt -noout -ext subjectAltName
```

## Переход на новый coordinator

1. Получить с coordinator новую пару `server.crt` / `server.key` (новая CA).
2. Положить в этот каталог, `chmod 644`.
3. Обновить `AGENT_AUTH_TOKEN` в `.env.hub` (как на новом coordinator).
4. Перезапустить агент из `agent/deploy/`:

```bash
docker compose -f docker-compose.hub.stack.yml --env-file .env.hub up -d --force-recreate agent
```

Успех: в логах `agent gRPC server started`, затем `RunChecks completed` / `RunStorageRates completed` от coordinator.
