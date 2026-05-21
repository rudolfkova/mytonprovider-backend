# Секреты для деплоя coordinator

**[English version](README.md)**

Положите секреты coordinator сюда перед `task coordinator:deploy:up`.

Нужны:
- `agents-ca.crt` — CA для проверки TLS сертификатов агентов.
- `metrics.token` — строка токена для Prometheus `bearer_token_file`.

Заметки:
- значение `metrics.token` должно быть в `SYSTEM_ACCESS_TOKENS` в `.env`;
- в файле только текст токена, без JSON/YAML-обёртки.
