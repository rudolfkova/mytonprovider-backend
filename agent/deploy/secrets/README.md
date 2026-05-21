# Agent deploy secrets

**[Русская версия](README.ru.md)**

Place agent TLS files here before `task agent:deploy:up` or `task agent:hub:up`.

Required files:
- `server.crt`
- `server.key`

Both are mounted into the container as read-only `/run/secrets/*`.
