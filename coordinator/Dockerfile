FROM golang:1.26-bookworm AS builder

WORKDIR /src

COPY go.work ./go.work
COPY agent/go.mod agent/go.sum ./agent/
COPY coordinator/go.mod coordinator/go.sum ./coordinator/
COPY contracts/go.mod contracts/go.sum ./contracts/

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go work sync && \
    cd coordinator && go mod download

COPY agent ./agent
COPY coordinator ./coordinator
COPY contracts ./contracts

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/coordinator ./coordinator/cmd

FROM debian:bookworm-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates && \
    rm -rf /var/lib/apt/lists/* && \
    groupadd --system app && \
    useradd --system --gid app --create-home --home-dir /home/app app

COPY --from=builder /out/coordinator /usr/local/bin/coordinator

USER app:app

ENTRYPOINT ["/usr/local/bin/coordinator"]
