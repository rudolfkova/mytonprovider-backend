# Coordinator deploy secrets

**[Русская версия](README.ru.md)**

Place coordinator secret files here before `task coordinator:deploy:up`.

## Files

| File | Purpose |
|------|---------|
| `agents-ca.crt` | Public CA that signed **agent server** certificates. Coordinator uses it to verify agent TLS on gRPC. |
| `metrics.token` | **Raw** token string (single line). Prometheus sends it as `Authorization: Bearer` when scraping `/metrics`. |

## `SYSTEM_ACCESS_TOKENS` in `.env`

- Format: `token1,token2` — **plain token strings**, not MD5 hashes.
- First token must match `metrics.token`.
- Additional tokens are for manual API calls (`Authorization: Bearer`).

After coordinator start:

```bash
RAW=$(tr -d '\n' < coordinator/deploy/secrets/metrics.token)
curl -fsS -H "Authorization: Bearer ${RAW}" http://127.0.0.1:8080/metrics | head -1
```

If `.env` previously contained MD5 hex instead of raw tokens, restore **raw** values and restart coordinator.

## Agents

- `AGENT_AUTH_TOKEN` in coordinator `.env` must match `AGENT_AUTH_TOKEN` in each `agent/deploy/.env.hub`.
- Do not copy `agents-ca.crt` to agents — only per-agent `server.crt` / `server.key`.

Cert issuance: [agent/README.md](../../../agent/README.md).
