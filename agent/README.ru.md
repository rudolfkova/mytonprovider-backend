# TLS для агента (без DNS)

**[English version](README.md)**

Сервис ожидает файлы TLS-сертификата и ключа при старте. Используйте свою CA (обычно на **coordinator**) и выпускайте server-сертификаты с **IP SAN**.

**Важно:** IP в SAN = адрес в `AGENT_ENDPOINTS` coordinator’а. Для связи через **Tailscale** указывайте Tailscale IPv4 (`100.x.x.x`), не обязательно публичный IP VPS.

После копирования на агент: `chmod 644` на `server.key` — см. [agent/deploy/secrets/README.ru.md](deploy/secrets/README.ru.md).

## 1) Генерация CA (один раз, на coordinator)

```bash
mkdir -p certs/ca
openssl genrsa -out certs/ca/ca.key 4096
openssl req -x509 -new -nodes -key certs/ca/ca.key -sha256 -days 3650 -out certs/ca/ca.crt -subj "/CN=mytonprovider-root-ca"
```

`certs/ca/ca.key` храните в секрете. Не коммитьте в git.

## 2) Server-сертификат для IP агента

Создайте OpenSSL config для каждого агента. Замените `203.0.113.10` на IP из `AGENT_ENDPOINTS` (публичный IP или Tailscale):

```bash
# Пример: выпуск на coordinator для агента DE (Tailscale)
issue_agent() {
  local name="$1" ip="$2"
  mkdir -p "certs/${name}"
  cat > "certs/${name}/openssl.cnf" <<EOF
[req]
distinguished_name = dn
req_extensions = v3_req
prompt = no
[dn]
CN = ${name}
[v3_req]
subjectAltName = @alt_names
extendedKeyUsage = serverAuth
keyUsage = digitalSignature,keyEncipherment
[alt_names]
IP.1 = ${ip}
EOF
  openssl genrsa -out "certs/${name}/server.key" 2048
  openssl req -new -key "certs/${name}/server.key" -out "certs/${name}/server.csr" -config "certs/${name}/openssl.cnf"
  openssl x509 -req -in "certs/${name}/server.csr" -CA certs/ca/ca.crt -CAkey certs/ca/ca.key -CAcreateserial \
    -out "certs/${name}/server.crt" -days 365 -sha256 -extensions v3_req -extfile "certs/${name}/openssl.cnf"
}
# issue_agent agent-de 100.98.253.40
```

Классический вариант с отдельным `.cnf` файлом — замените IP в `[alt_names]`:

```bash
cat > certs/agent-eu.cnf <<'EOF'
[req]
distinguished_name = dn
req_extensions = v3_req
prompt = no

[dn]
CN = agent-eu

[v3_req]
subjectAltName = @alt_names
extendedKeyUsage = serverAuth
keyUsage = digitalSignature,keyEncipherment

[alt_names]
IP.1 = 203.0.113.10
EOF
```

Ключ, CSR и подпись CA:

```bash
mkdir -p certs/agent-eu
openssl genrsa -out certs/agent-eu/server.key 2048
openssl req -new -key certs/agent-eu/server.key -out certs/agent-eu/server.csr -config certs/agent-eu.cnf
openssl x509 -req -in certs/agent-eu/server.csr -CA certs/ca/ca.crt -CAkey certs/ca/ca.key -CAcreateserial -out certs/agent-eu/server.crt -days 365 -sha256 -extensions v3_req -extfile certs/agent-eu.cnf
```

## 3) Env для контейнера агента

```bash
AGENT_LISTEN_ADDR=:8443
AGENT_ID=agent-eu
AGENT_LOCATION=eu
AGENT_AUTH_TOKEN=replace_with_long_random_token
AGENT_TLS_CERT_FILE=/run/secrets/server.crt
AGENT_TLS_KEY_FILE=/run/secrets/server.key
AGENT_MAX_CONCURRENT_PROVIDERS=30
# RunStorageRates / DHT + provider transport (не делите UDP-порт с coordinator на одном хосте)
AGENT_TON_CONFIG_URL=https://ton-blockchain.github.io/global.config.json
AGENT_ADNL_PORT=16167
# Опционально: 64 hex = 32-byte Ed25519 seed; иначе ключ генерируется при старте
# AGENT_ADNL_KEY=
# Опционально: лимит параллельных GetStorageRates; иначе AGENT_MAX_CONCURRENT_PROVIDERS
# AGENT_MAX_CONCURRENT_RATES=
# Опционально: после RunChecks — push сводок в Loki. Без trailing slash. Пример: http://127.0.0.1:3100
# AGENT_LOKI_URL=http://127.0.0.1:3100
# Опционально: Prometheus /metrics (HTTP). Пусто = выкл. Для scrape из Docker: 0.0.0.0:9090
# AGENT_METRICS_LISTEN_ADDR=0.0.0.0:9090
```

Скопируйте на агент в `agent/deploy/secrets/`, затем `chmod 644 server.crt server.key`. Файлы монтируются read-only в `/run/secrets/`.

## 4) gRPC health (`grpc.health.v1.Health`)

Агент регистрирует стандартный **gRPC Health Checking** на том же TLS-порту, что `RunChecks` / `RunStorageRates`.

- **`grpc.health.v1.Health/Check`** не требует заголовка coordinator `Authorization: Bearer`.
- После успешного старта имя сервиса `""` — **`SERVING`**; при shutdown — **`NOT_SERVING`** до закрытия TON transport.

Пример с [grpc_health_probe](https://github.com/grpc-ecosystem/grpc-health-probe):

```bash
grpc_health_probe -addr=127.0.0.1:8443 -tls -tls-no-verify
```

На gRPC-сервере включён **HTTP/2 keepalive** (пинги idle-клиентов ~раз в минуту), чтобы NAT/firewall реже рвал долгие соединения.

## 5) Доверие со стороны coordinator

gRPC-клиент coordinator должен доверять CA (`ca.crt`), которой подписаны server-сертификаты агентов.
Приватный ключ CA на серверы не копируйте.
