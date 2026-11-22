use std::fs;
use std::path::{Path, PathBuf};

use serde::{Deserialize, Serialize};

use crate::config;
use crate::paths;
use crate::remote::{self, Remote};
use crate::visio;

/// Пункт плана в ответе `GET /api/plans`. Подписи сборника сервер отдаёт сам и не хранит их в записи.
#[derive(Clone, Debug, Default, PartialEq, Serialize, Deserialize)]
pub struct ServerItem {
    #[serde(default, skip_serializing_if = "String::is_empty")]
    id: String,
    #[serde(default)]
    kind: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    song_id: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    songbook_id: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    title: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    module_short_name: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    module_short_name_second: String,
    #[serde(default)]
    book: i32,
    #[serde(default)]
    chapter: i32,
    #[serde(default)]
    verse: i32,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    caption: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    reference: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    quote: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    class: String,
    #[serde(default)]
    position: i32,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    created_at: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    updated_at: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    songbook_title: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    section_title: String,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
pub struct ServerPlan {
    #[serde(default)]
    id: String,
    #[serde(default)]
    owner_user_id: Option<String>,
    #[serde(default)]
    dimension_id: Option<String>,
    #[serde(default)]
    title: String,
    #[serde(default)]
    version: i64,
    #[serde(default)]
    updated_at: String,
    #[serde(default)]
    deleted: bool,
    #[serde(default)]
    items: Vec<ServerItem>,
}

#[derive(Serialize)]
pub struct PlanResult {
    plan: ServerPlan,
    conflict: bool,
}

#[derive(Deserialize)]
struct ConflictBody {
    #[serde(default)]
    error: String,
    #[serde(default)]
    plan: Option<ServerPlan>,
}

#[derive(Serialize, Deserialize)]
pub struct ImportedPlan {
    #[serde(flatten)]
    plan: ServerPlan,
    #[serde(default)]
    mojibake: bool,
    #[serde(default)]
    encoding_warning: String,
    #[serde(default)]
    suggested_text: String,
    #[serde(default)]
    suggested_title: String,
    #[serde(default)]
    suggested_items: Vec<ServerItem>,
}

#[derive(Default, Serialize, Deserialize)]
pub struct EncodingNotice {
    #[serde(default)]
    mojibake: bool,
    #[serde(default)]
    encoding_warning: String,
    #[serde(default)]
    suggested_text: String,
    #[serde(default)]
    suggested_title: String,
    #[serde(default)]
    suggested_items: Vec<ServerItem>,
}

#[derive(Serialize, Deserialize)]
pub struct ParsedPlan {
    #[serde(default)]
    title: String,
    #[serde(default)]
    items: Vec<ServerItem>,
    #[serde(default)]
    encoding_notice: EncodingNotice,
}

#[derive(Serialize)]
pub struct FolderPlan {
    title: String,
    server_id: String,
}

#[derive(Deserialize, Serialize)]
pub struct TreeSection {
    #[serde(default)]
    id: String,
    #[serde(default)]
    title: String,
    #[serde(default)]
    count: i32,
}

#[derive(Deserialize, Serialize)]
pub struct TreeBook {
    #[serde(default)]
    id: String,
    #[serde(default)]
    title: String,
    #[serde(default)]
    short_name: String,
    #[serde(default)]
    sections: Vec<TreeSection>,
}

#[derive(Deserialize, Serialize)]
pub struct TreeGroup {
    #[serde(default)]
    group: String,
    #[serde(default)]
    name: String,
    #[serde(default)]
    songbooks: Vec<TreeBook>,
}

#[derive(Deserialize, Serialize)]
pub struct FoundSong {
    #[serde(default)]
    songbook_id: String,
    #[serde(default)]
    fbid: String,
    #[serde(default)]
    title: String,
    #[serde(default)]
    songbook_title: String,
    #[serde(default)]
    section_title: String,
    #[serde(default)]
    status: String,
}

#[derive(Deserialize)]
struct SongList {
    #[serde(default)]
    songs: Vec<FoundSong>,
}

fn signed_client() -> Result<Remote, String> {
    let (_, cfg) = load()?;
    if cfg.token.is_empty() {
        return Err("Сначала войдите в учётную запись".into());
    }
    Remote::new(&cfg.server_url, &cfg.token)
}

fn load() -> Result<(PathBuf, config::Config), String> {
    let dir = paths::config_dir()?;
    let cfg = config::load(&dir)?;
    Ok((dir, cfg))
}

fn decode<T: for<'de> Deserialize<'de>>(status: u16, text: &str) -> Result<T, String> {
    if !(200..300).contains(&status) {
        return Err(remote::parse_failure(status, text).to_string());
    }
    serde_json::from_str(text).map_err(|err| format!("Ответ сервера не разобран: {err}"))
}

fn decode_save(status: u16, text: &str) -> Result<PlanResult, String> {
    if status == 409 {
        let body: ConflictBody = serde_json::from_str(text)
            .map_err(|_| remote::parse_failure(status, text).to_string())?;
        if let (true, Some(plan)) = (body.error == "конфликт", body.plan) {
            return Ok(PlanResult {
                plan,
                conflict: true,
            });
        }
        return Err(remote::parse_failure(status, text).to_string());
    }
    Ok(PlanResult {
        plan: decode(status, text)?,
        conflict: false,
    })
}

fn export_query(id: &str, format: &str, encoding: &str) -> String {
    let mut path = format!(
        "/api/plans/{}/export?format={}",
        remote::urlencoding_min(id),
        remote::urlencoding_min(format)
    );
    if !encoding.is_empty() && format != "json" {
        path.push_str("&encoding=");
        path.push_str(&remote::urlencoding_min(encoding));
    }
    path
}

fn song_query(query: &str, songbook_id: &str, section_id: &str) -> String {
    let mut path = format!(
        "/api/songs/search?q={}&limit=20&offset=0",
        remote::urlencoding_min(query.trim())
    );
    if !songbook_id.is_empty() {
        path.push_str("&songbook_id=");
        path.push_str(&remote::urlencoding_min(songbook_id));
    }
    if !section_id.is_empty() {
        path.push_str("&section_id=");
        path.push_str(&remote::urlencoding_min(section_id));
    }
    path
}

fn canonical_file(path: &Path) -> Option<PathBuf> {
    if path.exists() {
        return path.canonicalize().ok();
    }
    let parent = path.parent()?;
    let name = path.file_name()?;
    Some(parent.canonicalize().ok()?.join(name))
}

/// Скачанный план нельзя положить в каталог VisioBible: туда пишет только синхронизация.
fn export_allowed(dest: &Path, roots: &[PathBuf]) -> Result<(), String> {
    if dest.as_os_str().is_empty() || dest.file_name().is_none() {
        return Err("Укажите файл".into());
    }
    let Some(dest) = canonical_file(dest) else {
        return Ok(());
    };
    for root in roots {
        if root.as_os_str().is_empty() || !root.exists() {
            continue;
        }
        let Ok(root) = root.canonicalize() else {
            continue;
        };
        if dest.starts_with(&root) {
            return Err(
                "Этот путь внутри папки VisioBible. Скачанный план сохраняется отдельно, папку меняет только синхронизация.".into(),
            );
        }
    }
    Ok(())
}

fn visio_roots(visio_path: &str) -> Vec<PathBuf> {
    if visio_path.is_empty() {
        return Vec::new();
    }
    let mut roots = vec![PathBuf::from(visio_path)];
    if let Ok(scan) = visio::open_location(Path::new(visio_path)) {
        for path in [scan.root, scan.modules_dir, scan.plans_dir] {
            if !path.is_empty() {
                roots.push(PathBuf::from(path));
            }
        }
    }
    roots
}

#[tauri::command]
pub async fn plans_list() -> Result<Vec<ServerPlan>, String> {
    let client = signed_client()?;
    let reply = client
        .exchange(reqwest::Method::GET, "/api/plans?scope=all", None::<&()>)
        .await?;
    decode(reply.status, &reply.text)
}

#[tauri::command]
pub async fn plans_read(id: String) -> Result<ServerPlan, String> {
    let client = signed_client()?;
    let path = format!("/api/plans/{}", remote::urlencoding_min(&id));
    let reply = client
        .exchange(reqwest::Method::GET, &path, None::<&()>)
        .await?;
    decode(reply.status, &reply.text)
}

/// Полный текст песни для просмотра пункта плана (GET /api/songs/text).
#[tauri::command]
pub async fn plans_song_text(
    songbook_id: String,
    fbid: String,
) -> Result<serde_json::Value, String> {
    let client = signed_client()?;
    let path = format!(
        "/api/songs/text?songbook_id={}&fbid={}",
        remote::urlencoding_min(&songbook_id),
        remote::urlencoding_min(&fbid)
    );
    let reply = client
        .exchange(reqwest::Method::GET, &path, None::<&()>)
        .await?;
    decode(reply.status, &reply.text)
}

/// Один стих перевода для просмотра пункта плана (GET /api/bible/verse).
#[tauri::command]
pub async fn plans_verse_text(
    module: String,
    book: u32,
    chapter: u32,
    verse: u32,
) -> Result<serde_json::Value, String> {
    let client = signed_client()?;
    let path = format!(
        "/api/bible/verse?module={}&book={book}&chapter={chapter}&verse={verse}",
        remote::urlencoding_min(&module)
    );
    let reply = client
        .exchange(reqwest::Method::GET, &path, None::<&()>)
        .await?;
    decode(reply.status, &reply.text)
}

#[tauri::command]
pub async fn plans_create(
    title: String,
    dimension_id: String,
    items: Vec<ServerItem>,
) -> Result<ServerPlan, String> {
    let client = signed_client()?;
    let body = serde_json::json!({
        "title": title.trim(),
        "dimension_id": dimension_id,
        "items": items,
    });
    let reply = client
        .exchange(reqwest::Method::POST, "/api/plans", Some(&body))
        .await?;
    decode(reply.status, &reply.text)
}

#[derive(Serialize)]
struct WriteBody<'a> {
    title: &'a str,
    version: i64,
    #[serde(skip_serializing_if = "str::is_empty")]
    updated_at: &'a str,
    items: &'a [ServerItem],
}

#[tauri::command]
pub async fn plans_save(
    id: String,
    title: String,
    version: i64,
    updated_at: String,
    items: Vec<ServerItem>,
) -> Result<PlanResult, String> {
    let client = signed_client()?;
    let body = WriteBody {
        title: title.trim(),
        version,
        updated_at: &updated_at,
        items: &items,
    };
    let path = format!("/api/plans/{id}");
    let reply = client
        .exchange(reqwest::Method::PUT, &path, Some(&body))
        .await?;
    decode_save(reply.status, &reply.text)
}

#[tauri::command]
pub async fn plans_delete(
    id: String,
    version: i64,
    updated_at: String,
) -> Result<PlanResult, String> {
    let client = signed_client()?;
    let mut path = format!(
        "/api/plans/{}?version={version}",
        remote::urlencoding_min(&id)
    );
    if !updated_at.is_empty() {
        path.push_str("&updated_at=");
        path.push_str(&remote::urlencoding_min(&updated_at));
    }
    let reply = client
        .exchange(reqwest::Method::DELETE, &path, None::<&()>)
        .await?;
    decode_save(reply.status, &reply.text)
}

#[tauri::command]
pub async fn plans_import(
    format: String,
    payload: String,
    title: String,
    dimension_id: String,
) -> Result<ImportedPlan, String> {
    let client = signed_client()?;
    let body = serde_json::json!({
        "format": format,
        "payload": payload,
        "title": title,
        "dimension_id": dimension_id,
    });
    let reply = client
        .exchange(reqwest::Method::POST, "/api/plans/import", Some(&body))
        .await?;
    decode(reply.status, &reply.text)
}

#[tauri::command]
pub async fn plans_parse(format: String, payload: String) -> Result<ParsedPlan, String> {
    let client = signed_client()?;
    let body = serde_json::json!({ "format": format, "payload": payload });
    let reply = client
        .exchange(reqwest::Method::POST, "/api/plans/parse", Some(&body))
        .await?;
    decode(reply.status, &reply.text)
}

#[tauri::command]
pub async fn plans_download(
    id: String,
    format: String,
    encoding: String,
    path: String,
) -> Result<(), String> {
    let (_, cfg) = load()?;
    if cfg.token.is_empty() {
        return Err("Сначала войдите в учётную запись".into());
    }
    let dest = PathBuf::from(&path);
    export_allowed(&dest, &visio_roots(&cfg.visio_path))?;
    let client = Remote::new(&cfg.server_url, &cfg.token)?;
    let bytes = client
        .get_bytes(&export_query(&id, &format, &encoding))
        .await?;
    if let Some(parent) = dest.parent() {
        if !parent.as_os_str().is_empty() && !parent.exists() {
            return Err("Папка для файла не найдена".into());
        }
    }
    fs::write(&dest, bytes).map_err(|err| err.to_string())
}

#[tauri::command]
pub fn plans_folder() -> Result<Vec<FolderPlan>, String> {
    let (_, cfg) = load()?;
    if cfg.visio_path.is_empty() {
        return Ok(Vec::new());
    }
    let scan = match visio::open_location(Path::new(&cfg.visio_path)) {
        Ok(scan) if !scan.needs_choice => scan,
        _ => return Ok(Vec::new()),
    };
    let mut out = Vec::new();
    for info in scan.plans {
        let server_id = cfg
            .plans
            .get(&info.path)
            .map(|bind| bind.server_id.clone())
            .unwrap_or_default();
        out.push(FolderPlan {
            title: info.name,
            server_id,
        });
    }
    Ok(out)
}

#[tauri::command]
pub async fn plans_songbooks() -> Result<Vec<TreeGroup>, String> {
    let client = signed_client()?;
    let reply = client
        .exchange(reqwest::Method::GET, "/api/songbooks/tree", None::<&()>)
        .await?;
    decode(reply.status, &reply.text)
}

#[tauri::command]
pub async fn plans_find_songs(
    query: String,
    songbook_id: String,
    section_id: String,
) -> Result<Vec<FoundSong>, String> {
    let query = query.trim().to_string();
    if query.is_empty() {
        return Err("Введите слово из названия или текста песни.".into());
    }
    let client = signed_client()?;
    let path = song_query(&query, &songbook_id, &section_id);
    let reply = client
        .exchange(reqwest::Method::GET, &path, None::<&()>)
        .await?;
    let list: SongList = decode(reply.status, &reply.text)?;
    Ok(list.songs)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn conflict_keeps_the_songbook_title() {
        let text = r#"{"error":"конфликт","plan":{"id":"11111111-1111-4111-8111-111111111111","title":"Воскресенье","version":4,"items":[{"kind":"song","title":"Ты мой Бог","songbook_title":"Песнь Возрождения"}]}}"#;
        let result = decode_save(409, text).unwrap();
        assert!(result.conflict);
        assert_eq!(result.plan.version, 4);
        assert_eq!(result.plan.items[0].songbook_title, "Песнь Возрождения");
    }

    #[test]
    fn blank_timestamp_is_not_sent_back() {
        let item = ServerItem {
            kind: "text".into(),
            caption: "Объявление".into(),
            quote: "Чай".into(),
            ..ServerItem::default()
        };
        let json = serde_json::to_value(&item).unwrap();
        assert!(json.get("created_at").is_none());
        assert_eq!(json["caption"], "Объявление");
    }

    #[test]
    fn export_query_skips_encoding_for_json() {
        assert_eq!(
            export_query("abc", "vplt", "windows-1251"),
            "/api/plans/abc/export?format=vplt&encoding=windows-1251"
        );
        assert_eq!(
            export_query("abc", "json", "windows-1251"),
            "/api/plans/abc/export?format=json"
        );
    }

    #[test]
    fn download_refuses_a_file_inside_visio() {
        let visio = paths::TmpDir::new("visio");
        let outside = paths::TmpDir::new("outside");
        let plans = visio.path.join("Plans");
        fs::create_dir_all(&plans).unwrap();
        let inside = plans.join("plan.vplt");
        let elsewhere = outside.path.join("plan.vplt");
        let roots = std::slice::from_ref(&visio.path);
        assert!(export_allowed(&inside, roots).is_err());
        assert!(export_allowed(&elsewhere, roots).is_ok());
        assert!(export_allowed(Path::new(""), roots).is_err());
    }
}
