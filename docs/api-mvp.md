# API MVP

Новые маршруты лежат рядом с уже существующими `/register`, `/login`, `/dimensions` и `/songbooks`. Префикс `/api` у новых путей. Сессия та же: `Authorization: Bearer <token>`.

Роль пользователя — `user` или `admin`. Поле `role` из тела регистрации не читается. Администратором становится адрес из `ADMIN_EMAIL` (сравнение без учёта регистра) в момент регистрации и повторно при входе. Пароль в окружении не задаётся.

Личный план принадлежит пользователю (`owner_user_id`). План общины принадлежит пространству `community` (`dimension_id`), участник должен состоять в нём. Сборники по-прежнему живут в пространстве: личное пространство создаётся при регистрации, община — отдельной записью.

## Переменные окружения

| Переменная | Назначение |
| --- | --- |
| `ADMIN_EMAIL` | Почта, которая получает `role=admin`. Необязательна. |
| `RELEASES_DIR` | Каталог файлов релизов клиента. Без неё загрузка и скачивание релизов отвечают ошибкой. Имя загруженного файла не используется: сервер пишет только `flammebible-linux`, `flammebible-windows.exe`, `flammebible-macos`. |
| `FLAMMEBIBLE_SERVER` | Адрес API для `cmd/upload`. По умолчанию `http://127.0.0.1:8080`. |
| `FLAMMEBIBLE_TOKEN` | Токен администратора для `cmd/upload`. |

Пример значений — в `.env.example`. Настоящих секретов там нет.

## Сессия и общины

`POST /register` и `POST /login` дополнительно возвращают `role`.

`GET /api/me` — `user_id`, `email`, `role`.

`POST /api/communities` и `POST /api/dimensions` создают общину (то же тело, что у `POST /dimensions`).

`GET /api/communities` — общины текущего пользователя. `GET /api/dimensions` — личное пространство и общины.

`GET /api/communities/{id}` и `GET /api/dimensions/{id}` — карточка, если пользователь состоит в пространстве.

`POST /api/communities/{id}/invites` и `POST /api/dimensions/{id}/invites` — приглашение администратора общины. Тело: `{"ttl_seconds":3600}`.

`POST /api/invites/{token}/accept` — вступление.

## Планы

Пункт `kind`:

- `song` — `song_id` и/или `title`, необязательный `songbook_id`
- `bible` — `module_short_name` (например `RST`), необязательный `module_short_name_second`, `book`, `chapter`, `verse`, `reference`, `quote`
- `text` — `caption`, `reference`, `quote`, `class` (если пустой, сервер ставит `-1`)

`POST /api/plans`

```json
{"title":"Воскресенье","dimension_id":"","items":[]}
```

Пустой `dimension_id` — личный план. Иначе план общины.

`GET /api/plans?scope=all|personal|community` и `GET /api/plans?dimension_id=...` — списки без удалённых. `GET /api/plans/{id}` отдаёт и tombstone.

`PUT /api/plans/{id}` и `POST /api/plans/{id}/push` заменяют название и пункты. В теле обязательны `version` и актуальные `items`. Если передан `updated_at`, он тоже должен совпасть с сервером. Несовпадение — `409` и тело `{"error":"конфликт","plan":...}`. Успех увеличивает `version` и `updated_at`.

`GET /api/plans/{id}/pull?version=&updated_at=` — `{"changed":false,"version":...,"updated_at":...}` либо `{"changed":true,"plan":...}`.

`DELETE /api/plans/{id}?version=` — помечает план удалённым и увеличивает версию. Повтор с той же версией идемпотентен.

`POST /api/plans/import` — `format` равен `json`, `vplt`, `history` или `plandef`. `payload` — текст. `title` необязателен.

Для `vplt`, `history` и `plandef` сервер не переписывает текст молча. Если payload — уже валидный UTF-8, но похож на кириллицу, прочитанную как windows-1251 или latin-1 и сохранённую снова, план записывается как прислан, а в ответ добавляются поля:

- `mojibake`: `true`
- `encoding_warning`: короткое предупреждение
- `suggested_text`: тот же файл после обратного прогона
- `suggested_title` и `suggested_items`: разбор `suggested_text`, если он читается

Обычный русский текст, немецкий и французский с умлаутами и смешанный текст этих языков флаг не получают. JSON-импорт не проверяется: строка в теле уже Unicode. Файлы `.vbm` этот разбор не касается.

`GET /api/plans/{id}/export?format=json|vplt|history|plandef&encoding=utf-8|windows-1251`.

`encoding` по умолчанию `utf-8`. Для XML заголовок `encoding` совпадает с байтами ответа: `UTF-8` или `windows-1251`. Если в тексте есть символ, которого нет в windows-1251, ответ `400` и `{"error":"в тексте есть символы, которых нет в windows-1251"}`: файл клиенту не отдаётся. JSON всегда в Unicode, параметр `encoding` для него не применяется.

`plan.vplt` пишет только `Caption`, `Reference`, `Quote`, `Class`: VisioBible при сохранении стирает незнакомые атрибуты. Книга, глава, стих и второй перевод сохраняются в `history` и `plandef`.

History — XML `VisioBibleJournal` / `Journal Name="Bible"|"Text"|"Songs"`. У стиха атрибуты `ModuleShortName`, `ModuleShortNameSecond`, `Book`, `Chapter`, `Verse`, `Reference`, `Quote`.

PlanDef — секции `[Plan]`, `[Bible.N]`, `[Text.N]`, `[Songs.N]`. Перевод строки в значении записывается как `\n`.

Байты windows-1251 принимаются разбором payload, если это не UTF-8. В JSON-теле строка уже Unicode. Повторно сохранённая кириллица (валидный UTF-8 с «ÐŸÑ€…» или «Ïðèâåò») не исправляется при чтении: импорт только предупреждает и предлагает текст.

## Поиск песен

`GET /api/songs/search?q=&songbook_id=&dimension_id=&community_id=&limit=&offset=`

Ищет по названию, альтернативному названию и тексту частей. Нужна сессия. Видны песни своих пространств (включая `draft`, `pending`, `rejected`) и одобренные публичные. `limit` от 1 до 100, по умолчанию 20. `offset` от 0 до 10000. Символы `%`, `_` и `\` в запросе не являются шаблоном.

Ответ: `{"songs":[{"songbook_id","fbid","title","songbook_title","dimension_id","status"}]}`.

## Модерация

У сборника и модуля Библии есть `status`: `draft`, `pending`, `approved`, `rejected`, а также `moderation_note`, `reviewed_by`, `reviewed_at`.

`POST /api/modules` — `short_name`, `title`, необязательный `dimension_id`. Статус `draft`.

`GET /api/modules` — свои и общинные. `GET /api/modules/{id}` — если есть доступ.

`POST /api/modules/{id}/submit` и `POST /api/songbooks/{id}/submit` переводят `draft` или `rejected` в `pending`. Для сборника по-прежнему работает `POST /songbooks/{id}/publish`.

`GET /api/catalog/songbooks` и `GET /api/catalog/modules` без сессии отдают только `approved`. У сборника при этом ещё `public=true`. Старый `GET /catalog/matches` тоже смотрит только одобренные публичные песни. Свои черновики видны в `GET /dimensions/{id}/songbooks` и `GET /api/modules`.

Только `role=admin`:

- `GET /api/admin/submissions?status=pending|draft|approved|rejected|all&kind=songbook|module`
- `POST /api/admin/songbooks/{id}/approve`
- `POST /api/admin/songbooks/{id}/reject` — тело `{"note":"причина"}`
- `POST /api/admin/songbooks/{id}/revoke` — снимает с публикации, статус снова `draft`. Тело `{}` или `{"note":"..."}`
- те же три пути для `/api/admin/modules/{id}/...`

Решение администратора пространства `POST /songbooks/{id}/publish/decision` сохранено: `public: true` ставит `approved`, `public: false` ставит `rejected`.

## Релизы клиента

Таблица `client_releases`: `version`, `status` (`dev` или `released`), хеши и размеры linux, windows и macos, `notes`, `created_at`, `published_at`. Активная версия — `app_settings.active_client_version`.

Версия — одна строка `[0-9A-Za-z][0-9A-Za-z._+-]{0,31}` без `..` и без разделителей пути.

Администратор:

- `POST /api/admin/releases` — `multipart/form-data`: `version`, файлы `linux_file`, `windows_file`, `macos_file` (хотя бы один), необязательные `notes` и `linux_sha256` / `windows_sha256` / `macos_sha256`. Хеш считает сервер. Если хеш передан и не совпал, запись не сохраняется. Повтор по версии `dev` обновляет только присланные файлы. Версия `released` не перезаписывается (`409`). Новый ряд — `201`, обновление — `200`.
- `GET /api/admin/releases` — `active_version` и список.
- `POST /api/admin/releases/{version}/publish` — `dev` становится `released`, активная версия обновляется. Нужны непустые linux и windows. Повторный вызов для уже выпущенной версии снова делает её активной.
- `DELETE /api/admin/releases/{version}` — только `dev`, вместе с каталогом файлов.

Без сессии:

- `GET /api/releases/latest` — активный выпущенный релиз или `404`.
- `GET /api/releases/{version}/download/{platform}` — `platform` равен `linux`, `windows` или `macos`. Только `released`. Заголовок `X-Content-SHA256`. Перед отдачей сервер сверяет файл с хешем и размером в базе.

Тело этого запроса может быть больше обычного предела 1 МиБ, до 512 МиБ на запрос и 200 МиБ на файл. Остальные маршруты остаются на 1 МиБ.

Загрузка с машины сборки:

```text
go run ./cmd/upload -server https://api.example.com -token "$FLAMMEBIBLE_TOKEN" -version 1.2.3 -linux ./flammebible -windows ./flammebible.exe
```

Токен берётся из флага или `FLAMMEBIBLE_TOKEN`. Пароль команда не спрашивает и не хранит.
