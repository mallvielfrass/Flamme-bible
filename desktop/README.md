# Настольный клиент FlammeBible

Отдельная программа на Tauri 2: она смотрит папку VisioBible на этом компьютере, сравнивает песни и планы с сервером и записывает результат. Перед заменой сборника или плана рядом остаётся копия с окончанием `.bak`.

Перед записью в папку VisioBible программа проверяет, не запущена ли она (`VisioBible.exe`, в том числе через Wine). Если запущена, файлы не меняются: на экране просьба закрыть её и нажать «Повторить».

Сервер по умолчанию — `http://localhost:8080`. В поле адреса можно указать и `https://example.com`. Почты, пароли и настоящие адреса в репозиторий не кладутся.

## Что умеет

- Вход и регистрация (`POST /login`, `POST /register`), карточка пользователя `GET /api/me`.
- Список пространств `GET /api/dimensions` и общин `GET /api/communities`.
- Сборники по прежним путям CLI: `/dimensions/{id}/songbooks`, снимок и `push`.
- Планы: список, создание, `push`/`pull` с `version` и `updated_at`, импорт и экспорт `json`, `vplt`, `history`, `plandef`.
- Поиск песен `GET /api/songs/search`.
- Проверка обновления `GET /api/releases/latest` и скачивание `GET /api/releases/{version}/download/{linux|windows|macos}` со сверкой `X-Content-SHA256`.

`plan.vplt` хранит только `Caption`, `Reference`, `Quote`, `Class`: VisioBible стирает остальные атрибуты. Номер стиха и перевод остаются в `History.ini` (это XML `VisioBibleJournal`) и в `PlanDef.ini` (`[Plan]`, `[Bible.N]`, `[Text.N]`, `[Songs.N]`). Файлы в Windows-1251 тоже читаются.

Настройки лежат в том же каталоге, что у консольного клиента: `%AppData%\flammebible` или `~/.config/flammebible` (переменная `FLAMMEBIBLE_CONFIG`). Файл клиента — `desktop.json`, сессию CLI он не затирает. Для самоподписанного сертификата сервера задайте `FLAMMEBIBLE_CA`.

## Разработка

Нужны Rust 1.90 или новее, Node.js 20 и npm.

Linux, чтобы открыть окно:

```text
sudo apt install libwebkit2gtk-4.1-dev libgtk-3-dev libayatana-appindicator3-dev librsvg2-dev patchelf
```

```bash
cd desktop
npm ci
npm run tauri dev
```

Проверки без окна:

```bash
cd desktop && npm ci && npm test && npm run build
cd desktop/src-tauri && cargo fmt --check && cargo clippy --all-targets -- -D warnings && cargo test
```

## Сборка

Из каталога `desktop`:

```bash
npm run tauri build
```

Готовые пакеты появляются в `desktop/src-tauri/target/release/bundle/`:

- Linux: deb и AppImage
- Windows: установщик NSIS (нужна сборка на Windows)
- macOS: dmg (нужна сборка на macOS)
