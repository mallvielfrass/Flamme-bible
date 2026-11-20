use std::fs;
use std::path::Path;

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

/// The download header `X-Content-SHA256` and the hash from `/api/releases/latest` must both match the bytes.
pub fn verify_download(bytes: &[u8], header_sha: &str, expected: &str) -> Result<(), String> {
    let got = sha256_hex(bytes);
    let expected = normalize_sha(expected)?;
    let header = normalize_sha(header_sha)?;
    if !ct_eq(&got, &expected) || !ct_eq(&header, &expected) {
        return Err("Файл обновления не совпал с контрольной суммой и не сохранён.".into());
    }
    Ok(())
}

pub fn sha256_hex(bytes: &[u8]) -> String {
    format!("{:x}", Sha256::digest(bytes))
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

pub fn update_path(
    dir: &Path,
    version: &str,
    platform: &str,
) -> Result<std::path::PathBuf, String> {
    if !version
        .chars()
        .all(|ch| ch.is_ascii_alphanumeric() || matches!(ch, '.' | '_' | '-' | '+'))
        || version.contains("..")
        || version.is_empty()
    {
        return Err("Номер версии не подходит".into());
    }
    let name = match platform {
        "windows" => "flammebible-windows.exe",
        "macos" => "flammebible-macos",
        _ => "flammebible-linux",
    };
    Ok(dir.join("updates").join(version).join(name))
}

pub fn save_verified(
    dir: &Path,
    version: &str,
    platform: &str,
    bytes: &[u8],
    header_sha: &str,
    expected: &str,
) -> Result<std::path::PathBuf, String> {
    verify_download(bytes, header_sha, expected)?;
    let path = update_path(dir, version, platform)?;
    if let Some(parent) = path.parent() {
        fs::create_dir_all(parent).map_err(|err| err.to_string())?;
    }
    let tmp = path.with_extension("partial");
    fs::write(&tmp, bytes).map_err(|err| err.to_string())?;
    fs::rename(&tmp, &path).map_err(|err| err.to_string())?;
    Ok(path)
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
        let bytes = b"abc";
        let hash = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad";
        assert!(verify_download(bytes, hash, hash).is_ok());
        assert!(verify_download(bytes, &"ab".repeat(32), hash).is_err());
        assert!(verify_download(b"nope", hash, hash).is_err());
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
}
