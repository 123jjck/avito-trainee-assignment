# Сервис назначения ревьюеров для Pull Request’ов

Тестовое задание для стажировки Авито.

Спецификация API находится в `openapi.yml`.

## Запуск

```bash
docker compose up --build
```

Сервис поднимется на `http://localhost:8080`, PostgreSQL — на `localhost:5432` (пользователь/пароль/БД: `pr_service`).

Стоп:

```bash
docker compose down
```

Таблицы создаются автоматически при старте

## Эндпоинты

Реализовал все необходимые по заданию эндпоинты + доп задание: статистика (количество PR по статусам и сколько ревьюов у каждого пользователя) + `GET /health` для отладки.

для ошибочного тела запроса возвращается `BAD_REQUEST`.

## Принятые допущения

- При повторном создании команды возвращается `400 TEAM_EXISTS`; пользователи внутри запроса создаются или обновляются (имя, команда, флаг активности).
- При назначениях и переназначениях автор PR не может стать ревьювером.
- Переназначение проверяет, что заменяемый ревьювер действительно был назначен; если нет кандидатов в его команде — `NO_CANDIDATE`.
- При merge, если PR уже `MERGED`, отдаётся текущее состояние без ошибки.
- Для `/pullRequest/reassign` по схеме прописано поле `old_user_id`, но в примере запроса есть также и `old_reviewer_id` (реализовал поддержку обоих параметров)

## Примеры запросов

```bash
# Создать команду
curl -X POST http://localhost:8080/team/add \
  -H "Content-Type: application/json" \
  -d '{"team_name":"backend","members":[{"user_id":"u1","username":"Alice","is_active":true},{"user_id":"u2","username":"Bob","is_active":true}]}'

# Создать PR и автоматическое назначение ревьюеров
curl -X POST http://localhost:8080/pullRequest/create \
  -H "Content-Type: application/json" \
  -d '{"pull_request_id":"pr-1","pull_request_name":"Add search","author_id":"u1"}'

# Переназначить ревьювера
curl -X POST http://localhost:8080/pullRequest/reassign \
  -H "Content-Type: application/json" \
  -d '{"pull_request_id":"pr-1","old_user_id":"u2"}'
```
