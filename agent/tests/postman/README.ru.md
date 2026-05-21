# Postman gRPC-тесты

**[English version](README.md)**

Файлы:

- `agent-grpc.postman_collection.json` — smoke и негативные тесты `RunChecks`.
- `agent-local.postman_environment.json` — переменные для одного локального агента.

## Запуск в Postman UI

1. Импортируйте оба JSON.
2. Задайте `agent_auth_token` в environment.
3. Откройте запрос `RunChecks - valid token and payload`.
4. Подключите proto `contracts/proto/providerchecks/v1/provider_checks.proto` в gRPC UI Postman.
5. Настройте доверие к CA, если агент на self-signed/private CA.
6. Запустите всю коллекцию.

## CLI из корня репозитория

```bash
task postman:run
```

### Заметки

- Task использует Postman CLI (`postman`), пишет вывод и JSON-отчёт.
- Отчёт: `agent/tests/postman/reports/postman-report.json` (gitignore).
- Для своей CA: `POSTMAN_CA_CERT`:

```bash
POSTMAN_CA_CERT=/absolute/path/to/ca.crt task postman:run
```
