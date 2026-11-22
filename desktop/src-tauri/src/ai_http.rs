use std::sync::Mutex;
use std::time::Duration;

use tauri::Emitter;

use crate::remote;

static CANCEL: Mutex<Vec<String>> = Mutex::new(Vec::new());

#[derive(Clone, serde::Serialize)]
pub struct Reply {
    pub status: u16,
    pub body: String,
    pub chunks: u32,
}

#[derive(Clone, serde::Serialize)]
struct SseEvent {
    id: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    status: Option<u16>,
    #[serde(skip_serializing_if = "Option::is_none")]
    chunk: Option<String>,
}

pub fn cancel(id: &str) {
    if let Ok(mut list) = CANCEL.lock() {
        if !list.iter().any(|item| item == id) {
            list.push(id.to_string());
        }
    }
}

fn cancelled(id: &str) -> bool {
    CANCEL
        .lock()
        .ok()
        .is_some_and(|list| list.iter().any(|item| item == id))
}

fn clear_cancel(id: &str) {
    if let Ok(mut list) = CANCEL.lock() {
        list.retain(|item| item != id);
    }
}

pub async fn exchange(
    base: &str,
    token: &str,
    method: &str,
    path: &str,
    body: Option<String>,
) -> Result<Reply, String> {
    check_chat_path(method, path)?;
    let method = parse_method(method)?;
    let response = match send(base, token, method, path, body, Duration::from_secs(30)).await {
        Ok(response) => response,
        Err(reply) => return Ok(reply),
    };
    let status = response.status().as_u16();
    let text = response.text().await.unwrap_or_default();
    Ok(Reply {
        status,
        body: text,
        chunks: 0,
    })
}

pub async fn stream(
    app: tauri::AppHandle,
    base: &str,
    token: &str,
    id: &str,
    method: &str,
    path: &str,
    body: Option<String>,
) -> Result<Reply, String> {
    check_id(id)?;
    let method = parse_method(method)?;
    check_ai_path(path)?;
    clear_cancel(id);
    let mut response = match send(base, token, method, path, body, Duration::from_secs(300)).await {
        Ok(response) => response,
        Err(reply) => return Ok(reply),
    };
    let status = response.status().as_u16();
    emit(
        &app,
        SseEvent {
            id: id.to_string(),
            status: Some(status),
            chunk: None,
        },
    )?;
    if !(200..300).contains(&status) {
        let text = response.text().await.unwrap_or_default();
        clear_cancel(id);
        return Ok(Reply {
            status,
            body: text,
            chunks: 0,
        });
    }
    let mut pending = Vec::new();
    let mut chunks = 0u32;
    loop {
        if cancelled(id) {
            break;
        }
        let next = response.chunk().await;
        let Some(bytes) = next.unwrap_or(None) else {
            break;
        };
        pending.extend_from_slice(&bytes);
        let text = drain_text(&mut pending);
        if text.is_empty() {
            continue;
        }
        chunks += 1;
        emit(
            &app,
            SseEvent {
                id: id.to_string(),
                status: None,
                chunk: Some(text),
            },
        )?;
    }
    if !pending.is_empty() {
        let text = String::from_utf8_lossy(&pending).into_owned();
        if !text.is_empty() {
            chunks += 1;
            emit(
                &app,
                SseEvent {
                    id: id.to_string(),
                    status: None,
                    chunk: Some(text),
                },
            )?;
        }
    }
    clear_cancel(id);
    Ok(Reply {
        status,
        body: String::new(),
        chunks,
    })
}

fn emit(app: &tauri::AppHandle, event: SseEvent) -> Result<(), String> {
    app.emit("ai-sse", event).map_err(|err| err.to_string())
}

async fn send(
    base: &str,
    token: &str,
    method: reqwest::Method,
    path: &str,
    body: Option<String>,
    timeout: Duration,
) -> Result<reqwest::Response, Reply> {
    let http = remote::http_client_direct(timeout).map_err(|_| offline())?;
    let url = format!("{base}{path}");
    let mut request = http.request(method, &url).bearer_auth(token);
    if let Some(body) = body {
        request = request
            .header(reqwest::header::CONTENT_TYPE, "application/json")
            .body(body);
    }
    request.send().await.map_err(|_| offline())
}

fn offline() -> Reply {
    Reply {
        status: 502,
        body: r#"{"error":"Нет связи с сервером."}"#.to_string(),
        chunks: 0,
    }
}

fn parse_method(raw: &str) -> Result<reqwest::Method, String> {
    match raw {
        "GET" => Ok(reqwest::Method::GET),
        "POST" => Ok(reqwest::Method::POST),
        "PATCH" => Ok(reqwest::Method::PATCH),
        "PUT" => Ok(reqwest::Method::PUT),
        "DELETE" => Ok(reqwest::Method::DELETE),
        _ => Err("Запрос чата не подходит".into()),
    }
}

pub fn check_ai_path(path: &str) -> Result<(), String> {
    safe_path(path)?;
    let bare = path.split('?').next().unwrap_or(path);
    if bare.starts_with("/api/ai/") {
        return Ok(());
    }
    Err("Запрос чата не подходит".into())
}

/// Чат и чтение текста песни или стиха. Запись — только в `/api/ai/`.
pub fn check_chat_path(method: &str, path: &str) -> Result<(), String> {
    safe_path(path)?;
    let bare = path.split('?').next().unwrap_or(path);
    if bare.starts_with("/api/ai/") {
        return Ok(());
    }
    if method == "GET" && (bare == "/api/bible/semantic" || snapshot_path(bare)) {
        return Ok(());
    }
    Err("Запрос чата не подходит".into())
}

fn safe_path(path: &str) -> Result<(), String> {
    if path.len() > 500 || path.contains("..") || path.contains('\\') || path.contains("//") {
        return Err("Запрос чата не подходит".into());
    }
    let ok = path.chars().all(|ch| {
        ch.is_ascii_alphanumeric()
            || matches!(ch, '/' | '?' | '&' | '=' | '-' | '_' | '.' | '%' | '~')
    });
    if !ok {
        return Err("Запрос чата не подходит".into());
    }
    Ok(())
}

fn snapshot_path(path: &str) -> bool {
    let Some(rest) = path.strip_prefix("/dimensions/") else {
        return false;
    };
    let Some((dim, rest)) = rest.split_once("/songbooks/") else {
        return false;
    };
    let Some((book, tail)) = rest.split_once('/') else {
        return false;
    };
    tail == "snapshot" && is_uuid(dim) && is_uuid(book)
}

fn is_uuid(value: &str) -> bool {
    let bytes = value.as_bytes();
    if bytes.len() != 36 {
        return false;
    }
    let groups = [8usize, 4, 4, 4, 12];
    let mut at = 0;
    for (index, len) in groups.iter().enumerate() {
        if index > 0 {
            if bytes[at] != b'-' {
                return false;
            }
            at += 1;
        }
        for _ in 0..*len {
            if !bytes[at].is_ascii_hexdigit() {
                return false;
            }
            at += 1;
        }
    }
    at == 36
}

fn check_id(id: &str) -> Result<(), String> {
    if (8..=80).contains(&id.len()) && id.chars().all(|ch| ch.is_ascii_alphanumeric() || ch == '-')
    {
        Ok(())
    } else {
        Err("Запрос чата не подходит".into())
    }
}

fn drain_text(buf: &mut Vec<u8>) -> String {
    match std::str::from_utf8(buf) {
        Ok(text) => {
            let out = text.to_string();
            buf.clear();
            out
        }
        Err(err) => {
            let valid = err.valid_up_to();
            if valid == 0 {
                if err.error_len().is_some() {
                    buf.drain(..1);
                }
                return String::new();
            }
            let out = std::str::from_utf8(&buf[..valid]).unwrap_or("").to_string();
            buf.drain(..valid);
            out
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn path_allows_chat_query_only() {
        assert!(check_ai_path("/api/ai/chats?q=%D0%BC").is_ok());
        assert!(check_ai_path("/api/ai/chats/abc/messages").is_ok());
        assert!(check_ai_path("/api/plans").is_err());
        assert!(check_ai_path("/api/ai/../plans").is_err());
        assert!(check_ai_path("/api/ai/chats?q=a b").is_err());
        assert!(check_ai_path("https://example.com/api/ai/chats").is_err());
        let book = "/dimensions/11111111-1111-4111-8111-111111111111/songbooks/22222222-2222-4222-8222-222222222222/snapshot";
        assert!(check_chat_path("GET", book).is_ok());
        assert!(check_chat_path("POST", book).is_err());
        assert!(check_chat_path("GET", "/api/bible/semantic?q=%D0%98%D0%BD").is_ok());
        assert!(check_chat_path("GET", "/api/plans").is_err());
        assert!(check_chat_path(
            "GET",
            "/dimensions/not-uuid/songbooks/22222222-2222-4222-8222-222222222222/snapshot"
        )
        .is_err());
    }

    #[test]
    fn utf8_split_keeps_a_partial_character() {
        let mut buf = vec![0xd0];
        assert!(drain_text(&mut buf).is_empty());
        buf.push(0x9f);
        assert_eq!(drain_text(&mut buf), "П");
    }
}
