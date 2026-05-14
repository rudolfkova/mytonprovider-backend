Place coordinator secret files here before `task coordinator:deploy:up`.

Required:
- `agents-ca.crt` - CA certificate used by coordinator to trust agent TLS certs.
- `metrics.token` - raw token string used by Prometheus bearer_token_file.

Notes:
- `metrics.token` value must be present in `SYSTEM_ACCESS_TOKENS` inside `.env`.
- File must contain token text without extra JSON/YAML formatting.
