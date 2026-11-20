# FlammeBible

Сервер синхронизации песенников и клиент (CLI + веб) для работы вместе с уже установленной VisioBible. Сама программа VisioBible, её модули и чужие сборники песен в этот репозиторий не входят.

Стек: Go (API и CLI), PostgreSQL, Vue 3 + Pinia + Vite (сайт).

Лицензия: MIT, см. `LICENSE`.

## Состав репозитория

```
cmd/server          HTTP API
cmd/cli             клиент синхронизации (командная строка)
internal/server     обработчики, миграции, схема PostgreSQL
internal/vbm        разбор и запись файлов .vbm
web                 сайт: вход, пространства, приглашения, состояние синхронизации
deploy/nginx        пример системного nginx с TLS
plans               архитектура, дорожная карта и задачи
rules.md            правила сопровождения кода
мое_представление.txt  исходная заметка о замысле, без правок
.env.example        имена переменных окружения, без секретов
Dockerfile          образ API
docker-compose.yml  API, сайт и PostgreSQL
```

## Требования

- Go 1.27 (см. `go.mod`)
- PostgreSQL 16 или новее (17 и следующие мажоры тоже подходят; патч не зафиксирован)
- Node.js 20+ и npm — для сайта
- Docker и Docker Compose — для запуска контейнерами
- openssl — только если нужен свой TLS на самом процессе Go

## Локальный запуск

1. Создайте базу PostgreSQL 16+ и задайте `DATABASE_URL`.

2. API (терминал 1). Без сертификатов сервер слушает HTTP — так его ставят за обратным прокси:

```bash
export DATABASE_URL='postgres://USER:PASSWORD@127.0.0.1:5432/flammebible?sslmode=disable'
# необязательно: ADDR=:8080
# необязательно: SESSION_TTL=720h
go run ./cmd/server
```

Свой TLS на процессе, без прокси:

```bash
openssl req -x509 -newkey rsa:2048 -keyout key.pem -out cert.pem -days 365 -nodes -subj "/CN=127.0.0.1"
export TLS_CERT=cert.pem
export TLS_KEY=key.pem
```

`cert.pem` и `key.pem` в git не попадают. `TLS_CERT` и `TLS_KEY` задаются только вместе. При запуске применяются миграции из `internal/server/schema.sql`. Нужен PostgreSQL с мажором не ниже 16.

3. Сайт (терминал 2):

```bash
cd web
npm install
npm run dev
```

Откройте http://localhost:5173. Vite проксирует запросы API, включая `/catalog`, на `http://127.0.0.1:8080` (или на `VITE_API_TARGET`). Для локального HTTPS API задайте `VITE_API_TARGET=https://127.0.0.1:8080`. Сборка: `npm run build`.

## Docker Compose

Из корня репозитория:

```bash
cp .env.example .env
# в .env замените POSTGRES_PASSWORD
docker compose up -d --build
```

Поднимаются три сервиса:

- `db` — PostgreSQL 16 (том `pgdata`). Тег образа можно сменить на `postgres:17-alpine` или новее.
- `api` — Go-сервер по HTTP внутри сети compose. Каталог медиафайлов — том `media`, путь в контейнере `/data/media` (`MEDIA_DIR`). Порт API наружу не публикуется.
- `web` — собранный сайт и nginx. Проксирует `/register`, `/login`, `/refresh`, `/logout`, `/dimensions`, `/invites`, `/songbooks`, `/catalog` на `api`. Сайт доступен только на `127.0.0.1:${FRONTEND_PORT:-8088}`.

Проверка конфигурации без запуска: `docker compose config`.

## Системный nginx и TLS

Публичный вход — nginx на хосте. Пример: `deploy/nginx/flammebible.conf.example`.

- `server_name bible.example.com`
- порт 80 отвечает редиректом 301 на HTTPS
- порт 443, без `default_server`: на хосте могут быть другие сайты
- сертификат — уже лежащий wildcard: в примере пути `/etc/ssl/example/wildcard.crt` и `/etc/ssl/example/wildcard.key`. Отдельный выпуск Let's Encrypt для имени сайта не нужен, если wildcard уже установлен. Подставьте свой каталог сертификата.
- `include /etc/letsencrypt/options-ssl-nginx.conf` и `ssl_dhparam /etc/letsencrypt/ssl-dhparams.pem` — общие параметры TLS, не выпуск сертификата на это имя
- прокси на `http://127.0.0.1:8088` (тот же `FRONTEND_PORT`)
- заголовки `X-Real-IP`, `X-Forwarded-For`, `X-Forwarded-Proto`, `server_tokens off`, `client_max_body_size 1m`, заголовки безопасности и лимит на `/register`, `/login`, `/refresh`

В compose у API включён `TRUST_PROXY=1`: адрес клиента берётся из `X-Real-IP`, который выставляет nginx. Не открывайте порт API напрямую с этой настройкой.

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

`--password` можно не передавать: в терминале пароль читается без эха. В скриптах и тестах по-прежнему передаётся флаг, из обычного stdin пароль не читается.

Для самоподписанного сертификата задайте `FLAMMEBIBLE_CA=cert.pem`. Сессия хранится в `~/.config/flammebible` (на Windows — `%AppData%\flammebible`), либо в каталоге `FLAMMEBIBLE_CONFIG`. Работать нужно только с копиями модулей VisioBible, не с оригиналами.

## Переменные окружения

| Имя | Кто использует | Назначение |
|---|---|---|
| `DATABASE_URL` | сервер, интеграционные тесты | строка подключения PostgreSQL |
| `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB` | compose | учётная запись контейнера PostgreSQL |
| `TLS_CERT`, `TLS_KEY` | сервер | необязательный TLS; оба сразу или оба пустые |
| `ADDR` | сервер | адрес listen, по умолчанию `:8080` |
| `SESSION_TTL` | сервер | срок сессии, по умолчанию `720h`; при `/refresh` срок начинается заново |
| `MEDIA_DIR` | сервер | каталог медиафайлов на томе |
| `TRUST_PROXY` | сервер | `1` — доверять `X-Real-IP` от своего прокси |
| `FRONTEND_PORT` | compose и nginx хоста | локальный порт сайта, по умолчанию `8088` |
| `VITE_API_TARGET` | сайт (`npm run dev`) | куда проксировать API |
| `FLAMMEBIBLE_CA` | CLI | дополнительный CA |
| `FLAMMEBIBLE_CONFIG` | CLI | каталог настроек |

Образец без секретов: `.env.example`.

Регистрация принимает почту вида `name@example.com` и пароль не короче 8 символов. На `/register`, `/login` и `/refresh` стоит лимит по IP: 5, 10 и 30 запросов в минуту. Тело запроса — не больше 1 МиБ.

## Тесты

```bash
go test ./internal/vbm
go test ./cmd/cli -skip 'TestTwoCLIProcessesExchangeSong|TestConflictBothOutcomes|TestDeleteVersusEditDoesNotChoose|TestBuildPatchOnServer'
```

Четыре интеграционных теста CLI и тесты `./internal/server` требуют PostgreSQL 16+ и `DATABASE_URL`. Тесты VBM и CLI не читают чужие песенники: синтетические `.vbm` собираются во временном каталоге.

## Что не входит в репозиторий

Программа VisioBible, архивы, сторонние `.vbm` / модули Библии и песенники, переписки и исследовательские дампы. Лицензия на их распространение не выяснена, и в репозитории с кодом им не место.
