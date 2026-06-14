# Coordinator VPS test deploy

**[Русская версия](README.ru.md)**

This stack runs:
- `coordinator`
- `postgres` (schema init from `coordinator/db/init.sql`)
- `prometheus`
- `grafana`

## Production deploy checklist (order)

1. VPS: Docker + Compose + Task + git; clone repo, checkout the branch you need.
2. **TLS:** CA and per-agent certs — [agent/README.md](../../agent/README.md); on coordinator: `secrets/agents-ca.crt`, `certs/agent-*/` for issuance.
3. `task coordinator:deploy:init` → edit `coordinator/deploy/.env` (see below).
4. `task coordinator:deploy:up` (compose publishes UDP `16167` for ADNL/DHT).
5. Smoke: `/health`, `/metrics` with Bearer (see [secrets/README.md](secrets/README.md)).
6. Optional: Tailscale on coordinator and agents; set `AGENT_ENDPOINTS` to agent Tailscale IPs.
7. On each agent: deploy new `server.crt`/`server.key`, same `AGENT_AUTH_TOKEN`, `chmod 644` on the key — [agent/deploy/secrets/README.md](../../agent/deploy/secrets/README.md).
8. Optional: `ufw` (do not expose Postgres `5432` publicly).
9. Optional: frontend — [Frontend](#frontend-nginx).

## Disk usage on VPS

`task coordinator:deploy:up` runs **`docker compose ... --build`**: the coordinator image is **compiled on the VPS**. Expect extra disk for the Go builder image and build cache — heavier than the agent **Docker Hub** flow.

For **agents** on other VPS nodes: [agent/deploy/README.md](../../agent/deploy/README.md).

**Advanced:** build and push the coordinator image on a dev machine, then on the VPS use `image:` instead of `build:` in compose.

## 1) Prepare

From repository root:

```bash
task coordinator:deploy:init
nano coordinator/deploy/.env
```

### Required `.env` fields

| Variable | Notes |
|----------|--------|
| `DB_PASSWORD` | Strong Postgres password |
| `SYSTEM_ACCESS_TOKENS` | CSV of **raw** tokens (see secrets); first token = metrics |
| `AGENT_AUTH_TOKEN` | Shared secret with every agent (`AGENT_AUTH_TOKEN` in `.env.hub`) |
| `AGENT_ENDPOINTS` | CSV `host:8443` — IP must match agent cert **SAN** |
| `AGENT_CA_CERT_FILE` | `/run/secrets/agents-ca.crt` |
| `TON_CONFIG_URL` | Usually `https://ton-blockchain.github.io/global.config.json` |

Until agents exist: `AGENT_ENDPOINTS=` (empty). After agents join, use Tailscale or public IP consistent with cert SAN.

### Secrets

See [secrets/README.md](secrets/README.md). Issue certs on the coordinator host; full steps in [agent/README.md](../../agent/README.md).

When **migrating to a new coordinator** with a new CA: re-issue all agent certs and replace `agents-ca.crt`. Old test-coordinator certs will not work.

## 2) Start

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

## 4) Logs / stop

```bash
task coordinator:deploy:logs
task coordinator:deploy:down
```

## Tailscale (coordinator ↔ agents)

Put coordinator and agents in the same tailnet. Use agent **Tailscale IPv4** in `AGENT_ENDPOINTS` (`100.x.x.x:8443`) and the same IP in each agent cert SAN.

## Frontend (nginx)

**By public IP (no Let's Encrypt):**

```bash
DOMAIN=<vps-public-ip> \
PUBLIC_ORIGIN=http://<vps-public-ip> \
INSTALL_SSL=false \
task coordinator:deploy:frontend
```

**By domain (SSL):** set `INSTALL_SSL=true` and `PUBLIC_ORIGIN=https://<domain>`.

## Firewall (example ufw)

Allow SSH, coordinator/frontend ports, UDP `16167` for ADNL. Do not expose Postgres `5432` to the internet unless required.
