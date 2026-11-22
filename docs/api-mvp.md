# API MVP

Новые маршруты лежат рядом с уже существующими `/register`, `/login`, `/dimensions` и `/songbooks`. Префикс `/api` у новых путей. Сессия та же: `Authorization: Bearer <token>`.

Роль пользователя — `user`, `moderator` или `admin`. Поле `role` из тела регистрации не читается. Администратором становится адрес из `ADMIN_EMAIL` (сравнение без учёта регистра) в момент регистрации и повторно при входе. Пароль в окружении не задаётся. Модератора назначает только администратор сайта. Модератор открывает `/admin`, очередь сборников и журнал. Релизы, пользователей, роли и общины он не меняет.

Личный план принадлежит пользователю (`owner_user_id`). План общины принадлежит пространству `community` (`dimension_id`), участник должен состоять в нём. Сборники по-прежнему живут в пространстве: личное пространство создаётся при регистрации, община — отдельной записью.

## Переменные окружения

| Переменная | Назначение |
| --- | --- |
| `ADMIN_EMAIL` | Почта, которая получает `role=admin`. Необязательна. |
| `RELEASES_DIR` | Каталог файлов релизов клиента. Без неё загрузка и скачивание релизов отвечают ошибкой. Имя загруженного файла не используется: сервер пишет только `flammebible-linux`, `flammebible-windows.exe`, `flammebible-macos`. |
| `FLAMMEBIBLE_SERVER` | Адрес API для `cmd/upload`. По умолчанию `http://127.0.0.1:8080`. |
| `FLAMMEBIBLE_TOKEN` | Токен администратора для `cmd/upload`. |
| `EMBED_URL` | Полный адрес POST локальной модели, для text-embeddings-inference это `http://embed:80/embed`. Пустое значение выключает смысловой поиск. |
| `EMBED_MODEL` | Имя модели. По умолчанию `cointegrated/rubert-tiny2`: 312 чисел, пулинг CLS, без префиксов. У модели с `e5` в имени к запросу добавляется `query:`, к индексируемому тексту — `passage:`. |

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

У пункта есть `created_at` и `updated_at`. Клиент их не задаёт: сервер оставляет `created_at` при том же id, а `updated_at` меняет только если изменился текст пункта. Перестановка места `updated_at` не двигает. В JSON эти поля есть. В `plan.vplt`, History.ini и PlanDef.ini их нет: VisioBible их не хранит.

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

`POST /api/plans/parse` — то же тело `format` и `payload`, но план в базу не пишется. Ответ: `{"title","items","encoding_notice"}`. `encoding_notice` — те же поля, что импорт добавляет при подозрительной кодировке (`mojibake`, `encoding_warning`, `suggested_text`, `suggested_title`, `suggested_items`). Лимит пунктов и длины полей тот же, что у импорта: не больше 500 пунктов.

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

Ответ: `{"songs":[{"songbook_id","fbid","title","songbook_title","dimension_id","status","section_title"}]}`. `section_title` — название первого раздела, в котором стоит песня. Его нет, если разделов нет.

`section_id` сужает выдачу до песен одного раздела. Раздел должен принадлежать доступному сборнику. Если вместе с ним передан `songbook_id` другого сборника, ответ `400`. Неизвестный или чужой раздел — `404`. Фильтр `songbook_id` по-прежнему берёт ровно этот сборник и не включает вложенные сборники `parent_id`.

## Смысловой поиск

Нужны `EMBED_URL` и PostgreSQL с расширением `vector`. Если `EMBED_URL` пуст, `GET /api/songs/semantic` и `GET /api/bible/semantic` отвечают `503` и `{"error":"смысловой поиск недоступен"}`. Остальные маршруты, включая поиск по словам, работают. Если адрес задан, а модель не отвечает, клиент видит ту же ошибку.

Контейнер модели в compose поднимается профилем `semantic` и переменной `EMBED_URL`. Модель — `cointegrated/rubert-tiny2` на CPU (text-embeddings-inference, веса safetensors), пулинг `cls`. Колонки `vector(312)`. За один запрос к модели уходит не больше 8 текстов: столько принимает CPU-сборка. Если колонка была другого размера, сервер очищает её при запуске. После этого нужна полная переиндексация.

Индекс песни — вектор на каждую уникальную непустую часть: название и текст части. Пустая часть пропускается, одинаковый припев не считается второй раз. Текст части обрезается до 4000 символов. Повторно вектор не считается, если модель и хеш текста части не изменились. Лишние части удаляются. Удалённая песня и модуль, снятый с публикации, из поиска пропадают. Прежний вектор на всю песню миграция удаляет. Сервер с `EMBED_URL` ставит части в очередь `embedding_jobs` и считает их одним воркером. Скорость — токены в секунду: режим `auto` берёт долю измеренной мощности (пока замера нет, 16 ток/с), `manual` — фиксированное число, `0` или пауза останавливают воркер. Число токенов берётся из TEI `POST /tokenize`, иначе оценка «символы / 4», не больше 512. Поиск по смыслу отвечает по уже готовым векторам; поиск по словам работает всегда. Если `GET /info` показывает другую модель или размерность, новые векторы в старый набор не пишутся, песни ищутся по словам, а полный пересчёт запускает только админ: он собирает теневые таблицы и переключает их одной транзакцией. `go run ./cmd/reindex` только ставит задания, флаг `-process` их разбирает.

Админ выбирает желаемую модель Hugging Face (`PUT /api/admin/embed/model`) или возвращает `cointegrated/rubert-tiny2` (`POST /api/admin/embed/model/reset`). API не перезапускает контейнеры и не монтирует сокет Docker. После записи в базе на хосте выполняют `EMBED_MODEL=org/name docker compose --profile semantic up -d --force-recreate embed`. Пока живой `GET /info` не совпадёт с уже посчитанными векторами, поиск песен идёт по словам, а полный пересчёт — только кнопка в админке.

Профиль `llama` необязателен и не включается вместе с `semantic` на машине около 4 ГБ. Файл GGUF грузится `POST /api/admin/embed/gguf?name=model.gguf` в `EMBED_MODELS_DIR` (заголовок `GGUF`, без путей вне каталога). Затем `LLAMA_GGUF=model.gguf docker compose --profile llama up -d --force-recreate llama` и `EMBED_URL=http://llama:8080/v1/embeddings`. Файл на диске меньше, чем память процесса: вес 512 МиБ легко занимает больше 1.5 ГБ вместе с контекстом.

`go run ./cmd/reindex` пересчитывает все части песен и одобренные модули. Флаги `-import каталог -module id` читают BibleQuote (`bibleqt.ini` и файлы книг) в таблицу стихов модуля и затем делают ту же переиндексацию. Каталог перевода на сервер приносит оператор. Тексты переводов в репозиторий не входят. Пока модуль не одобрен, его стихи в поиске не видны. Одобрение индексирует уже загруженные стихи отрывками по 3–5 стихов одной главы. Глава короче трёх стихов остаётся одним отрывком.

`GET /api/songs/semantic?q=&songbook_id=&dimension_id=&community_id=&limit=`

Те же правила видимости, что у `GET /api/songs/search`: свои пространства и одобренные сборники. Скрытый сборник (`hidden_at`) в выдачу не попадает, векторы его частей сохраняются. `limit` от 1 до 100, по умолчанию 20. Ответ: `{"songs":[{"songbook_id","fbid","title","songbook_title","dimension_id","status","section_title","fragment"}]}`. `fragment` — текст ближайшей по смыслу части, обрезанный до 280 символов. `section_id` работает так же, как в поиске по словам. Порядок — от ближайшей по смыслу.

## Разделы сборника

Один сборник делится на разделы. Песня может стоять в нескольких. Название раздела уникально внутри сборника: повтор `POST` или `PUT` с занятым `title` возвращает `409` и `{"error":"раздел"}`. Клиент синхронизации сопоставляет группы `.vbm` с этими разделами по названию.

```sql
songbook_sections (id, songbook_id, title, position)
songbook_section_songs (section_id, songbook_id, fbid, position)
```

Писать раздел может тот же участник, кто правит сборник.

`GET /api/songbooks/{id}/sections` — `[{id,title,position,count,songs}]`. `songs` — список `fbid` по порядку.

`POST /api/songbooks/{id}/sections` — `{"title"}`.

`PUT /api/songbooks/{id}/sections/{sectionID}` — новое `title`.

`DELETE /api/songbooks/{id}/sections/{sectionID}` — удаляет раздел и его состав.

`PUT /api/sections/{id}/songs` — `{"fbids":["..."]}` заменяет состав. Песня должна быть в этом сборнике и не быть удалённой.

`GET /api/songbooks/tree` — сборники, которые видны в поиске, кроме скрытых. Группы по порядку: общины пользователя, личные, затем одобренные чужие.

```json
[{"group":"community","dimension_id":"...","name":"Преображение","songbooks":[{"id","title","short_name","status","sections":[{"id","title","count"}]}]}]
```

`group` также бывает `personal` и `approved`.

`GET /api/bible/semantic?q=&module=&limit=`

Нужна сессия. Видны только одобренные модули. `module` — id модуля или короткое имя, например `RST`. Ответ: `{"passages":[{"module_id","module_short_name","module_title","book","book_name","chapter","verse_start","verse_end","reference","text"}]}`. `book` — номер книги в порядке `PathName` модуля. `reference` вроде `Истории 2:1-2`.

## Модерация

У сборника и модуля Библии есть `status`: `draft`, `pending`, `approved`, `rejected`, а также `moderation_note`, `reviewed_by`, `reviewed_at`.

`POST /api/modules` — `short_name`, `title`, необязательный `dimension_id`. Статус `draft`.

`GET /api/modules` — свои и общинные. `GET /api/modules/{id}` — если есть доступ.

`POST /api/modules/{id}/submit` и `POST /api/songbooks/{id}/submit` переводят `draft` или `rejected` в `pending`. Для сборника по-прежнему работает `POST /songbooks/{id}/publish`.

`GET /api/catalog/songbooks` и `GET /api/catalog/modules` без сессии отдают только `approved`. У сборника при этом ещё `public=true`. Старый `GET /catalog/matches` тоже смотрит только одобренные публичные песни. Свои черновики видны в `GET /dimensions/{id}/songbooks` и `GET /api/modules`.

`submitted_at` ставится, когда сборник или модуль уходит на проверку. Для уже лежавших в очереди строк берётся дата решения, а если её нет — время миграции.

`role=admin` или `role=moderator`:

- `GET /api/admin/submissions?status=pending|draft|approved|rejected|hidden|all&kind=songbook|module&q=&sort=submitted&limit=&offset=`
  Ответ: `submissions`, `total`, `counts`. В строке есть `community_name`, `author_email`, `short_name`, `songs_count` или `books_count`, `submitted_at`, у сборника ещё `publisher` и `patch_count`. По умолчанию статус `pending`, порядок — сначала давно отправленные. `q` ищет по названию, короткому имени и почте автора. `limit` по умолчанию 50, максимум 200.
- `GET /api/admin/summary` — `pending_songbooks`, `pending_modules`, `draft_releases`, `active_version`.
- `GET /api/admin/songbooks/{id}` — карточка, первые песни, число песен и похожие песни из каталога. Смотреть можно любой сборник, не только публичный.
- `DELETE /api/admin/songbooks/{id}` — если есть патчи (`parent_id`), сборник и патчи получают `hidden_at` и пропадают из каталога, поиска, списка синхронизации и обычной очереди. Их видно при `status=hidden`. `POST /api/admin/songbooks/{id}/unhide` возвращает. Без патчей сборник удаляется вместе с песнями.
- `DELETE /api/admin/modules/{id}` удаляет модуль, стихи и отрывки.
- `DELETE /api/admin/communities/{id}` удаляет общину, её участников, приглашения, сборники и планы. Личное пространство так не удаляется.
- `GET /api/admin/modules/{id}` — карточка, книги и число стихов из `module_verses`.
- `POST /api/admin/songbooks/{id}/approve` и `reject` — только из `pending` или `rejected`. Иначе `409`.
- `POST /api/admin/songbooks/{id}/reject` — тело `{"note":"причина"}`.
- `POST /api/admin/songbooks/{id}/revoke` — только из `approved`, статус снова `draft`. Тело `{}` или `{"note":"..."}`. Иначе `409`.
- те же три пути для `/api/admin/modules/{id}/...`

`POST /songbooks/{id}/publish/decision` больше не обходит модерацию администратором общины. Вызов доступен только `role=admin` сайта: `public: true` ставит `approved`, `public: false` ставит `rejected`.

## Общины для администратора сайта

Администратор сайта управляет участниками любой общины, даже если сам в ней не состоит. Администратор общины по-прежнему видит своих.

- `GET /api/admin/communities?q=&limit=&offset=` — название, адрес, числа участников, админов, сборников и планов, плюс `total`.
- `GET /api/admin/communities/{id}/members` и `GET /api/communities/{id}/members` — `user_id`, `email`, `role`, `excluded`.
- `POST /api/communities/{id}/members/{userID}/exclude` и `/include`. Старый `POST /dimensions/{id}/members/{userID}/exclude` работает для администратора общины.
- `POST /api/communities/{id}/members/{userID}/role` — `{"role":"admin|member"}`. Последнего администратора общины снять нельзя.
- `GET /api/communities/{id}/invites` — активные приглашения, `digest` это hex SHA-256 токена, исходный токен сервер не хранит.
- `DELETE /api/communities/{id}/invites/{digest}` отзывает приглашение.
- `POST /api/communities/{id}/invites` администратор сайта может вызвать для любой общины.

## Пользователи

Только `role=admin`.

`users.created_at`, `users.blocked_at`, `users.last_seen_at`. Блокировка запрещает вход и обновление сессии и сразу отзывает семьи сессий. Материалы пользователя остаются.

- `GET /api/admin/users?q=&role=user|moderator|admin&blocked=0|1&limit=&offset=` — `users`, `total`, `counts`.
- `POST /api/admin/users/{id}/role` — `{"role":"user|moderator|admin"}`. Нельзя снять с себя права администратора. В системе остаётся хотя бы один администратор. Иначе `409`.
- `POST /api/admin/users/{id}/block` и `/unblock`. Администратора и себя блокировать нельзя (`409`).
- `POST /api/admin/users/{id}/sessions/revoke` — `session_families.revoked_at = now()`.
- `DELETE /api/admin/users/{id}` — не себя и не последнего администратора. Личное пространство удаляется вместе со сборниками, планами и модулями. Членство в чужих общинах снимается, общины и их материалы остаются.

`ADMIN_EMAIL` по-прежнему поднимает роль при регистрации и входе.

## Журнал

Таблица `admin_audit`: кто, когда, действие, вид и название цели, заметка. Строка пишется в той же транзакции, что и изменение: одобрение, отказ, снятие, скрытие и возврат, удаление, смена роли, блок, отзыв сессий, загрузка и публикация сборки, откат, удаление черновика, заметки к версии, исключение и возврат участника, смена роли в общине, отзыв приглашения, `publish/decision`.

`GET /api/admin/audit?actor=&action=&target_kind=&limit=&offset=` — администратор и модератор. `actor` — id или часть почты. В строке есть `summary` по-русски. В карточке сборника и модуля поле `audit` — последние записи по этой цели.

## Релизы клиента

Таблица `client_releases`: `version`, `status` (`dev` или `released`), хеши и размеры linux, windows и macos, `notes`, `created_at`, `published_at`. Активная версия — `app_settings.active_client_version`.

Версия — одна строка `[0-9A-Za-z][0-9A-Za-z._+-]{0,31}` без `..` и без разделителей пути.

Администратор:

- `POST /api/admin/releases` — `multipart/form-data`: `version`, файлы `linux_file`, `windows_file`, `macos_file` (хотя бы один), необязательные `notes` и `linux_sha256` / `windows_sha256` / `macos_sha256`. Хеш считает сервер. Если хеш передан и не совпал, запись не сохраняется. Повтор по версии `dev` обновляет только присланные файлы. Версия `released` не перезаписывается (`409`). Новый ряд — `201`, обновление — `200`.
- `GET /api/admin/releases` — `active_version` и список.
- `GET /api/admin/releases/{version}/download/{platform}` — файл черновика или опубликованной сборки, только администратор. Публичный `GET /api/releases/...` по-прежнему отдаёт только `released`.
- `PATCH /api/admin/releases/{version}` — `{"notes":"..."}` для любой версии.
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
