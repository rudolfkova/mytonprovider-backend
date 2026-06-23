# mytonprovider-contracts

Shared gRPC/protobuf definitions for mytonprovider agent and clients.

Module path: `github.com/rudolfkova/mytonprovider-backend/contracts`

## Generate

From repo root:

```bash
task proto:gen
```

## Release tag (for mytonstorage-backend go.mod)

After proto changes:

```bash
git tag contracts/v0.2.1
git push origin contracts/v0.2.1
```

In `mytonstorage-backend/go.mod` (production):

```bash
go get github.com/rudolfkova/mytonprovider-backend/contracts@contracts/v0.2.1
```

Local development uses `replace` in `go.mod` pointing to sibling `../mytonprovider-backend/contracts`.
