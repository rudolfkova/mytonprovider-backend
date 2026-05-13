# Agent TLS Setup (No DNS)

This service expects TLS cert and key files at startup. For internet-facing VPS nodes,
use your own CA and issue per-agent server certificates with IP SAN entries.

## 1) Generate CA (once, offline machine)

```bash
mkdir -p certs/ca
openssl genrsa -out certs/ca/ca.key 4096
openssl req -x509 -new -nodes -key certs/ca/ca.key -sha256 -days 3650 -out certs/ca/ca.crt -subj "/CN=mytonprovider-root-ca"
```

Keep `certs/ca/ca.key` private. Never commit it to git.

## 2) Issue server cert for agent IP

Create OpenSSL config for each agent, replacing `203.0.113.10` with the real public IP:

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

Generate key/csr and sign with your CA:

```bash
mkdir -p certs/agent-eu
openssl genrsa -out certs/agent-eu/server.key 2048
openssl req -new -key certs/agent-eu/server.key -out certs/agent-eu/server.csr -config certs/agent-eu.cnf
openssl x509 -req -in certs/agent-eu/server.csr -CA certs/ca/ca.crt -CAkey certs/ca/ca.key -CAcreateserial -out certs/agent-eu/server.crt -days 365 -sha256 -extensions v3_req -extfile certs/agent-eu.cnf
```

## 3) Runtime env for agent container

```bash
AGENT_LISTEN_ADDR=:8443
AGENT_ID=agent-eu
AGENT_LOCATION=eu
AGENT_AUTH_TOKEN=replace_with_long_random_token
AGENT_TLS_CERT_FILE=/run/secrets/server.crt
AGENT_TLS_KEY_FILE=/run/secrets/server.key
AGENT_MAX_CONCURRENT_PROVIDERS=30
# RunStorageRates / DHT + provider transport (do not share UDP port with coordinator on same host)
AGENT_TON_CONFIG_URL=https://ton-blockchain.github.io/global.config.json
AGENT_ADNL_PORT=16167
# Optional: 64 hex chars = 32-byte Ed25519 seed; if unset, a random key is generated at startup
# AGENT_ADNL_KEY=
# Optional: parallel GetStorageRates cap; if unset, uses AGENT_MAX_CONCURRENT_PROVIDERS
# AGENT_MAX_CONCURRENT_RATES=
```

Mount `server.crt` and `server.key` read-only into the container.

## 4) gRPC health (`grpc.health.v1.Health`)

The agent registers the standard **gRPC Health Checking** service on the same TLS port as `RunChecks` / `RunStorageRates`.

- **`grpc.health.v1.Health/Check`** does **not** require the coordinator `Authorization: Bearer` header (so probes can stay tokenless).
- After successful startup the overall service name `""` is **`SERVING`**; on process shutdown cleanup sets **`NOT_SERVING`** before closing TON transport.

Example with [grpc_health_probe](https://github.com/grpc-ecosystem/grpc-health-probe) against a test agent (adjust CA / `-tls-no-verify` for your setup):

```bash
grpc_health_probe -addr=127.0.0.1:8443 -tls -tls-no-verify
```

## 5) Coordinator trust

Coordinator gRPC client must trust the CA cert (`ca.crt`) that signed agent server certs.
Do not copy CA private key to any server.
