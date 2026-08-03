module mytonprovider-coordinator

go 1.26.2

require (
	github.com/gofiber/fiber/v2 v2.52.14
	github.com/jackc/pgx/v5 v5.10.0
	github.com/rudolfkova/mytonprovider-backend/contracts v0.0.0
	github.com/xssnick/tonutils-go v1.18.0
	github.com/xssnick/tonutils-storage-provider v0.4.3
	google.golang.org/grpc v1.83.0
)

replace github.com/rudolfkova/mytonprovider-backend/contracts => ../contracts

require (
	filippo.io/edwards25519 v1.2.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/philhofer/fwd v1.1.3-0.20240916144458-20a13a1f6b7c // indirect
	github.com/pierrec/lz4/v4 v4.1.27 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.70.1 // indirect
	github.com/prometheus/procfs v0.21.1 // indirect
	github.com/tinylib/msgp v1.3.0 // indirect
	github.com/xssnick/raptorq v1.5.0 // indirect
	golang.org/x/net v0.57.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/text v0.40.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260526163538-3dc84a4a5aaa // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

require (
	github.com/andybalholm/brotli v1.1.1 // indirect
	github.com/caarlos0/env/v11 v11.4.1
	github.com/gofiber/adaptor/v2 v2.2.1
	github.com/google/uuid v1.6.0 // indirect
	github.com/klauspost/compress v1.19.1 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-runewidth v0.0.23 // indirect
	github.com/prometheus/client_golang v1.24.1
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasthttp v1.62.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
)
