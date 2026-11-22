mod config;
mod mojibake;
mod paths;
mod plan_tab;
mod plans;
mod remote;
mod running;
mod sync;
mod text;
mod updater;
mod visio;

use std::path::Path;
use std::sync::atomic::{AtomicBool, Ordering};

use tauri::{Emitter, Manager};
use uuid::Uuid;

static CLOSE_GUARD: AtomicBool = AtomicBool::new(false);
static ALLOW_CLOSE: AtomicBool = AtomicBool::new(false);

use crate::plans::PlanItem;
use crate::remote::{HeaderTemplate, NewSongbook, PushOp, Remote, RemoteError};
use crate::sync::{BookIn, Input, PlanIn, Preview, RemoteBook};
use crate::visio::{Book, Header, Scan};

#[derive(Clone, serde::Serialize)]
struct Progress {
    stage: String,
    current: String,
    completed: u32,
    total: u32,
    percentage: u32,
}

#[derive(serde::Deserialize)]
struct StartRequest {
    #[serde(default)]
    songs: Vec<sync::SongDecision>,
    #[serde(default)]
    plans: Vec<sync::PlanDecision>,
}

#[derive(serde::Serialize)]
struct SyncReport {
    notes: Vec<String>,
    backups: Vec<String>,
}

#[derive(serde::Serialize)]
struct UpdateView {
    current: String,
    latest: String,
    notes: String,
    available: bool,
    platform: String,
}

fn settings_dir() -> Result<std::path::PathBuf, String> {
    paths::config_dir()
}

fn load_config() -> Result<(std::path::PathBuf, config::Config), String> {
    let dir = settings_dir()?;
    let cfg = config::load(&dir)?;
    Ok((dir, cfg))
}

fn device_id(cfg: &mut config::Config) -> String {
    if cfg.user.device_id.is_empty() {
        cfg.user.device_id = Uuid::new_v4().to_string();
    }
    cfg.user.device_id.clone()
}

fn require_login(cfg: &config::Config) -> Result<(), String> {
    if cfg.token.is_empty() || cfg.user.dimension_id.is_empty() {
        return Err("Сначала войдите в учётную запись".into());
    }
    Ok(())
}

#[tauri::command]
fn client_version() -> String {
    updater::current_version().to_string()
}

#[tauri::command]
fn load_settings() -> Result<config::PublicSettings, String> {
    let (_, cfg) = load_config()?;
    Ok(cfg.public())
}

#[tauri::command]
fn save_settings(server_url: String) -> Result<config::PublicSettings, String> {
    let (dir, mut cfg) = load_config()?;
    config::apply_server_url(&mut cfg, &server_url)?;
    config::save(&dir, &cfg)?;
    Ok(cfg.public())
}

#[tauri::command]
async fn register(email: String, password: String) -> Result<config::PublicSettings, String> {
    sign_in(true, email, password).await
}

#[tauri::command]
async fn login(email: String, password: String) -> Result<config::PublicSettings, String> {
    sign_in(false, email, password).await
}

async fn sign_in(
    create: bool,
    email: String,
    password: String,
) -> Result<config::PublicSettings, String> {
    let email = email.trim().to_string();
    if !email.contains('@') || email.len() > 320 || password.is_empty() {
        return Err("Укажите почту и пароль".into());
    }
    let (dir, mut cfg) = load_config()?;
    let device = device_id(&mut cfg);
    config::save(&dir, &cfg)?;
    let client = Remote::new(&cfg.server_url, "")?;
    let auth = if create {
        client.register(&email, &password, &device).await?
    } else {
        client.login(&email, &password, &device).await?
    };
    cfg.token = auth.token;
    cfg.user.user_id = auth.user_id;
    cfg.user.device_id = if auth.device_id.is_empty() {
        device
    } else {
        auth.device_id
    };
    cfg.user.dimension_id = auth.dimension_id;
    cfg.user.email = email;
    cfg.user.role = auth.role;
    if let Ok(me) = Remote::new(&cfg.server_url, &cfg.token)?.me().await {
        if !me.user_id.is_empty() {
            cfg.user.user_id = me.user_id;
        }
        cfg.user.role = me.role;
        if !me.email.is_empty() {
            cfg.user.email = me.email;
        }
    }
    config::save(&dir, &cfg)?;
    Ok(cfg.public())
}

#[tauri::command]
async fn logout() -> Result<config::PublicSettings, String> {
    let (dir, mut cfg) = load_config()?;
    if !cfg.token.is_empty() {
        let _ = Remote::new(&cfg.server_url, &cfg.token)?.logout().await;
    }
    cfg.token.clear();
    config::save(&dir, &cfg)?;
    Ok(cfg.public())
}

#[tauri::command]
async fn list_dimensions() -> Result<Vec<remote::Dimension>, String> {
    let (_, cfg) = load_config()?;
    require_login(&cfg)?;
    Remote::new(&cfg.server_url, &cfg.token)?.dimensions().await
}

#[tauri::command]
async fn select_dimension(dimension_id: String) -> Result<config::PublicSettings, String> {
    let (dir, mut cfg) = load_config()?;
    require_login(&cfg)?;
    cfg.user.dimension_id = dimension_id;
    config::save(&dir, &cfg)?;
    Ok(cfg.public())
}

#[tauri::command]
async fn create_community(name: String) -> Result<remote::Dimension, String> {
    let (_, cfg) = load_config()?;
    require_login(&cfg)?;
    let name = name.trim().to_string();
    if name.is_empty() {
        return Err("Укажите название общины".into());
    }
    Remote::new(&cfg.server_url, &cfg.token)?
        .create_community(&name)
        .await
}

#[tauri::command]
fn open_visio(path: String) -> Result<Scan, String> {
    let (dir, mut cfg) = load_config()?;
    let scan = visio::open_location(Path::new(&path))?;
    cfg.visio_path = path;
    config::save(&dir, &cfg)?;
    Ok(scan)
}

#[tauri::command]
async fn preview_sync() -> Result<Preview, String> {
    let (dir, cfg, Gathered(input, _, warning)) = gather().await?;
    let mut preview = sync::preview(&input)?;
    if !warning.is_empty() {
        preview.warnings.push(warning);
    }
    let _ = (dir, cfg);
    Ok(preview)
}

#[tauri::command]
async fn start_sync(app: tauri::AppHandle, request: StartRequest) -> Result<SyncReport, String> {
    let (dir, mut cfg, Gathered(input, cursor, warning)) = gather().await?;
    if !warning.is_empty() {
        return Err(warning);
    }
    let mut next_id = || Uuid::new_v4().to_string();
    let outcome = sync::apply(&input, &request.songs, &request.plans, &mut next_id)?;
    let writes_local =
        outcome.books.iter().any(|book| book.write) || outcome.plans.iter().any(|plan| plan.write);
    if writes_local {
        refuse_if_visiobible_open(&app, &outcome)?;
    }
    let mut notes = outcome.warnings.clone();
    let client = Remote::new(&cfg.server_url, &cfg.token)?;
    let mut steps = Vec::new();
    for book in &outcome.books {
        if book.create || !book.push.is_empty() {
            steps.push(("Отправка на сервер", book.title.clone()));
        }
        if book.write {
            steps.push(("Запись на компьютер", book.title.clone()));
        }
    }
    for plan in &outcome.plans {
        if plan.push || plan.create {
            steps.push(("Отправка плана", plan.title.clone()));
        }
        if plan.write {
            steps.push(("Запись плана", plan.title.clone()));
        }
    }
    let total = steps.len() as u32;
    let mut completed = 0u32;
    let mut backups = Vec::new();
    let mut cursor = cursor;
    if steps.is_empty() {
        emit(&app, "Готово", "Изменений нет", 1, 1);
        notes.push("Изменений нет: сборники и планы совпадают с сервером.".into());
    }
    let mut step_index = 0usize;
    for book in &outcome.books {
        let mut server_id = book.server_id.clone();
        if book.create || !book.push.is_empty() {
            emit_step(&app, &steps, step_index, completed, total);
            step_index += 1;
            if server_id.is_empty() {
                let created = client
                    .create_songbook(
                        &cfg.user.dimension_id,
                        &NewSongbook {
                            fbid: &book.book_fbid,
                            title: &book.title,
                            short_name: &book.short_name,
                            publisher: &book.publisher,
                            revision_date: &book.revision_date,
                            comment: &book.comment,
                            header: &HeaderTemplate {
                                format_version: book.format_version.clone(),
                                flags: book.flags,
                                charset: book.charset.clone(),
                                header_short: book.header_short.clone(),
                                header_title: book.header_title.clone(),
                            },
                        },
                    )
                    .await?;
                server_id = created.id;
            }
            if !book.push.is_empty() {
                cursor =
                    push_songs(&client, &cfg.user.dimension_id, &server_id, cursor, book).await?;
                notes.push(format!(
                    "Песни сборника «{}» отправлены на сервер.",
                    book.title
                ));
            }
            completed += 1;
        }
        if book.write {
            emit_step(&app, &steps, step_index, completed, total);
            ensure_visiobible_closed()?;
            step_index += 1;
            let existed = Path::new(&book.path).is_file();
            visio::write_book_file(Path::new(&book.path), &book_from(book))?;
            if existed {
                backups.push(
                    visio::backup_path(Path::new(&book.path))
                        .display()
                        .to_string(),
                );
                notes.push(format!(
                    "Сборник «{}» записан. Рядом сохранена копия с окончанием .bak.",
                    book.title
                ));
            } else {
                notes.push(format!("Создан сборник «{}».", book.title));
            }
            completed += 1;
        }
        cfg.songbooks.insert(
            book.path.clone(),
            config::SongbookBind {
                server_id,
                book_fbid: book.book_fbid.clone(),
                cursor,
                songs: book.songs.clone(),
            },
        );
    }
    for plan in &outcome.plans {
        let mut server_id = plan.server_id.clone();
        let mut version = plan.version;
        let mut updated_at = plan.updated_at.clone();
        if plan.push || plan.create {
            emit_step(&app, &steps, step_index, completed, total);
            step_index += 1;
            let saved = if server_id.is_empty() {
                client
                    .create_plan(&plan.title, &plan.dimension_id, &plan.items)
                    .await
            } else {
                client
                    .push_plan(
                        &server_id,
                        &plan.title,
                        version.max(1),
                        &updated_at,
                        &plan.items,
                    )
                    .await
            };
            match saved {
                Ok(view) => {
                    server_id = view.id;
                    version = view.version;
                    updated_at = view.updated_at;
                    notes.push(format!("План «{}» отправлен на сервер.", plan.title));
                }
                Err(RemoteError::PlanConflict(_)) => {
                    return Err(
                        "План на сервере уже изменился. Откройте список изменений ещё раз.".into(),
                    );
                }
                Err(RemoteError::Message(message)) => return Err(message),
            }
            completed += 1;
        }
        if plan.write {
            emit_step(&app, &steps, step_index, completed, total);
            ensure_visiobible_closed()?;
            step_index += 1;
            let path = Path::new(&plan.path);
            let existed = path.is_file();
            let bytes = plans::render_file(path, &plan.items, &plan.title, &cfg.plan_encoding)?;
            if existed {
                let _ = std::fs::copy(path, visio::backup_path(path));
            }
            if let Some(parent) = path.parent() {
                if !parent.as_os_str().is_empty() {
                    std::fs::create_dir_all(parent).map_err(|err| err.to_string())?;
                }
            }
            std::fs::write(path, bytes).map_err(|err| err.to_string())?;
            if existed {
                backups.push(visio::backup_path(path).display().to_string());
                notes.push(format!(
                    "План «{}» записан. Рядом сохранена копия с окончанием .bak.",
                    plan.title
                ));
            }
            completed += 1;
        }
        if !plan.path.is_empty() {
            cfg.plans.insert(
                plan.path.clone(),
                config::PlanBind {
                    server_id,
                    dimension_id: plan.dimension_id.clone(),
                    version,
                    updated_at,
                    scope: if plan.dimension_id.is_empty() {
                        "personal".into()
                    } else {
                        "community".into()
                    },
                    items: plan.items.clone(),
                },
            );
        }
    }
    emit(
        &app,
        "Готово",
        "Синхронизация закончена",
        total.max(1),
        total.max(1),
    );
    config::save(&dir, &cfg)?;
    Ok(SyncReport { notes, backups })
}

fn ensure_visiobible_closed() -> Result<(), String> {
    if running::visiobible_running() {
        return Err(running::CLOSE_MESSAGE.to_string());
    }
    Ok(())
}

fn refuse_if_visiobible_open(
    app: &tauri::AppHandle,
    outcome: &sync::Outcome,
) -> Result<(), String> {
    if !running::visiobible_running() {
        return Ok(());
    }
    let current = outcome
        .books
        .iter()
        .find(|book| book.write)
        .map(|book| book.title.clone())
        .or_else(|| {
            outcome
                .plans
                .iter()
                .find(|plan| plan.write)
                .map(|plan| plan.title.clone())
        })
        .unwrap_or_default();
    emit(app, "Запись на компьютер", &current, 0, 1);
    Err(running::CLOSE_MESSAGE.to_string())
}

#[tauri::command]
fn set_skipped(kind: String, id: String, skip: bool) -> Result<(), String> {
    let (dir, mut cfg) = load_config()?;
    require_login(&cfg)?;
    config::set_skipped(&mut cfg, &kind, &id, skip)?;
    config::save(&dir, &cfg)?;
    Ok(())
}

fn emit_step(
    app: &tauri::AppHandle,
    steps: &[(&str, String)],
    index: usize,
    completed: u32,
    total: u32,
) {
    if let Some((stage, current)) = steps.get(index) {
        emit(app, stage, current, completed, total);
    }
}

fn emit(app: &tauri::AppHandle, stage: &str, current: &str, completed: u32, total: u32) {
    let _ = app.emit(
        "sync-progress",
        Progress {
            stage: stage.to_string(),
            current: current.to_string(),
            completed,
            total,
            percentage: sync::percentage(completed, total),
        },
    );
}

async fn push_songs(
    client: &Remote,
    dimension_id: &str,
    book_id: &str,
    mut cursor: i64,
    book: &sync::BookWork,
) -> Result<i64, String> {
    let mut ops: Vec<PushOp> = book
        .push
        .iter()
        .map(|song| PushOp {
            operation_id: Uuid::new_v4().to_string(),
            song: song.clone(),
            order: None,
        })
        .collect();
    if let Some(last) = ops.last_mut() {
        last.order = Some(book.order.clone());
    }
    for chunk in ops.chunks(100) {
        let results = client.push(dimension_id, book_id, cursor, chunk).await?;
        for result in results {
            if result.status == "conflict" {
                return Err("На сервере появилась более новая правка песни. Откройте список изменений ещё раз.".into());
            }
            if result.status != "applied" {
                return Err("Сервер не принял песню".into());
            }
            if result.revision > cursor {
                cursor = result.revision;
            }
        }
    }
    Ok(cursor)
}

fn book_from(book: &sync::BookWork) -> Book {
    Book {
        header: Header {
            format_version: book.format_version.clone(),
            compressed: 0,
            uncompressed: 0,
            flags: book.flags,
            charset: book.charset.clone(),
            header_short: book.header_short.clone(),
            header_title: book.header_title.clone(),
            zlib_offset: 0,
        },
        title: book.title.clone(),
        short_name: book.short_name.clone(),
        publisher: book.publisher.clone(),
        revision_date: book.revision_date.clone(),
        comment: book.comment.clone(),
        songs: book.songs.clone(),
        groups: Vec::new(),
        bytes_left: 0,
    }
}

struct Gathered(Input, i64, String);

async fn gather() -> Result<(std::path::PathBuf, config::Config, Gathered), String> {
    let (dir, mut cfg) = load_config()?;
    require_login(&cfg)?;
    if cfg.visio_path.is_empty() {
        return Err("Сначала выберите папку VisioBible или Modules".into());
    }
    let scan = visio::open_location(Path::new(&cfg.visio_path))?;
    if scan.needs_choice {
        return Err("Найдено несколько папок. Выберите одну.".into());
    }
    let mut pending = cfg.pending_fbids.clone();
    let mut books = Vec::new();
    for info in &scan.songbooks {
        if !info.error.is_empty() {
            continue;
        }
        let parsed = visio::parse_file(Path::new(&info.path))?;
        let songs = visio::prepare_local_songs(&info.path, &parsed.songs, &mut pending, || {
            Uuid::new_v4().to_string()
        })?;
        let bind = cfg.songbooks.get(&info.path);
        books.push(BookIn {
            path: info.path.clone(),
            title: parsed.title.clone(),
            short_name: parsed.short_name.clone(),
            publisher: parsed.publisher.clone(),
            revision_date: parsed.revision_date.clone(),
            comment: parsed.comment.clone(),
            format_version: parsed.header.format_version.clone(),
            charset: parsed.header.charset.clone(),
            flags: parsed.header.flags,
            header_short: parsed.header.header_short.clone(),
            header_title: parsed.header.header_title.clone(),
            original: parsed.songs,
            songs,
            base: bind.map(|bind| bind.songs.clone()).unwrap_or_default(),
            server_id: bind.map(|bind| bind.server_id.clone()).unwrap_or_default(),
            book_fbid: bind.map(|bind| bind.book_fbid.clone()).unwrap_or_default(),
            has_groups: !parsed.groups.is_empty(),
        });
    }
    cfg.pending_fbids = pending;
    config::save(&dir, &cfg)?;
    let client = Remote::new(&cfg.server_url, &cfg.token)?;
    let views = client.songbooks(&cfg.user.dimension_id).await?;
    let mut remote_books = Vec::new();
    let mut cursor = 0i64;
    for view in views {
        let snap = client.snapshot(&cfg.user.dimension_id, &view.id).await?;
        cursor = cursor.max(snap.cursor);
        let mut songs = Vec::new();
        for song in snap.songs {
            songs.push(visio::normalize_remote_song(&song)?);
        }
        remote_books.push(RemoteBook {
            server_id: view.id,
            fbid: view.fbid,
            title: view.title,
            short_name: view.short_name,
            publisher: view.publisher,
            revision_date: view.revision_date,
            comment: view.comment,
            format_version: view.header.format_version,
            charset: view.header.charset,
            flags: view.header.flags,
            header_short: view.header.header_short,
            header_title: view.header.header_title,
            songs,
            order: snap.order,
        });
    }
    let (remote_plans, plan_warning) = match client.plans("all").await {
        Ok(plans) => (plans, String::new()),
        Err(err) => (Vec::new(), format!("Планы на сервере не прочитаны: {err}")),
    };
    let mut local_plans = Vec::new();
    for info in &scan.plans {
        let bind = cfg.plans.get(&info.path);
        local_plans.push(PlanIn {
            key: info.path.clone(),
            path: info.path.clone(),
            title: info.name.clone(),
            format: plans::file_format(Path::new(&info.path)).to_string(),
            items: info.items.clone(),
            base: bind.map(|bind| bind.items.clone()).unwrap_or_default(),
            server_id: bind.map(|bind| bind.server_id.clone()).unwrap_or_default(),
            dimension_id: bind
                .map(|bind| bind.dimension_id.clone())
                .filter(|id| !id.is_empty())
                .unwrap_or_default(),
            version: bind.map(|bind| bind.version).unwrap_or(0),
            updated_at: bind.map(|bind| bind.updated_at.clone()).unwrap_or_default(),
        });
    }
    let skipped = cfg.skipped_for();
    let input = Input {
        books,
        plans: local_plans,
        remote_books,
        remote_plans,
        modules_dir: scan.modules_dir,
        plans_dir: scan.plans_dir,
        skip_songbooks: skipped.songbooks.into_iter().collect(),
        skip_plans: skipped.plans.into_iter().collect(),
    };
    Ok((dir, cfg, Gathered(input, cursor, plan_warning)))
}

#[tauri::command]
fn create_plan(name: String, items: Vec<PlanItem>) -> Result<visio::PlanInfo, String> {
    let (_, cfg) = load_config()?;
    if cfg.visio_path.is_empty() {
        return Err("Сначала выберите папку VisioBible".into());
    }
    let scan = visio::open_location(Path::new(&cfg.visio_path))?;
    if scan.needs_choice || scan.plans_dir.is_empty() {
        return Err("Сначала выберите одну папку VisioBible".into());
    }
    let name = name.trim().to_string();
    if name.is_empty() {
        return Err("Укажите название плана".into());
    }
    let path = plans::write_new_plan(
        Path::new(&scan.plans_dir),
        &name,
        &items,
        &cfg.plan_encoding,
    )?;
    Ok(visio::PlanInfo {
        path: path.display().to_string(),
        name,
        kind: plans::file_format(&path).to_string(),
        items,
        mojibake: false,
        encoding_warning: String::new(),
        preview_before: String::new(),
        preview_after: String::new(),
    })
}

#[tauri::command]
fn set_plan_encoding(windows1251: bool) -> Result<config::PublicSettings, String> {
    let (dir, mut cfg) = load_config()?;
    cfg.plan_encoding = if windows1251 {
        "windows-1251".into()
    } else {
        String::new()
    };
    config::save(&dir, &cfg)?;
    Ok(cfg.public())
}

#[tauri::command]
fn repair_plan_encoding(path: String) -> Result<visio::Scan, String> {
    let (_, cfg) = load_config()?;
    if cfg.visio_path.is_empty() {
        return Err("Сначала выберите папку VisioBible".into());
    }
    let file = Path::new(&path);
    let bytes = plans::repaired_utf8(file)?;
    visio::replace_file(file, &bytes)?;
    visio::open_location(Path::new(&cfg.visio_path))
}

#[tauri::command]
async fn list_communities() -> Result<Vec<remote::Dimension>, String> {
    let (_, cfg) = load_config()?;
    require_login(&cfg)?;
    Remote::new(&cfg.server_url, &cfg.token)?
        .communities()
        .await
}

#[tauri::command]
async fn pull_plan(
    id: String,
    version: i64,
    updated_at: String,
) -> Result<remote::PlanPull, String> {
    let (_, cfg) = load_config()?;
    require_login(&cfg)?;
    Remote::new(&cfg.server_url, &cfg.token)?
        .pull_plan(&id, version, &updated_at)
        .await
        .map_err(|err| err.to_string())
}

#[tauri::command]
async fn delete_plan(id: String, version: i64) -> Result<plans::RemotePlan, String> {
    let (_, cfg) = load_config()?;
    require_login(&cfg)?;
    Remote::new(&cfg.server_url, &cfg.token)?
        .delete_plan(&id, version)
        .await
        .map_err(|err| err.to_string())
}

#[tauri::command]
async fn import_plan(
    format: String,
    payload: String,
    title: String,
    dimension_id: String,
) -> Result<plans::RemotePlan, String> {
    let (_, cfg) = load_config()?;
    require_login(&cfg)?;
    Remote::new(&cfg.server_url, &cfg.token)?
        .import_plan(&format, &payload, &title, &dimension_id)
        .await
        .map_err(|err| err.to_string())
}

#[tauri::command]
async fn export_plan(id: String, format: String) -> Result<String, String> {
    let (_, cfg) = load_config()?;
    require_login(&cfg)?;
    Remote::new(&cfg.server_url, &cfg.token)?
        .export_plan(&id, &format)
        .await
}

#[tauri::command]
async fn search_songs(query: String) -> Result<Vec<remote::SongHit>, String> {
    let (_, cfg) = load_config()?;
    require_login(&cfg)?;
    Remote::new(&cfg.server_url, &cfg.token)?
        .search_songs(query.trim(), &cfg.user.dimension_id)
        .await
}

#[tauri::command]
async fn check_update() -> Result<UpdateView, String> {
    let (_, cfg) = load_config()?;
    let info = Remote::new(&cfg.server_url, "")?.latest_release().await?;
    let platform = updater::this_platform();
    Ok(UpdateView {
        current: updater::current_version().into(),
        latest: info.version.clone(),
        notes: info.notes.clone(),
        available: updater::version_is_newer(updater::current_version(), &info.version)
            && info.status == "released"
            && info.file(platform).is_some(),
        platform: platform.into(),
    })
}

#[tauri::command]
async fn download_update() -> Result<String, String> {
    let (dir, cfg) = load_config()?;
    let client = remote::Remote::download_client(&cfg.server_url)?;
    let info = client.latest_release().await?;
    let platform = updater::this_platform();
    let file = info
        .file(platform)
        .ok_or("Для этой системы нет файла обновления")?;
    if file.platform != platform {
        return Err("Для этой системы нет файла обновления".into());
    }
    let download = client.download_release(&info.version, platform).await?;
    if download.bytes.len() as i64 != file.size {
        return Err("Размер файла обновления не совпал".into());
    }
    let path = updater::save_verified(
        &dir,
        &info.version,
        platform,
        &download.bytes,
        &download.sha256,
        &file.sha256,
    )?;
    Ok(path.display().to_string())
}

#[tauri::command]
fn set_close_guard(enabled: bool) {
    CLOSE_GUARD.store(enabled, Ordering::SeqCst);
}

#[tauri::command]
fn finish_close(window: tauri::WebviewWindow) -> Result<(), String> {
    ALLOW_CLOSE.store(true, Ordering::SeqCst);
    if let Err(err) = window.close() {
        ALLOW_CLOSE.store(false, Ordering::SeqCst);
        return Err(err.to_string());
    }
    Ok(())
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_dialog::init())
        .setup(|app| {
            if let Some(window) = app.get_webview_window("main") {
                let emitter = window.clone();
                window.on_window_event(move |event| {
                    if let tauri::WindowEvent::CloseRequested { api, .. } = event {
                        if CLOSE_GUARD.load(Ordering::SeqCst) && !ALLOW_CLOSE.load(Ordering::SeqCst)
                        {
                            api.prevent_close();
                            let _ = emitter.emit("close-requested", ());
                        }
                    }
                });
            }
            Ok(())
        })
        .invoke_handler(tauri::generate_handler![
            client_version,
            load_settings,
            save_settings,
            register,
            login,
            logout,
            list_dimensions,
            select_dimension,
            create_community,
            open_visio,
            preview_sync,
            start_sync,
            set_skipped,
            create_plan,
            set_plan_encoding,
            repair_plan_encoding,
            list_communities,
            pull_plan,
            delete_plan,
            import_plan,
            export_plan,
            search_songs,
            check_update,
            download_update,
            set_close_guard,
            finish_close,
            plan_tab::plans_list,
            plan_tab::plans_read,
            plan_tab::plans_create,
            plan_tab::plans_save,
            plan_tab::plans_delete,
            plan_tab::plans_import,
            plan_tab::plans_parse,
            plan_tab::plans_download,
            plan_tab::plans_folder,
            plan_tab::plans_songbooks,
            plan_tab::plans_find_songs,
        ])
        .run(tauri::generate_context!())
        .expect("окно не открылось");
}
