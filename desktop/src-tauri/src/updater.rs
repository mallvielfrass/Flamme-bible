use std::fs::{self, File};
use std::io::{Read, Write};
use std::path::{Path, PathBuf};

use sha2::{Digest, Sha256};

pub fn current_version() -> &'static str {
    env!("CARGO_PKG_VERSION")
}

pub fn this_platform() -> &'static str {
    match std::env::consts::OS {
        "windows" => "windows",
        "macos" => "macos",
        _ => "linux",
    }
}

pub fn platform_label(platform: &str) -> &'static str {
    match platform {
        "windows" => "Windows",
        "macos" => "macOS",
        _ => "Linux",
    }
}

pub fn valid_version(version: &str) -> bool {
    !version.is_empty()
        && !version.contains("..")
        && version
            .chars()
            .all(|ch| ch.is_ascii_alphanumeric() || matches!(ch, '.' | '_' | '-' | '+'))
}

pub fn version_is_newer(current: &str, latest: &str) -> bool {
    let Some(current) = parts(current) else {
        return false;
    };
    let Some(latest) = parts(latest) else {
        return false;
    };
    latest > current
}

fn parts(value: &str) -> Option<(u64, u64, u64)> {
    let core = value
        .trim()
        .trim_start_matches('v')
        .split('-')
        .next()
        .unwrap_or(value);
    let mut numbers = core.split('.');
    let major = numbers.next()?.parse().ok()?;
    let minor = numbers.next()?.parse().ok()?;
    let patch = numbers.next()?.parse().ok()?;
    if numbers.next().is_some() {
        return None;
    }
    Some((major, minor, patch))
}

pub fn verify_hashes(got: &str, header_sha: &str, expected: &str) -> Result<(), String> {
    let expected = normalize_sha(expected)?;
    let header = normalize_sha(header_sha)?;
    let got = normalize_sha(got)?;
    if !ct_eq(&got, &expected) || !ct_eq(&header, &expected) {
        return Err("Файл обновления не совпал с контрольной суммой и не сохранён.".into());
    }
    Ok(())
}

pub fn hashes_equal(left: &str, right: &str) -> bool {
    match (normalize_sha(left), normalize_sha(right)) {
        (Ok(left), Ok(right)) => ct_eq(&left, &right),
        _ => false,
    }
}

pub fn hash_file(path: &Path) -> Result<String, String> {
    let mut file = File::open(path).map_err(|err| err.to_string())?;
    let mut hasher = Sha256::new();
    let mut buf = [0u8; 64 * 1024];
    loop {
        let read = file.read(&mut buf).map_err(|err| err.to_string())?;
        if read == 0 {
            break;
        }
        hasher.update(&buf[..read]);
    }
    Ok(format!("{:x}", hasher.finalize()))
}

pub fn percent(received: u64, total: u64) -> u32 {
    if total == 0 {
        return 0;
    }
    let value = received.saturating_mul(100) / total;
    u32::try_from(value).unwrap_or(100).min(100)
}

pub fn normalize_sha(value: &str) -> Result<String, String> {
    let lower = value.trim().to_ascii_lowercase();
    if lower.len() != 64 || !lower.bytes().all(|byte| byte.is_ascii_hexdigit()) {
        return Err("Контрольная сумма обновления не подходит".into());
    }
    Ok(lower)
}

fn ct_eq(left: &str, right: &str) -> bool {
    if left.len() != right.len() {
        return false;
    }
    let mut diff = 0u8;
    for (a, b) in left.bytes().zip(right.bytes()) {
        diff |= a ^ b;
    }
    diff == 0
}

pub fn update_path(dir: &Path, version: &str, platform: &str) -> Result<PathBuf, String> {
    if !valid_version(version) {
        return Err("Номер версии не подходит".into());
    }
    let name = match platform {
        "windows" => "flammebible-windows.exe",
        "macos" => "flammebible-macos",
        _ => "flammebible-linux",
    };
    Ok(dir.join("updates").join(version).join(name))
}

/// Writes the response to `dest`. On any error the partial file is removed.
/// Returns the `X-Content-SHA256` header. The caller checks it against the release hash.
pub async fn stream_to<F>(
    client: &reqwest::Client,
    url: &str,
    dest: &Path,
    expected_size: u64,
    mut on_progress: F,
) -> Result<String, String>
where
    F: FnMut(u64, u64) + Send,
{
    if expected_size == 0 {
        return Err("Размер файла обновления не совпал".into());
    }
    let mut response = client
        .get(url)
        .send()
        .await
        .map_err(|err| format!("Нет связи с сервером: {err}"))?;
    let status = response.status();
    let header = response
        .headers()
        .get("x-content-sha256")
        .and_then(|value| value.to_str().ok())
        .unwrap_or("")
        .to_string();
    if !status.is_success() {
        return Err(format!("Обновление не скачалось ({status})"));
    }
    if let Some(len) = response.content_length() {
        if len != expected_size {
            return Err("Размер файла обновления не совпал".into());
        }
    }
    if let Some(parent) = dest.parent() {
        if !parent.as_os_str().is_empty() {
            fs::create_dir_all(parent).map_err(|err| err.to_string())?;
        }
    }
    let written = write_body(&mut response, dest, expected_size, &mut on_progress).await;
    if written.is_err() {
        let _ = fs::remove_file(dest);
    }
    written?;
    Ok(header)
}

async fn write_body<F>(
    response: &mut reqwest::Response,
    dest: &Path,
    expected_size: u64,
    on_progress: &mut F,
) -> Result<(), String>
where
    F: FnMut(u64, u64) + Send,
{
    let mut file = File::create(dest).map_err(|err| err.to_string())?;
    let mut received = 0u64;
    while let Some(chunk) = response.chunk().await.map_err(|err| err.to_string())? {
        let len = u64::try_from(chunk.len()).unwrap_or(u64::MAX);
        received = received.saturating_add(len);
        if received > expected_size {
            return Err("Размер файла обновления не совпал".into());
        }
        file.write_all(&chunk).map_err(|err| err.to_string())?;
        on_progress(received, expected_size);
    }
    file.sync_all().map_err(|err| err.to_string())?;
    if received != expected_size {
        return Err("Размер файла обновления не совпал".into());
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::remote::ReleaseInfo;

    #[test]
    fn newer_version_and_hash() {
        assert!(version_is_newer("0.1.0", "0.1.1"));
        assert!(version_is_newer("0.1.9", "0.2.0"));
        assert!(!version_is_newer("0.2.0", "0.1.9"));
        assert!(!version_is_newer("0.1.0", "0.1.0"));
        let hash = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad";
        assert!(verify_hashes(hash, hash, hash).is_ok());
        assert!(verify_hashes(&"ab".repeat(32), hash, hash).is_err());
        assert!(verify_hashes(&"cd".repeat(32), hash, hash).is_err());
    }

    #[test]
    fn latest_release_exposes_linux_hash_and_size() {
        let info = ReleaseInfo {
            version: "0.2.0".into(),
            status: "released".into(),
            linux_sha256: "cd".repeat(32),
            linux_size: 42,
            windows_sha256: "ef".repeat(32),
            windows_size: 7,
            macos_sha256: String::new(),
            macos_size: 0,
            notes: "исправления".into(),
        };
        let file = info.file("linux").unwrap();
        assert_eq!(file.size, 42);
        assert_eq!(file.sha256.len(), 64);
        assert!(version_is_newer(current_version(), &info.version) || current_version() == "0.1.0");
    }

    #[test]
    fn hash_file_matches_bytes_and_percent_fills() {
        let dir = crate::paths::TmpDir::new("hash");
        let path = dir.path.join("body");
        fs::write(&path, b"abc").unwrap();
        let hash = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad";
        assert_eq!(hash_file(&path).unwrap(), hash);
        assert!(valid_version("0.2.0"));
        assert!(valid_version("2.0.0-dev"));
        assert!(!valid_version("../0.2.0"));
        assert!(!valid_version(""));
        assert_eq!(percent(0, 4), 0);
        assert_eq!(percent(1, 4), 25);
        assert_eq!(percent(4, 4), 100);
        assert_eq!(percent(1, 0), 0);
        assert_eq!(platform_label("windows"), "Windows");
    }

    #[test]
    fn stream_checks_size_and_removes_a_short_file() {
        let body = b"abc";
        let hash = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad";
        let url = serve(body, hash);
        let dir = crate::paths::TmpDir::new("stream");
        let dest = dir.path.join("file.partial");
        let remote = crate::remote::Remote::download_client(&origin_of(&url)).unwrap();
        let mut seen = Vec::new();
        let size = u64::try_from(body.len()).unwrap();
        let header = tauri::async_runtime::block_on(stream_to(
            remote.http(),
            &url,
            &dest,
            size,
            |got, total| seen.push((got, total)),
        ))
        .unwrap();
        assert_eq!(header, hash);
        assert_eq!(fs::read(&dest).unwrap(), body);
        assert_eq!(seen.last().copied(), Some((size, size)));

        let again = serve(body, hash);
        let short = dir.path.join("short.partial");
        let err =
            tauri::async_runtime::block_on(stream_to(remote.http(), &again, &short, 9, |_, _| {}))
                .unwrap_err();
        assert!(err.contains("Размер"));
        assert!(!short.exists());
    }

    fn origin_of(url: &str) -> String {
        let rest = url.trim_start_matches("http://");
        let host = rest.split('/').next().unwrap_or(rest);
        format!("http://{host}")
    }

    fn serve(body: &[u8], sha: &str) -> String {
        use std::io::{Read, Write};
        use std::net::TcpListener;
        let listener = TcpListener::bind("127.0.0.1:0").unwrap();
        let port = listener.local_addr().unwrap().port();
        let body = body.to_vec();
        let sha = sha.to_string();
        std::thread::spawn(move || {
            let (mut sock, _) = listener.accept().unwrap();
            let mut buf = Vec::new();
            let mut tmp = [0u8; 256];
            loop {
                let read = sock.read(&mut tmp).unwrap_or(0);
                if read == 0 {
                    break;
                }
                buf.extend_from_slice(&tmp[..read]);
                if buf.windows(4).any(|item| item == b"\r\n\r\n") {
                    break;
                }
            }
            let header = format!(
                "HTTP/1.1 200 OK\r\nContent-Length: {}\r\nX-Content-SHA256: {sha}\r\nConnection: close\r\n\r\n",
                body.len()
            );
            let _ = sock.write_all(header.as_bytes());
            let _ = sock.write_all(&body);
        });
        format!("http://127.0.0.1:{port}/api/releases/0.2.0/download/linux")
    }
}
