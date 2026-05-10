# Postman gRPC tests

Files:

- `agent-grpc.postman_collection.json` - smoke and negative tests for `RunChecks`.
- `agent-local.postman_environment.json` - local variables for running against one agent.

## Manual run in Postman UI

1. Import both JSON files.
2. Set `agent_auth_token` in imported environment.
3. Open request `RunChecks - valid token and payload`.
4. Attach proto file `contracts/proto/providerchecks/v1/provider_checks.proto` in Postman gRPC UI.
5. Configure trust for your CA cert if agent uses self-signed/private CA.
6. Run all requests in collection.

## CLI run from repository root

```bash
task postman:run
```

### Notes

- This task uses Postman CLI (`postman`) and writes detailed CLI output plus a JSON report.
- Export location: `agent/tests/postman/reports/postman-report.json`.
- Set `POSTMAN_CA_CERT` env var when you need to trust custom CA cert:

```bash
POSTMAN_CA_CERT=/absolute/path/to/ca.crt task postman:run
```
