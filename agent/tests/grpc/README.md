# gRPC smoke tests

These smoke checks run without coordinator and validate agent behavior directly via `grpcurl`.

## Commands

From repository root:

```bash
task agent:run:test
```

Runs agent with local test config and local certs.

```bash
task agent:test:smoke
```

Starts agent in background, runs 3 checks via `grpcurl`, writes logs and outputs into:

- `agent/tests/grpc/reports/agent.log`
- `agent/tests/grpc/reports/runchecks-valid.out`
- `agent/tests/grpc/reports/runchecks-invalid-token.out`
- `agent/tests/grpc/reports/runchecks-invalid-payload.out`

## Prerequisites

- `grpcurl` installed and available in `PATH`
- `openssl` installed
