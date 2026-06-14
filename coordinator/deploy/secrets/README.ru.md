# Секреты для деплоя coordinator

**[English version](README.md)**

Положите секреты coordinator сюда перед `task coordinator:deploy:up`.

## Файлы

| Файл | Назначение |
|------|------------|
| `agents-ca.crt` | Публичный CA, которым подписаны **server-сертификаты агентов**. Coordinator проверяет TLS при gRPC к агентам. |
| `metrics.token` | **Сырой** токен (одна строка, без `\n` в конце по возможности). Prometheus шлёт его в `Authorization: Bearer` при scrape `/metrics`. |

## `SYSTEM_ACCESS_TOKENS` в `.env`

- Формат: `token1,token2` — **обычные строки-токены**, не MD5 и не base64.
- Первый токен должен совпадать с содержимым `metrics.token`.
- Второй (и далее) — для ручных вызовов API (`Authorization: Bearer`).

Проверка после старта coordinator:

```bash
RAW=$(tr -d '\n' < coordinator/deploy/secrets/metrics.token)
curl -fsS -H "Authorization: Bearer ${RAW}" http://127.0.0.1:8080/metrics | head -1
```

Если в `.env` когда-то вручную подставляли MD5-хеши вместо сырых токенов — верните **сырые** значения и перезапустите coordinator.

## Связь с агентами

- `AGENT_AUTH_TOKEN` в `.env` coordinator = `AGENT_AUTH_TOKEN` в `agent/deploy/.env.hub` на **каждом** агенте (отдельно от metrics/API токенов).
- `agents-ca.crt` **не** копируется на агенты. На агенты идут только `server.crt` + `server.key` (подписаны этой CA).

Выпуск CA и сертификатов: [agent/README.ru.md](../../../agent/README.ru.md).
