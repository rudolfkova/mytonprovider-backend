# Секреты для деплоя агента

**[English version](README.md)**

Положите TLS-файлы агента сюда перед `task agent:deploy:up` или `task agent:hub:up`.

Нужны:
- `server.crt`
- `server.key`

Монтируются в контейнер read-only как `/run/secrets/*`.
