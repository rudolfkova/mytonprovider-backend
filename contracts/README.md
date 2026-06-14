# mytonprovider-contracts

Shared gRPC/protobuf definitions for mytonprovider agent and clients.

## Generate

From repo root:

```bash
task proto:gen
```

## Release tag (for mytonstorage-backend go.mod)

After proto changes:

```bash
git tag contracts/v0.2.0
git push origin contracts/v0.2.0
```

In `mytonstorage-backend/go.mod` (production, without local replace):

```bash
go get mytonprovider-contracts@contracts/v0.2.0
```

Local development uses `replace mytonprovider-contracts => ../mytonprovider-backend/contracts`.
