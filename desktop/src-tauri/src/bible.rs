use std::fs;
use std::path::{Path, PathBuf};

use sha2::{Digest, Sha256};

use crate::plans::valid_module_name;
use crate::text;

const MAX_MODULE_FILE: u64 = 16 << 20;
const MAX_MODULE_BYTES: u64 = 64 << 20;
const MAX_MODULE_FILES: usize = 80;

/// Тот же канонический текст, что `bibleIdentity` на сервере.
/// `BibleId` и `BookHash` не входят: VisioBible дописывает их, не меняя книги.
#[derive(Clone, Debug, PartialEq)]
pub struct BibleIdentity {
    pub dir: PathBuf,
    pub short_name: String,
    pub title: String,
    pub content_hash: String,
    pub files: Vec<String>,
}

struct Kv {
    key: String,
    value: String,
}

struct BookCanon {
    path: String,
    keys: Vec<Kv>,
}

/// Модуль уже есть на сервере — файл не отправлять, сохранить id.
pub fn should_upload(exists: bool, server_id: &str) -> bool {
    !exists || server_id.is_empty()
}

pub fn identity(dir: &Path) -> Result<BibleIdentity, String> {
    if !dir.is_dir() {
        return Err("каталог модуля".into());
    }
    let ini_path = find_ini(dir)?;
    let ini_name = ini_path
        .file_name()
        .and_then(|name| name.to_str())
        .ok_or("bibleqt.ini")?
        .to_string();
    let raw = fs::read(&ini_path).map_err(|err| err.to_string())?;
    if raw.len() as u64 > MAX_MODULE_FILE {
        return Err("Модуль больше допустимого размера".into());
    }
    let text = text::decode_text(&raw);
    let (short_name, title, header, books) = parse_canon(&text)?;
    let mut total = raw.len() as u64;
    let mut files = vec![ini_name];
    let mut payload = String::new();
    payload.push_str("short=");
    payload.push_str(&short_name);
    payload.push('\n');
    payload.push_str("title=");
    payload.push_str(&title);
    payload.push('\n');
    for item in &header {
        payload.push_str("h=");
        payload.push_str(&item.key);
        payload.push('\t');
        payload.push_str(&item.value);
        payload.push('\n');
    }
    for book in &books {
        let (rel, data) = read_rel(dir, &book.path)?;
        if data.len() as u64 > MAX_MODULE_FILE {
            return Err("Модуль больше допустимого размера".into());
        }
        total += data.len() as u64;
        if total > MAX_MODULE_BYTES {
            return Err("Модуль больше допустимого размера".into());
        }
        let sum = sha256_hex(&data);
        files.push(rel);
        payload.push_str("book=");
        payload.push_str(&book.path);
        payload.push('\n');
        for item in &book.keys {
            payload.push_str("k=");
            payload.push_str(&item.key);
            payload.push('\t');
            payload.push_str(&item.value);
            payload.push('\n');
        }
        payload.push_str("file=");
        payload.push_str(&sum);
        payload.push('\n');
    }
    if files.len() > MAX_MODULE_FILES {
        return Err("Модуль больше допустимого размера".into());
    }
    Ok(BibleIdentity {
        dir: dir.to_path_buf(),
        short_name,
        title,
        content_hash: sha256_hex(payload.as_bytes()),
        files,
    })
}

pub fn read_upload(module: &BibleIdentity) -> Result<Vec<(String, Vec<u8>)>, String> {
    let mut out = Vec::with_capacity(module.files.len());
    for rel in &module.files {
        let path = module.dir.join(rel);
        let data = fs::read(&path).map_err(|err| err.to_string())?;
        out.push((rel.clone(), data));
    }
    Ok(out)
}

fn parse_canon(text: &str) -> Result<(String, String, Vec<Kv>, Vec<BookCanon>), String> {
    let mut header = Vec::new();
    let mut books = Vec::new();
    let mut book: Option<BookCanon> = None;
    let mut short_name = String::new();
    let mut title = String::new();
    for line in text.split('\n') {
        let line = line.trim().trim_end_matches('\r');
        if line.is_empty() || line.starts_with(';') {
            continue;
        }
        let Some((key, value)) = line.split_once('=') else {
            continue;
        };
        let key = key.trim();
        let value = value.trim();
        if key.is_empty() {
            continue;
        }
        if key.eq_ignore_ascii_case("BibleId") || key.eq_ignore_ascii_case("BookHash") {
            continue;
        }
        if key.eq_ignore_ascii_case("PathName") {
            if let Some(done) = book.take() {
                books.push(done);
            }
            book = Some(BookCanon {
                path: value.to_string(),
                keys: Vec::new(),
            });
            continue;
        }
        if book.is_none() {
            if key.eq_ignore_ascii_case("BibleShortName") {
                short_name = value.to_string();
                continue;
            }
            if key.eq_ignore_ascii_case("BibleName") {
                title = value.to_string();
                continue;
            }
            header.push(Kv {
                key: key.to_string(),
                value: value.to_string(),
            });
            continue;
        }
        if let Some(current) = book.as_mut() {
            current.keys.push(Kv {
                key: key.to_string(),
                value: value.to_string(),
            });
        }
    }
    if let Some(done) = book.take() {
        books.push(done);
    }
    if !valid_module_name(&short_name) || title.chars().count() > 200 || books.is_empty() {
        return Err("запрос".into());
    }
    let mut seen = Vec::new();
    for item in &books {
        let key = rel_key(&item.path)?;
        if seen.iter().any(|prev| prev == &key) {
            return Err("запрос".into());
        }
        seen.push(key);
    }
    Ok((short_name, title, header, books))
}

fn rel_key(path: &str) -> Result<String, String> {
    let parts = rel_parts(path)?;
    Ok(parts.join("/").to_ascii_lowercase())
}

fn rel_parts(path: &str) -> Result<Vec<String>, String> {
    if path.contains('\0') {
        return Err("файл книги вне каталога".into());
    }
    let mut parts = Vec::new();
    for part in path.replace('\\', "/").split('/') {
        if part.is_empty() || part == "." || part == ".." {
            return Err("файл книги вне каталога".into());
        }
        parts.push(part.to_string());
    }
    if parts.is_empty() {
        return Err("файл книги вне каталога".into());
    }
    Ok(parts)
}

fn find_ini(dir: &Path) -> Result<PathBuf, String> {
    let mut found = None;
    for entry in fs::read_dir(dir).map_err(|err| err.to_string())? {
        let entry = entry.map_err(|err| err.to_string())?;
        let name = entry.file_name();
        let Some(name) = name.to_str() else {
            continue;
        };
        if !name.eq_ignore_ascii_case("bibleqt.ini") {
            continue;
        }
        let meta = fs::symlink_metadata(entry.path()).map_err(|err| err.to_string())?;
        if meta.file_type().is_symlink() {
            return Err("bibleqt.ini".into());
        }
        if !meta.is_file() {
            continue;
        }
        if found.is_some() {
            return Err("bibleqt.ini".into());
        }
        found = Some(entry.path());
    }
    found.ok_or_else(|| "bibleqt.ini".into())
}

fn read_rel(root: &Path, rel: &str) -> Result<(String, Vec<u8>), String> {
    let parts = rel_parts(rel)?;
    let mut current = root.to_path_buf();
    let mut built = Vec::new();
    for (index, part) in parts.iter().enumerate() {
        let mut matched = None;
        for entry in fs::read_dir(&current).map_err(|_| "файл книги".to_string())? {
            let entry = entry.map_err(|_| "файл книги".to_string())?;
            let name = entry.file_name();
            let Some(name) = name.to_str() else {
                continue;
            };
            if name.eq_ignore_ascii_case(part) {
                if matched.is_some() {
                    return Err("файл книги".into());
                }
                matched = Some(name.to_string());
            }
        }
        let Some(name) = matched else {
            return Err("файл книги".into());
        };
        current.push(&name);
        let meta = fs::symlink_metadata(&current).map_err(|_| "файл книги".to_string())?;
        if meta.file_type().is_symlink() {
            return Err("файл книги вне каталога".into());
        }
        let last = index + 1 == parts.len();
        if last {
            if !meta.is_file() || meta.len() > MAX_MODULE_FILE {
                return Err("файл книги".into());
            }
        } else if !meta.is_dir() {
            return Err("файл книги".into());
        }
        built.push(name);
    }
    let data = fs::read(&current).map_err(|err| err.to_string())?;
    if data.len() as u64 > MAX_MODULE_FILE {
        return Err("Модуль больше допустимого размера".into());
    }
    Ok((built.join("/"), data))
}

fn sha256_hex(bytes: &[u8]) -> String {
    const HEX: &[u8; 16] = b"0123456789abcdef";
    let sum = Sha256::digest(bytes);
    let mut out = String::with_capacity(sum.len() * 2);
    for byte in sum {
        out.push(HEX[(byte >> 4) as usize] as char);
        out.push(HEX[(byte & 0xf) as usize] as char);
    }
    out
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::paths::TmpDir;

    const INI: &str = "BibleName=Пример\nBibleShortName=EX\nChapterSign=<h4>\nVerseSign=<p>\nPathName=grief.htm\nFullName=Истории\nShortName=Ист\nChapterQty=1\n";
    const HASH: &str = "c004ac1417fa0542735fca37e8e3e1517583e5c4e505ad0f5d19c7f0cb3770a0";

    fn write_module(dir: &Path, noisy: bool) {
        let ini = if noisy {
            "BibleName=Пример\nBibleShortName=EX\nBibleId=999\nChapterSign=<h4>\nVerseSign=<p>\nPathName=grief.htm\nFullName=Истории\nBookHash=deadbeef\nShortName=Ист\nChapterQty=1\n"
        } else {
            INI
        };
        if noisy {
            fs::write(dir.join("visiobible.idx"), b"idx").unwrap();
            fs::write(dir.join("visiobible.cache"), b"cache").unwrap();
        }
        fs::write(dir.join("bibleqt.ini"), ini).unwrap();
        fs::write(dir.join("grief.htm"), b"verse-1").unwrap();
    }

    #[test]
    fn hash_ignores_visio_marks_and_matches_the_server() {
        let tmp = TmpDir::new("bible");
        write_module(&tmp.path, false);
        let module = identity(&tmp.path).unwrap();
        assert_eq!(module.content_hash, HASH);
        assert_eq!(module.short_name, "EX");
        assert_eq!(module.title, "Пример");
        assert_eq!(
            module.files,
            vec!["bibleqt.ini".to_string(), "grief.htm".to_string()]
        );
        let noisy = TmpDir::new("bible-noisy");
        write_module(&noisy.path, true);
        let again = identity(&noisy.path).unwrap();
        assert_eq!(again.content_hash, module.content_hash);
        assert!(!again.files.iter().any(|name| name.contains("idx")));
        fs::write(tmp.path.join("grief.htm"), b"verse-2").unwrap();
        let changed = identity(&tmp.path).unwrap();
        assert_ne!(changed.content_hash, HASH);
    }

    #[test]
    fn existing_module_is_not_uploaded() {
        assert!(!should_upload(true, "11111111-1111-4111-8111-111111111111"));
        assert!(should_upload(false, ""));
        assert!(should_upload(true, ""));
        let tmp = TmpDir::new("bible-up");
        write_module(&tmp.path, true);
        let module = identity(&tmp.path).unwrap();
        let files = read_upload(&module).unwrap();
        assert_eq!(files.len(), 2);
        assert!(files.iter().all(|(name, _)| name != "visiobible.idx"));
    }
}
