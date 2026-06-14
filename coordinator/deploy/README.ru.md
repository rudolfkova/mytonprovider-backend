# Тестовый деплой coordinator на VPS

**[English version](README.md)**

Стек:
- `coordinator`
- `postgres` (инициализация схемы из `coordinator/db/init.sql`)
- `prometheus`
- `grafana`

## Чеклист продакшн-деплоя (порядок)

1. VPS: Docker + Compose + Task + git; клон репо, нужная ветка (`git checkout <branch>`).
2. **TLS:** CA и сертификаты агентов — [agent/README.ru.md](../../agent/README.ru.md); на coordinator: `secrets/agents-ca.crt`, каталог `certs/agent-*/` для выдачи.
3. `task coordinator:deploy:init` → правка `coordinator/deploy/.env` (см. ниже).
4. `task coordinator:deploy:up` (в compose уже проброшен UDP `16167` для ADNL/DHT).
5. Smoke: `/health`, `/metrics` с Bearer (см. [secrets/README.ru.md](secrets/README.ru.md)).
6. Опционально: Tailscale на coordinator и агенты; `AGENT_ENDPOINTS` = Tailscale IP агентов.
7. На каждом агенте: новые `server.crt`/`server.key`, тот же `AGENT_AUTH_TOKEN`, `chmod 644` на ключ — [agent/deploy/secrets/README.ru.md](../../agent/deploy/secrets/README.ru.md).
8. Опционально: `ufw` (не открывать `5432` наружу).
9. Опционально: фронт — [§ Фронт](#фронт-nginx).

## Место на диске (VPS)

`task coordinator:deploy:up` запускает **`docker compose ... --build`**: образ coordinator **собирается на VPS** (см. `build:` у сервиса `coordinator` в [docker-compose.yml](docker-compose.yml)). Ожидайте расход диска на образ Go builder и кэш сборки — это тяжелее, чем hub-режим **агента**.

Для **агентов** на других VPS: [agent/deploy/README.ru.md](../../agent/deploy/README.ru.md) (образ из Docker Hub, без сборки на VPS агента).

**Продвинутый вариант:** соберите и запушьте образ coordinator на dev-машине, на VPS в compose замените `build:` на `image:`:

```bash
docker build -f coordinator/Dockerfile -t <user>/mytonprovider-coordinator:latest .
docker push <user>/mytonprovider-coordinator:latest
```

## 1) Подготовка

Из корня репозитория:

```bash
task coordinator:deploy:init
nano coordinator/deploy/.env
```

### Обязательно в `.env`

| Переменная | Заметка |
|------------|---------|
| `DB_PASSWORD` | Сильный пароль Postgres |
| `SYSTEM_ACCESS_TOKENS` | CSV **сырых** токенов (см. secrets); первый = metrics |
| `AGENT_AUTH_TOKEN` | Общий секрет с **каждым** агентом (`AGENT_AUTH_TOKEN` в `.env.hub`) |
| `AGENT_ENDPOINTS` | CSV `host:8443` — IP **должен совпадать с SAN** в `server.crt` агента |
| `AGENT_CA_CERT_FILE` | `/run/secrets/agents-ca.crt` |
| `TON_CONFIG_URL` | Обычно `https://ton-blockchain.github.io/global.config.json` |

Пока агентов нет: `AGENT_ENDPOINTS=` (пусто). После подключения агентов — Tailscale IP или публичный IP, тот же, что в SAN сертификата.

### Секреты

См. [secrets/README.ru.md](secrets/README.ru.md):
- `agents-ca.crt` — CA, которой подписаны сертификаты агентов
- `metrics.token` — **сырой** токен для Prometheus (дубликат первого токена из `SYSTEM_ACCESS_TOKENS`)

Выпуск сертификатов на coordinator (пример):

```bash
# CA один раз
mkdir -p certs/ca
openssl genrsa -out certs/ca/ca.key 4096
openssl req -x509 -new -nodes -key certs/ca/ca.key -sha256 -days 3650 \
  -out certs/ca/ca.crt -subj "/CN=mytonprovider-root-ca"
cp certs/ca/ca.crt coordinator/deploy/secrets/agents-ca.crt

# Сертификат агента: IP.1 = адрес из AGENT_ENDPOINTS (часто Tailscale 100.x.x.x)
# Подробно: agent/README.ru.md
```

При **смене coordinator** (новая CA): перевыпустите все `server.crt`/`server.key` агентов и обновите `agents-ca.crt`. Старые сертификаты от тестового coordinator не подойдут.

## 2) Запуск

```bash
task coordinator:deploy:up
```

## 3) Smoke checks

```bash
task coordinator:deploy:ps
curl -fsS "http://127.0.0.1:${COORDINATOR_PORT:-8080}/health"

METRICS=$(tr -d '\n' < coordinator/deploy/secrets/metrics.token)
curl -fsS -H "Authorization: Bearer ${METRICS}" \
  "http://127.0.0.1:${COORDINATOR_PORT:-8080}/metrics" | head -3
```

С Tailscale coordinator:

```bash
nc -zv <agent-tailscale-ip> 8443
```

В логах coordinator не должно быть постоянных `connection refused` / `all agents are unavailable` после того, как агенты подняты.

## 4) Логи / остановка

```bash
task coordinator:deploy:logs
task coordinator:deploy:down
```

## Tailscale (coordinator ↔ агенты)

Рекомендуется: coordinator и агенты в одной tailnet; в `AGENT_ENDPOINTS` — **Tailscale IPv4** агентов (`100.x.x.x:8443`), в SAN сертификата агента — **тот же** IP.

Проверка с coordinator:

```bash
tailscale ip -4
ping -c 1 <agent-ip>
nc -zv <agent-ip> 8443
```

Публичный IP агента в endpoints при сертификате только на Tailscale IP — TLS handshake не пройдёт.

## Фронт (nginx)

После запущенного coordinator, на **той же VPS**:

**По IP (без Let's Encrypt):**

```bash
DOMAIN=<vps-public-ip> \
PUBLIC_ORIGIN=http://<vps-public-ip> \
INSTALL_SSL=false \
COORDINATOR_PORT=8080 \
task coordinator:deploy:frontend
```

**По домену (SSL):**

```bash
DOMAIN=mytonprovider.org \
PUBLIC_ORIGIN=https://mytonprovider.org \
INSTALL_SSL=true \
task coordinator:deploy:frontend
```

Откройте в firewall `80` / `443` при необходимости.

## Firewall (пример ufw)

```bash
ufw allow OpenSSH
ufw allow 8080/tcp    # coordinator API (или только через nginx :80)
ufw allow 80/tcp      # frontend
ufw allow 443/tcp     # frontend + SSL
ufw allow 16167/udp   # coordinator ADNL
# Postgres 5432 наружу не открывать
ufw enable
```

Prometheus/Grafana (`9092`, `3001`) — только если нужны снаружи.
