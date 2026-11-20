use std::time::Duration;

use serde::{Deserialize, Serialize};

use crate::plans::{PlanItem, RemotePlan};
use crate::visio::Song;

#[derive(Debug)]
pub enum RemoteError {
    Message(String),
    PlanConflict(Box<RemotePlan>),
}

impl std::fmt::Display for RemoteError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::Message(text) => write!(f, "{text}"),
            Self::PlanConflict(plan) => write!(f, "План «{}» на сервере уже изменился", plan.title),
        }
    }
}

pub type RemoteResult<T> = Result<T, RemoteError>;

#[derive(Clone, Debug, Deserialize)]
pub struct AuthReply {
    pub user_id: String,
    #[serde(default)]
    pub device_id: String,
    #[serde(default)]
    pub dimension_id: String,
    pub token: String,
    #[serde(default)]
    pub role: String,
}

#[derive(Clone, Debug, Deserialize)]
pub struct Me {
    pub user_id: String,
    #[serde(default)]
    pub email: String,
    #[serde(default)]
    pub role: String,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct Dimension {
    pub id: String,
    #[serde(default)]
    pub kind: String,
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub address: String,
    #[serde(default)]
    pub email: String,
    #[serde(default)]
    pub confession: String,
    #[serde(default)]
    pub description: String,
}

#[derive(Clone, Debug, Deserialize)]
pub struct SongbookView {
    pub id: String,
    #[serde(default)]
    pub fbid: String,
    #[serde(default)]
    pub title: String,
    #[serde(default)]
    pub short_name: String,
    #[serde(default)]
    pub publisher: String,
    #[serde(default)]
    pub revision_date: String,
    #[serde(default)]
    pub comment: String,
    #[serde(default)]
    pub header: HeaderTemplate,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
pub struct HeaderTemplate {
    #[serde(default)]
    pub format_version: String,
    #[serde(default)]
    pub flags: u32,
    #[serde(default)]
    pub charset: String,
    #[serde(default)]
    pub header_short: String,
    #[serde(default)]
    pub header_title: String,
}

#[derive(Clone, Debug, Deserialize)]
pub struct Snapshot {
    pub cursor: i64,
    #[serde(default)]
    pub songs: Vec<Song>,
    #[serde(default)]
    #[allow(dead_code)]
    pub order: Vec<String>,
}

#[derive(Clone, Debug, Serialize)]
pub struct PushOp {
    pub operation_id: String,
    pub song: Song,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub order: Option<Vec<String>>,
}

#[derive(Clone, Debug, Deserialize)]
pub struct OpResult {
    #[serde(default)]
    #[allow(dead_code)]
    pub operation_id: String,
    #[serde(default)]
    pub status: String,
    #[serde(default)]
    pub revision: i64,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct SongHit {
    #[serde(default)]
    pub songbook_id: String,
    #[serde(default)]
    pub fbid: String,
    #[serde(default)]
    pub title: String,
    #[serde(default)]
    pub songbook_title: String,
    #[serde(default)]
    pub dimension_id: String,
    #[serde(default)]
    pub status: String,
}

#[derive(Clone, Debug, Deserialize)]
pub struct ReleaseInfo {
    pub version: String,
    #[serde(default)]
    pub status: String,
    #[serde(default)]
    pub linux_sha256: String,
    #[serde(default)]
    pub linux_size: i64,
    #[serde(default)]
    pub windows_sha256: String,
    #[serde(default)]
    pub windows_size: i64,
    #[serde(default)]
    pub macos_sha256: String,
    #[serde(default)]
    pub macos_size: i64,
    #[serde(default)]
    pub notes: String,
}

impl ReleaseInfo {
    pub fn file(&self, platform: &str) -> Option<ReleaseFile> {
        let (sha, size) = match platform {
            "linux" => (&self.linux_sha256, self.linux_size),
            "windows" => (&self.windows_sha256, self.windows_size),
            "macos" => (&self.macos_sha256, self.macos_size),
            _ => return None,
        };
        if sha.len() != 64 || size <= 0 {
            return None;
        }
        Some(ReleaseFile {
            platform: platform.to_string(),
            sha256: sha.to_ascii_lowercase(),
            size,
        })
    }
}

#[derive(Clone, Debug)]
pub struct ReleaseFile {
    pub platform: String,
    pub sha256: String,
    pub size: i64,
}

pub struct Remote {
    base: String,
    token: String,
    http: reqwest::Client,
}

impl Remote {
    pub fn new(base: &str, token: &str) -> Result<Self, String> {
        Ok(Self {
            base: parse_server(base)?,
            token: token.to_string(),
            http: client(Duration::from_secs(30))?,
        })
    }

    pub fn download_client(base: &str) -> Result<Self, String> {
        Ok(Self {
            base: parse_server(base)?,
            token: String::new(),
            http: client(Duration::from_secs(300))?,
        })
    }

    pub async fn register(
        &self,
        email: &str,
        password: &str,
        device_id: &str,
    ) -> Result<AuthReply, String> {
        self.auth("/register", email, password, device_id).await
    }

    pub async fn login(
        &self,
        email: &str,
        password: &str,
        device_id: &str,
    ) -> Result<AuthReply, String> {
        self.auth("/login", email, password, device_id).await
    }

    async fn auth(
        &self,
        path: &str,
        email: &str,
        password: &str,
        device_id: &str,
    ) -> Result<AuthReply, String> {
        let mut body = serde_json::json!({ "email": email, "password": password });
        if !device_id.is_empty() {
            body["device_id"] = serde_json::json!(device_id);
        }
        self.send(reqwest::Method::POST, path, Some(&body))
            .await
            .map_err(|err| err.to_string())
    }

    pub async fn logout(&self) -> Result<(), String> {
        let _: serde_json::Value = self
            .send(reqwest::Method::POST, "/logout", None::<&()>)
            .await
            .map_err(|err| err.to_string())?;
        Ok(())
    }

    pub async fn me(&self) -> Result<Me, String> {
        self.send(reqwest::Method::GET, "/api/me", None::<&()>)
            .await
            .map_err(|err| err.to_string())
    }

    pub async fn dimensions(&self) -> Result<Vec<Dimension>, String> {
        self.send(reqwest::Method::GET, "/api/dimensions", None::<&()>)
            .await
            .map_err(|err| err.to_string())
    }

    pub async fn communities(&self) -> Result<Vec<Dimension>, String> {
        self.send(reqwest::Method::GET, "/api/communities", None::<&()>)
            .await
            .map_err(|err| err.to_string())
    }

    pub async fn create_community(&self, name: &str) -> Result<Dimension, String> {
        let body = serde_json::json!({ "name": name });
        self.send(reqwest::Method::POST, "/api/communities", Some(&body))
            .await
            .map_err(|err| err.to_string())
    }

    pub async fn songbooks(&self, dimension_id: &str) -> Result<Vec<SongbookView>, String> {
        let path = format!("/dimensions/{dimension_id}/songbooks");
        self.send(reqwest::Method::GET, &path, None::<&()>)
            .await
            .map_err(|err| err.to_string())
    }

    pub async fn create_songbook(
        &self,
        dimension_id: &str,
        book: &NewSongbook<'_>,
    ) -> Result<SongbookView, String> {
        let path = format!("/dimensions/{dimension_id}/songbooks");
        self.send(reqwest::Method::POST, &path, Some(book))
            .await
            .map_err(|err| err.to_string())
    }

    pub async fn snapshot(&self, dimension_id: &str, book_id: &str) -> Result<Snapshot, String> {
        let path = format!("/dimensions/{dimension_id}/songbooks/{book_id}/snapshot");
        self.send(reqwest::Method::GET, &path, None::<&()>)
            .await
            .map_err(|err| err.to_string())
    }

    pub async fn push(
        &self,
        dimension_id: &str,
        book_id: &str,
        base_revision: i64,
        operations: &[PushOp],
    ) -> Result<Vec<OpResult>, String> {
        let path = format!("/dimensions/{dimension_id}/songbooks/{book_id}/push");
        let body = serde_json::json!({ "base_revision": base_revision, "operations": operations });
        let response: PushResponse = self
            .send(reqwest::Method::POST, &path, Some(&body))
            .await
            .map_err(|err| err.to_string())?;
        Ok(response.results)
    }

    pub async fn plans(&self, scope: &str) -> RemoteResult<Vec<RemotePlan>> {
        let path = format!("/api/plans?scope={scope}");
        self.send(reqwest::Method::GET, &path, None::<&()>).await
    }

    pub async fn create_plan(
        &self,
        title: &str,
        dimension_id: &str,
        items: &[PlanItem],
    ) -> RemoteResult<RemotePlan> {
        let body =
            serde_json::json!({ "title": title, "dimension_id": dimension_id, "items": items });
        self.send(reqwest::Method::POST, "/api/plans", Some(&body))
            .await
    }

    pub async fn push_plan(
        &self,
        id: &str,
        title: &str,
        version: i64,
        updated_at: &str,
        items: &[PlanItem],
    ) -> RemoteResult<RemotePlan> {
        let mut body = serde_json::json!({ "title": title, "version": version, "items": items });
        if !updated_at.is_empty() {
            body["updated_at"] = serde_json::json!(updated_at);
        }
        let path = format!("/api/plans/{id}/push");
        self.send(reqwest::Method::POST, &path, Some(&body)).await
    }

    pub async fn pull_plan(
        &self,
        id: &str,
        version: i64,
        updated_at: &str,
    ) -> RemoteResult<PlanPull> {
        let mut path = format!("/api/plans/{id}/pull?version={version}");
        if !updated_at.is_empty() {
            path.push_str("&updated_at=");
            path.push_str(&urlencoding_min(updated_at));
        }
        self.send(reqwest::Method::GET, &path, None::<&()>).await
    }

    pub async fn delete_plan(&self, id: &str, version: i64) -> RemoteResult<RemotePlan> {
        let path = format!("/api/plans/{id}?version={version}");
        self.send(reqwest::Method::DELETE, &path, None::<&()>).await
    }

    pub async fn export_plan(&self, id: &str, format: &str) -> Result<String, String> {
        let path = format!("/api/plans/{id}/export?format={format}");
        let url = format!("{}{path}", self.base);
        let response = self
            .http
            .get(&url)
            .bearer_auth(&self.token)
            .send()
            .await
            .map_err(|err| format!("Нет связи с сервером: {err}"))?;
        let status = response.status();
        let text = response.text().await.map_err(|err| err.to_string())?;
        if !status.is_success() {
            return Err(parse_failure(status.as_u16(), &text).to_string());
        }
        Ok(text)
    }

    pub async fn import_plan(
        &self,
        format: &str,
        payload: &str,
        title: &str,
        dimension_id: &str,
    ) -> RemoteResult<RemotePlan> {
        let body = serde_json::json!({
            "format": format,
            "payload": payload,
            "title": title,
            "dimension_id": dimension_id,
        });
        self.send(reqwest::Method::POST, "/api/plans/import", Some(&body))
            .await
    }

    pub async fn search_songs(
        &self,
        query: &str,
        dimension_id: &str,
    ) -> Result<Vec<SongHit>, String> {
        let path = format!(
            "/api/songs/search?q={}&dimension_id={}&limit=20&offset=0",
            urlencoding_min(query),
            urlencoding_min(dimension_id)
        );
        let response: SearchResponse = self
            .send(reqwest::Method::GET, &path, None::<&()>)
            .await
            .map_err(|err| err.to_string())?;
        Ok(response.songs)
    }

    pub async fn latest_release(&self) -> Result<ReleaseInfo, String> {
        self.send(reqwest::Method::GET, "/api/releases/latest", None::<&()>)
            .await
            .map_err(|err| err.to_string())
    }

    pub fn release_url(&self, version: &str, platform: &str) -> String {
        format!("{}/api/releases/{version}/download/{platform}", self.base)
    }

    pub async fn download_release(
        &self,
        version: &str,
        platform: &str,
    ) -> Result<Download, String> {
        let url = self.release_url(version, platform);
        let response = self
            .http
            .get(&url)
            .send()
            .await
            .map_err(|err| format!("Нет связи с сервером: {err}"))?;
        let status = response.status();
        let header = response
            .headers()
            .get("x-content-sha256")
            .and_then(|value| value.to_str().ok())
            .map(|value| value.to_string());
        if !status.is_success() {
            return Err(format!("Обновление не скачалось ({status})"));
        }
        let bytes = response
            .bytes()
            .await
            .map_err(|err| err.to_string())?
            .to_vec();
        Ok(Download {
            bytes,
            sha256: header.unwrap_or_default(),
        })
    }

    async fn send<T: for<'de> Deserialize<'de>, B: Serialize>(
        &self,
        method: reqwest::Method,
        path: &str,
        body: Option<&B>,
    ) -> RemoteResult<T> {
        let url = format!("{}{path}", self.base);
        let mut request = self.http.request(method, &url);
        if !self.token.is_empty() {
            request = request.bearer_auth(&self.token);
        }
        if let Some(body) = body {
            request = request.json(body);
        }
        let response = request
            .send()
            .await
            .map_err(|err| RemoteError::Message(format!("Нет связи с сервером: {err}")))?;
        let status = response.status();
        let text = response
            .text()
            .await
            .map_err(|err| RemoteError::Message(err.to_string()))?;
        if !status.is_success() {
            return Err(parse_failure(status.as_u16(), &text));
        }
        serde_json::from_str(&text)
            .map_err(|err| RemoteError::Message(format!("Ответ сервера не разобран: {err}")))
    }
}

#[derive(Clone, Debug)]
pub struct Download {
    pub bytes: Vec<u8>,
    pub sha256: String,
}

#[derive(Serialize)]
pub struct NewSongbook<'a> {
    pub fbid: &'a str,
    pub title: &'a str,
    pub short_name: &'a str,
    pub publisher: &'a str,
    pub revision_date: &'a str,
    pub comment: &'a str,
    pub header: &'a HeaderTemplate,
}

#[derive(Deserialize)]
struct PushResponse {
    #[serde(default)]
    results: Vec<OpResult>,
}

#[derive(Debug, Deserialize, Serialize)]
pub struct PlanPull {
    #[serde(default)]
    pub changed: bool,
    #[serde(default)]
    pub version: i64,
    #[serde(default)]
    pub updated_at: String,
    #[serde(default)]
    pub plan: Option<RemotePlan>,
}

#[derive(Deserialize)]
struct SearchResponse {
    #[serde(default)]
    songs: Vec<SongHit>,
}

#[derive(Deserialize)]
struct ErrorBody {
    #[serde(default)]
    error: String,
    #[serde(default)]
    plan: Option<RemotePlan>,
}

pub fn parse_server(raw: &str) -> Result<String, String> {
    let url = raw.trim().trim_end_matches('/').to_string();
    if url.len() > 200 || url.contains('@') || url.chars().any(|ch| ch.is_whitespace()) {
        return Err("Адрес сервера не подходит".into());
    }
    if !(url.starts_with("http://") || url.starts_with("https://")) {
        return Err("Адрес сервера должен начинаться с http:// или https://".into());
    }
    let rest = url
        .trim_start_matches("https://")
        .trim_start_matches("http://");
    if rest.is_empty() {
        return Err("Адрес сервера не подходит".into());
    }
    Ok(url)
}

pub fn parse_failure(status: u16, text: &str) -> RemoteError {
    if status == 409 {
        if let Ok(body) = serde_json::from_str::<ErrorBody>(text) {
            if body.error == "конфликт" {
                if let Some(plan) = body.plan {
                    return RemoteError::PlanConflict(Box::new(plan));
                }
            }
        }
    }
    let message = serde_json::from_str::<ErrorBody>(text)
        .ok()
        .map(|body| body.error)
        .filter(|error| !error.is_empty())
        .unwrap_or_else(|| match status {
            401 => "Нужно войти ещё раз".into(),
            403 => "Недостаточно прав".into(),
            404 => "На сервере это не найдено".into(),
            _ => "Сервер не принял запрос".into(),
        });
    RemoteError::Message(message)
}

fn client(timeout: Duration) -> Result<reqwest::Client, String> {
    let mut builder = reqwest::Client::builder().timeout(timeout);
    if let Ok(path) = std::env::var("FLAMMEBIBLE_CA") {
        if !path.is_empty() {
            let pem =
                std::fs::read(&path).map_err(|err| format!("Сертификат не прочитан: {err}"))?;
            let cert = reqwest::Certificate::from_pem(&pem)
                .map_err(|_| "Сертификат не подходит".to_string())?;
            builder = builder.add_root_certificate(cert);
        }
    }
    builder.build().map_err(|err| err.to_string())
}

fn urlencoding_min(value: &str) -> String {
    let mut out = String::new();
    for byte in value.bytes() {
        match byte {
            b'A'..=b'Z' | b'a'..=b'z' | b'0'..=b'9' | b'-' | b'_' | b'.' | b'~' => {
                out.push(byte as char)
            }
            _ => out.push_str(&format!("%{byte:02X}")),
        }
    }
    out
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn server_address_accepts_placeholders_only_as_http() {
        assert_eq!(
            parse_server("http://localhost:8080/").unwrap(),
            "http://localhost:8080"
        );
        assert_eq!(
            parse_server("https://example.com").unwrap(),
            "https://example.com"
        );
        assert!(parse_server("file:///tmp").is_err());
        assert!(parse_server("http://user:secret@localhost").is_err());
    }

    #[test]
    fn plan_conflict_keeps_the_server_copy() {
        let text = r#"{"error":"конфликт","plan":{"id":"11111111-1111-4111-8111-111111111111","title":"Воскресенье","version":4,"items":[]}}"#;
        match parse_failure(409, text) {
            RemoteError::PlanConflict(plan) => {
                assert_eq!(plan.version, 4);
                assert_eq!(plan.title, "Воскресенье");
            }
            other => panic!("{other}"),
        }
    }

    #[test]
    fn release_picks_the_platform_hash() {
        let info = ReleaseInfo {
            version: "0.2.0".into(),
            status: "released".into(),
            linux_sha256: "ab".repeat(32),
            linux_size: 10,
            windows_sha256: String::new(),
            windows_size: 0,
            macos_sha256: String::new(),
            macos_size: 0,
            notes: String::new(),
        };
        let file = info.file("linux").unwrap();
        assert_eq!(file.size, 10);
        assert!(info.file("windows").is_none());
    }
}
