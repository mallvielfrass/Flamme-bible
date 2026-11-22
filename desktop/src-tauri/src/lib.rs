mod ai_http;
mod bible;
mod config;
mod groups;
mod mojibake;
mod paths;
mod plan_tab;
mod plans;
mod portable;
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
    #[serde(default)]
    groups: Vec<sync::GroupDecision>,
}

#[derive(serde::Serialize)]
struct SyncReport {
    notes: Vec<String>,
    backups: Vec<String>,
}

fn module_reply_matches(reply: &remote::ModuleExists, module: &bible::BibleIdentity) -> bool {
    (reply.short_name.is_empty() || reply.short_name == module.short_name)
        && (reply.title.is_empty() || reply.title == module.title)
        && (reply.content_hash.is_empty() || reply.content_hash == module.content_hash)
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
    let outcome = sync::apply(
        &input,
        &request.songs,
        &request.plans,
        &request.groups,
        &mut next_id,
    )?;
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
        if book
            .group_ops
            .iter()
            .any(|row| row.create || row.rename || row.push_songs)
        {
            steps.push(("Разделы сборника", book.title.clone()));
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
        let mut links: Vec<config::GroupLink> = book
            .group_state
            .iter()
            .map(|row| config::GroupLink {
                section_id: row.section_id.clone(),
                name: row.name.clone(),
                songs: row.songs.clone(),
            })
            .collect();
        if book
            .group_ops
            .iter()
            .any(|row| row.create || row.rename || row.push_songs)
        {
            emit_step(&app, &steps, step_index, completed, total);
            step_index += 1;
            if server_id.is_empty() {
                return Err("Сборник ещё не создан на сервере".into());
            }
            apply_groups(&client, &server_id, book, &mut links).await?;
            notes.push(format!(
                "Группы сборника «{}» сверены с разделами.",
                book.title
            ));
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
                groups: links,
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
        groups: book.groups.clone(),
        bytes_left: 0,
    }
}

async fn apply_groups(
    client: &Remote,
    book_id: &str,
    book: &sync::BookWork,
    links: &mut [config::GroupLink],
) -> Result<(), String> {
    let live: std::collections::BTreeSet<&str> =
        book.songs.iter().map(|song| song.fbid.as_str()).collect();
    for op in &book.group_ops {
        let songs: Vec<String> = op
            .songs
            .iter()
            .filter(|id| live.contains(id.as_str()))
            .cloned()
            .collect();
        let section_id = if op.create {
            let id = ensure_section(client, book_id, &op.name).await?;
            if let Some(link) = links
                .iter_mut()
                .find(|link| link.section_id.is_empty() && link.name == op.name)
            {
                link.section_id = id.clone();
            }
            id
        } else {
            if op.rename {
                client
                    .rename_section(book_id, &op.section_id, &op.name)
                    .await?;
            }
            op.section_id.clone()
        };
        if op.push_songs || (op.create && !songs.is_empty()) {
            client.set_section_songs(&section_id, &songs).await?;
        }
    }
    Ok(())
}

async fn ensure_section(client: &Remote, book_id: &str, title: &str) -> Result<String, String> {
    match client.create_section(book_id, title).await {
        Ok(section) => Ok(section.id),
        Err(err) if err == "раздел" => client
            .sections(book_id)
            .await?
            .into_iter()
            .find(|section| section.title == title)
            .map(|section| section.id)
            .ok_or(err),
        Err(err) => Err(err),
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
            groups: parsed.groups,
            base_groups: bind
                .map(|bind| {
                    bind.groups
                        .iter()
                        .map(|group| groups::Baseline {
                            section_id: group.section_id.clone(),
                            name: group.name.clone(),
                            songs: group.songs.clone(),
                        })
                        .collect()
                })
                .unwrap_or_default(),
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
        let mut sections = client.sections(&view.id).await?;
        sections.sort_by(|left, right| {
            left.position
                .cmp(&right.position)
                .then(left.title.cmp(&right.title))
                .then(left.id.cmp(&right.id))
        });
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
            sections: sections
                .into_iter()
                .map(|section| groups::Section {
                    id: section.id,
                    name: section.title,
                    songs: section.songs,
                })
                .collect(),
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

#[derive(serde::Serialize)]
struct BibleSyncRow {
    path: String,
    short_name: String,
    title: String,
    content_hash: String,
    server_id: String,
    uploaded: bool,
}

#[tauri::command]
fn bible_module_links() -> Result<Vec<BibleSyncRow>, String> {
    let (_, cfg) = load_config()?;
    let mut rows: Vec<BibleSyncRow> = cfg
        .bibles
        .iter()
        .map(|(hash, bind)| BibleSyncRow {
            path: bind.path.clone(),
            short_name: bind.short_name.clone(),
            title: bind.title.clone(),
            content_hash: hash.clone(),
            server_id: bind.server_id.clone(),
            uploaded: false,
        })
        .collect();
    rows.sort_by(|left, right| left.short_name.cmp(&right.short_name));
    Ok(rows)
}

#[tauri::command]
async fn sync_bible_modules() -> Result<Vec<BibleSyncRow>, String> {
    let (dir, mut cfg) = load_config()?;
    require_login(&cfg)?;
    if cfg.visio_path.is_empty() {
        return Err("Сначала выберите папку VisioBible".into());
    }
    let scan = visio::open_location(Path::new(&cfg.visio_path))?;
    if scan.needs_choice {
        return Err("Найдено несколько папок. Выберите одну.".into());
    }
    let client = Remote::new(&cfg.server_url, &cfg.token)?;
    let mut rows = Vec::new();
    for bible in &scan.bibles {
        let module = bible::identity(Path::new(&bible.path))?;
        let found = client
            .module_exists(&module.short_name, &module.title, &module.content_hash)
            .await?;
        let (server_id, uploaded) = if bible::should_upload(found.exists, &found.id) {
            let saved = client.upload_module(&module).await?;
            if saved.id.is_empty() || !module_reply_matches(&saved, &module) {
                return Err("Сервер не вернул id модуля".into());
            }
            (saved.id, saved.uploaded)
        } else if !module_reply_matches(&found, &module) {
            return Err("Сервер вернул другой модуль".into());
        } else {
            (found.id, false)
        };
        cfg.bibles.insert(
            module.content_hash.clone(),
            config::BibleBind {
                server_id: server_id.clone(),
                short_name: module.short_name.clone(),
                title: module.title.clone(),
                path: module.dir.display().to_string(),
            },
        );
        config::save(&dir, &cfg)?;
        rows.push(BibleSyncRow {
            path: module.dir.display().to_string(),
            short_name: module.short_name,
            title: module.title,
            content_hash: module.content_hash,
            server_id,
            uploaded,
        });
    }
    Ok(rows)
}

pub fn apply_pending_update() -> bool {
    portable::apply_pending_update()
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

#[tauri::command]
async fn ai_http(
    method: String,
    path: String,
    body: Option<String>,
) -> Result<ai_http::Reply, String> {
    let (_, cfg) = load_config()?;
    if cfg.token.is_empty() {
        return Ok(ai_http::Reply {
            status: 401,
            body: r#"{"error":"не авторизован"}"#.into(),
            chunks: 0,
        });
    }
    ai_http::exchange(&cfg.server_url, &cfg.token, &method, &path, body).await
}

#[tauri::command]
async fn ai_http_stream(
    app: tauri::AppHandle,
    id: String,
    method: String,
    path: String,
    body: Option<String>,
) -> Result<ai_http::Reply, String> {
    let (_, cfg) = load_config()?;
    if cfg.token.is_empty() {
        return Ok(ai_http::Reply {
            status: 401,
            body: r#"{"error":"не авторизован"}"#.into(),
            chunks: 0,
        });
    }
    ai_http::stream(app, &cfg.server_url, &cfg.token, &id, &method, &path, body).await
}

#[tauri::command]
fn ai_http_cancel(id: String) {
    ai_http::cancel(&id);
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
            portable::check_update,
            portable::download_update,
            portable::install_update,
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
            bible_module_links,
            sync_bible_modules,
            ai_http,
            ai_http_stream,
            ai_http_cancel,
        ])
        .run(tauri::generate_context!())
        .expect("окно не открылось");
}
