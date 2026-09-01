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

`task coordinator:deploy:up` runs **`docker compose ... --build`**: the coordinator image is **compiled on the VPS**. Expect extra disk for the Go builder image and build cache — heavier than the **Docker Hub** flow.

**Recommended for VPS:** pull a prebuilt image — see [Docker Hub](#docker-hub-recommended-for-vps) below.

## Docker Hub (recommended for VPS)

Build on a dev machine, on the VPS only `pull` + `compose up` — no `golang:bookworm` compile on the server.

| Step | Where | Command |
|------|-------|---------|
| 1 | Dev machine | `COORDINATOR_IMAGE=<user>/mytonprovider-coordinator:<tag> task coordinator:image:build:push` |
| 2 | VPS | Clone repo, `coordinator/deploy/` + secrets |
| 3 | VPS | `task coordinator:hub:init` → edit `coordinator/deploy/.env.hub` |
| 4 | VPS | `task coordinator:hub:up` |

Hub compose ([docker-compose.hub.yml](docker-compose.hub.yml)) uses `image: ${COORDINATOR_IMAGE}` with `pull_policy: always`.

### Dev machine: build + push

```bash
COORDINATOR_IMAGE=<docker-user>/mytonprovider-coordinator:latest task coordinator:image:build:push
```

### VPS: pull + up

```bash
task coordinator:hub:init
nano coordinator/deploy/.env.hub
task coordinator:hub:up
```

Set `COORDINATOR_IMAGE` in `.env.hub`; other vars match [`.env.example`](.env.example).

Stop: `task coordinator:hub:down`.

Frontend deploy is unchanged — [Frontend](#frontend-nginx).

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

### StoreProof timings

`StoreProof` anchors the full cycle from **start**: `nextFullAt = start + INTERVAL`. One retry of failed bags runs inside that window; when finished, the worker sleeps only the **remaining** time until `nextFullAt` (not a fresh hour after retry).

| Variable | Default | Meaning |
|----------|---------|---------|
| `COORDINATOR_STOREPROOF_INTERVAL_MINUTES` | `60` | Full cycle length from `StoreProof` start |
| `COORDINATOR_STOREPROOF_RETRY_DELAY_MINUTES` | `15` | Wait after the main pass before one retry. `0` disables retry (main-pass fails are committed immediately) |
| `COORDINATOR_RUNCHECKS_TOTAL_MS` | `1200000` (20m) | Per-agent budget for proof checks in one `RunChecks` |
| `AGENT_RPC_TIMEOUT_MS` | see `.env` | gRPC wait ceiling. Must be **≥** `RUNCHECKS_TOTAL_MS` and must not eat the retry window |

Hardcoded: retry must stop **5 minutes** before `nextFullAt` (buffer for the next full run).

**Retry window (worst case):**

```text
retryWindow ≈ INTERVAL - mainDuration - RETRY_DELAY - 5m
```

Example (INTERVAL=60, TotalMs≈20m, delay=15m):

```text
0 ........ 20m .............. 35m ................. 55m .... 60m
|--- main ---|---- delay ----|---- retry ≤20m ----|-buf-| next
```

If the main pass finishes earlier than 20m, delay starts earlier and the retry window is larger.

**Important:** if `AGENT_RPC_TIMEOUT_MS` is much larger than `COORDINATOR_RUNCHECKS_TOTAL_MS` (e.g. 50m vs 20m), a slow/hung RPC can consume the delay and retry window. Keep RPC timeout near TotalMs plus a small margin (e.g. 25m when TotalMs=20m).

DB write policy: successes and non-retriable errors are written immediately; transient fails are deferred until retry (or committed if retry is skipped / `RETRY_DELAY=0`). Pipeline events are recorded immediately for debugging.

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

## Frontend (separate deploy)

The catalog UI lives in [mytonprovider-frontend](https://github.com/mytonprovider/mytonprovider-frontend) and is deployed independently from coordinator.

**DNS:** point `api.mytonprovider.org` to the VPS (same host as coordinator). Coordinator listens on `${COORDINATOR_PORT:-8080}`.

**CORS:** in `.env.hub` set the frontend origin so the browser can call the API from another subdomain:

```env
CORS_ALLOWED_ORIGINS=https://mytonprovider.org
```

**Frontend image** (built in the frontend repo with `VITE_API_URL=https://api.mytonprovider.org/api/v1`):

```bash
task hub:init   # in mytonprovider-frontend
task hub:up
```

TLS for `mytonprovider.org` and `api.mytonprovider.org` is configured outside this stack (Cloudflare, certbot, etc.).

### Legacy frontend (nginx on host)

`task coordinator:deploy:frontend` still deploys the old Next.js app via host nginx + static build. Prefer the Docker Hub flow above for the new frontend.

## Frontend (nginx, legacy)

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
