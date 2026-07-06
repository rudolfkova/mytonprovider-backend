# Миграции БД coordinator

**[English version](README.md)**

## Pipeline events (001)

### Прод: порядок выката

1. Выполнить SQL **до** деплоя новой версии coordinator:

```bash
psql -h <host> -U pguser -d providerdb -f coordinator/db/migrations/001_pipeline_events.up.sql
```

2. Проверить таблицы:

```sql
\dt providers.*pipeline*
```

3. Deploy coordinator (код только INSERT; миграции при старте не запускаются).

4. После следующего StoreProof (~60 мин):

```sql
SELECT * FROM providers.bag_pipeline_events ORDER BY created_at DESC LIMIT 5;
SELECT * FROM providers.provider_pipeline_events ORDER BY created_at DESC LIMIT 5;
```

### Откат SQL

```bash
psql ... -f coordinator/db/migrations/001_pipeline_events.down.sql
```

Откат кода coordinator без down.sql: запись событий прекращается, таблицы можно оставить.

### Примеры запросов для triage

**История провайдера:**

```sql
SELECT created_at, status, stage, reason_code, error_message, run_id
FROM providers.provider_pipeline_events
WHERE provider_pubkey = '<hex_pubkey>'
ORDER BY created_at;
```

**Последнее состояние каждого bag провайдера:**

```sql
SELECT DISTINCT ON (contract_address)
    bag_id, contract_address, status, stage, reason_code, error_message, created_at
FROM providers.bag_pipeline_events
WHERE provider_pubkey = '<hex_pubkey>'
ORDER BY contract_address, created_at DESC;
```

**Bags с активной ошибкой:**

```sql
SELECT DISTINCT ON (contract_address)
    bag_id, contract_address, stage, reason_code, error_message, created_at
FROM providers.bag_pipeline_events
WHERE provider_pubkey = '<hex_pubkey>'
ORDER BY contract_address, created_at DESC
HAVING status = 'error';
-- или обернуть в подзапрос:
SELECT * FROM (
    SELECT DISTINCT ON (contract_address) *
    FROM providers.bag_pipeline_events
    WHERE provider_pubkey = '<hex_pubkey>'
    ORDER BY contract_address, created_at DESC
) t WHERE t.status = 'error';
```

**Сводка: всего bags vs с активной ошибкой:**

```sql
SELECT
    COUNT(*) AS total_bags,
    (
        SELECT COUNT(*) FROM (
            SELECT DISTINCT ON (bpe.contract_address) bpe.status
            FROM providers.bag_pipeline_events bpe
            WHERE bpe.provider_pubkey = p.public_key
            ORDER BY bpe.contract_address, bpe.created_at DESC
        ) latest WHERE latest.status = 'error'
    ) AS bags_with_active_error
FROM providers.storage_contracts sc
JOIN providers.providers p ON p.address = sc.provider_address
WHERE p.public_key = '<hex_pubkey>';
```

Retention: cleaner удаляет события старше `SYSTEM_STORE_HISTORY_DAYS` (default 90).
