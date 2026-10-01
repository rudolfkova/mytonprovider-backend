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
go mod edit -dropreplace=github.com/rudolfkova/mytonprovider-backend/contracts
GOPRIVATE=github.com/rudolfkova/* go get github.com/rudolfkova/mytonprovider-backend/contracts@v0.2.1
GOPRIVATE=github.com/rudolfkova/* go mod tidy
go mod edit -replace=github.com/rudolfkova/mytonprovider-backend/contracts=../mytonprovider-backend/contracts
```

Or run `task mod:sum:contracts` from mytonstorage-backend.

**Note:** use `@v0.2.1`, not `@contracts/v0.2.1`. After restoring `replace`, do not run `go mod tidy` — it removes contract checksums from `go.sum` and breaks Docker builds.

Local development uses `replace` in `go.mod` pointing to sibling `../mytonprovider-backend/contracts`.
