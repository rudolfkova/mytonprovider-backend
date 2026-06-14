# Agent deploy secrets

**[Русская версия](README.ru.md)**

Place agent TLS files here before `task agent:hub:up`, `task agent:hub:stack:up`, or `task agent:deploy:up`.

## Files

- `server.crt` — server certificate (signed by the coordinator’s CA)
- `server.key` — private key

Mounted in the container as `/run/secrets/server.crt` and `/run/secrets/server.key`.

## File permissions (required)

The agent image runs as user **`app`**. A root-owned `server.key` with mode `600` causes:

`create TLS credentials: open /run/secrets/server.key: permission denied`

and the container restart-loops without listening on `8443`.

After copying certs:

```bash
chmod 644 agent/deploy/secrets/server.crt agent/deploy/secrets/server.key
```

Verify:

```bash
docker exec -u app deploy-agent-1 test -r /run/secrets/server.key && echo OK
```

(Container name may be `agent-1` with `docker-compose.hub.yml` — check `docker ps`.)

## Certificate SAN

The IP in `subjectAltName` must match what the coordinator uses in `AGENT_ENDPOINTS` (often a **Tailscale** `100.x.x.x` address).

## Migrating to a new coordinator

1. Copy new `server.crt` / `server.key` from the coordinator host (new CA).
2. `chmod 644` both files.
3. Set `AGENT_AUTH_TOKEN` in `.env.hub` to match the new coordinator.
4. Recreate the agent container from `agent/deploy/`.

Success: logs show `agent gRPC server started`, then `RunChecks completed` from the coordinator.
