# FlammeBible

Сервер синхронизации песенников и клиент (CLI + веб) для работы вместе с уже установленной VisioBible. Сама программа VisioBible, её модули и чужие сборники песен в этот репозиторий не входят.

Стек: Go (API и CLI), PostgreSQL, Vue 3 + Pinia + Vite (сайт).

## Состав репозитория

```
cmd/server          HTTP API
cmd/cli             клиент синхронизации (командная строка)
internal/server     обработчики, миграции, схема PostgreSQL
internal/vbm        разбор и запись файлов .vbm
web                 сайт: вход, пространства, приглашения, состояние синхронизации
plans               архитектура, дорожная карта и задачи
rules.md            правила сопровождения кода
.env.example        имена переменных окружения, без секретов
```

## Требования

- Go 1.27 (см. `go.mod`)
- PostgreSQL **16.3** (версия зафиксирована тестом миграции)
- Node.js 20+ и npm — для сайта
- openssl — чтобы один раз сделать локальный сертификат

## Локальный запуск

1. Создайте базу и пользователя PostgreSQL 16.3, затем задайте `DATABASE_URL`.

2. Самоподписанный сертификат для локальной разработки (из корня репозитория):

```bash
openssl req -x509 -newkey rsa:2048 -keyout key.pem -out cert.pem -days 365 -nodes -subj "/CN=127.0.0.1"
```

`cert.pem` и `key.pem` в git не попадают.

3. API (терминал 1):

```bash
export DATABASE_URL='postgres://USER:PASSWORD@127.0.0.1:5432/flammebible?sslmode=disable'
export TLS_CERT=cert.pem
export TLS_KEY=key.pem
# необязательно: ADDR=:8080
go run ./cmd/server
```

Сервер слушает HTTPS. Без `DATABASE_URL`, `TLS_CERT` и `TLS_KEY` он не стартует. При запуске применяются миграции из `internal/server/schema.sql`.

4. Сайт (терминал 2):

```bash
cd web
npm install
npm run dev
```

Откройте http://localhost:5173. Vite проксирует запросы API на `https://127.0.0.1:8080` (или на `VITE_API_TARGET`). Для сборки: `npm run build`.

## CLI

```bash
go run ./cmd/cli login --server https://127.0.0.1:8080 --email you@example.com --password '...'
go run ./cmd/cli space <dimension-id>
go run ./cmd/cli sync --db local.db путь/к/копии/книги.vbm
go run ./cmd/cli resolve ...
go run ./cmd/cli build ...
go run ./cmd/cli install --root путь/к/копии/VisioBible
go run ./cmd/cli plan путь/к/plan.vplt
go run ./cmd/cli bible путь/к/копии/модуля
```

Для самоподписанного сертификата задайте `FLAMMEBIBLE_CA=cert.pem`. Сессия хранится в `~/.config/flammebible` (на Windows — `%AppData%\flammebible`), либо в каталоге `FLAMMEBIBLE_CONFIG`. Работать нужно только с копиями модулей VisioBible, не с оригиналами.

## Переменные окружения

| Имя | Кто использует | Назначение |
|---|---|---|
| `DATABASE_URL` | сервер, интеграционные тесты | строка подключения PostgreSQL |
| `TLS_CERT`, `TLS_KEY` | сервер | пути к сертификату и ключу |
| `ADDR` | сервер | адрес listen, по умолчанию `:8080` |
| `VITE_API_TARGET` | сайт (`npm run dev`) | куда проксировать API |
| `FLAMMEBIBLE_CA` | CLI | дополнительный CA |
| `FLAMMEBIBLE_CONFIG` | CLI | каталог настроек |

Образец без секретов: `.env.example`.

## Тесты

```bash
go test ./internal/vbm
go test ./cmd/cli -skip 'TestTwoCLIProcessesExchangeSong|TestConflictBothOutcomes|TestDeleteVersusEditDoesNotChoose|TestBuildPatchOnServer'
```

Четыре интеграционных теста CLI и тесты `./internal/server` требуют PostgreSQL 16.3 и `DATABASE_URL`. Тесты VBM и CLI не читают чужие песенники: синтетические `.vbm` собираются во временном каталоге.

## Что не входит в репозиторий

Программа VisioBible, архивы, сторонние `.vbm` / модули Библии и песенники, переписки и исследовательские дампы. Лицензия на их распространение не выяснена, и в репозитории с кодом им не место.

Лицензию самого проекта перед публикацией нужно указать отдельно: файл `LICENSE` в этом снимке не добавлен.
