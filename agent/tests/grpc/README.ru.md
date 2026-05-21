# gRPC smoke-тесты

**[English version](README.md)**

Smoke-проверки без coordinator: поведение агента напрямую через `grpcurl`.

## Команды

Из корня репозитория:

```bash
task agent:run:test
```

Запуск агента с локальным тестовым конфигом и сертификатами.

```bash
task agent:test:smoke
```

Нужен уже запущенный тестовый агент (`task agent:run:test` в другом терминале). Три проверки `grpcurl`; опционально отчёты в `agent/tests/grpc/reports/` (в gitignore): `runchecks-valid.out`, `runchecks-invalid-token.out`, `runchecks-invalid-payload.out`.

## Живой запрос (real flow)

Один запрос, сгенерированный логикой coordinator:

```bash
task agent:run:test
```

В другом терминале:

```bash
task agent:test:live
```

Цепочка:

1. `task coordinator:dump:runchecks` → `agent/tests/grpc/runchecks-live.json`
2. Отправка JSON в `RunChecks` через `grpcurl`

По умолчанию `dump-runchecks` с **`--storage=memory`**: in-process fake DB (без Postgres). Нужна сеть до TON (и `MASTER_ADDRESS` / `TON_CONFIG_URL`, если дефолтов мало):

```bash
export MASTER_ADDRESS=<your-master-wallet>
task coordinator:dump:runchecks
```

Чтение из Postgres:

```bash
go run ./coordinator/cmd/dump-runchecks --storage=postgres --limit 1 --out agent/tests/grpc/runchecks-live.json
```

(`DB_*` как у coordinator).

## RunStorageRates (provider GetStorageRates)

Агент слушает UDP **ADNL** для DHT и transport (порт по умолчанию `16167`, `AGENT_ADNL_PORT`; в тестовом task — `36167`, чтобы не конфликтовать с локальным coordinator).

### Dump + live test (как RunChecks)

`dump-runchecks` может выдать второй JSON рядом с RunChecks:

1. `task coordinator:dump:runchecks:with-storage-rates` — `runchecks-live.json` и **`runchecks-live-storage-rates.json`** (pubkeys из RunChecks payload, тот же порядок).
2. При запущенном агенте (`task agent:run:test`) — **`task agent:test:live:storage-rates`**; ответ в `agent/tests/grpc/reports/runchecks-storage-rates-last.json`.
3. **`task agent:test:live:storage-rates:replay`** — только replay существующего `runchecks-live-storage-rates.json`.

Вручную (шаг 1):

```bash
go run ./coordinator/cmd/dump-runchecks --limit 50 --out agent/tests/grpc/runchecks-live.json --also-dump-storage-rates
```

Флаги: `--storage-rates-out`, `--rates-job-id`, `--rates-query-timeout-ms`, `--rates-total-ms`, `--rates-query-size` (см. `go run ./coordinator/cmd/dump-runchecks -help`).

Пример `grpcurl` (замените `PROVIDER_PUBKEY_HEX64` на 64 hex символа):

```bash
grpcurl -insecure \
  -import-path contracts/proto \
  -proto providerchecks/v1/provider_checks.proto \
  -H "authorization: Bearer test-token" \
  -d '{"jobId":"rates-smoke-1","providerPubkeys":["PROVIDER_PUBKEY_HEX64"],"querySize":1,"timeouts":{"queryTimeoutMs":14000,"totalMs":60000}}' \
  127.0.0.1:8443 \
  providerchecks.v1.ProviderChecksService/RunStorageRates
```

## Требования

- `grpcurl` в `PATH`
- `openssl`
- для live dump: исходящая сеть до TON lite / config URL
- для `--storage=postgres`: Postgres со схемой coordinator
