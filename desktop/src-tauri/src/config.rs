use std::collections::BTreeMap;
use std::fs;
use std::path::Path;

use serde::{Deserialize, Serialize};

use crate::plans::PlanItem;
use crate::remote;
use crate::visio::Song;

/// `desktop.json` is separate from the CLI `session.json`: the shapes differ,
/// and one program must not wipe the other's file.
#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct Config {
    #[serde(default = "default_server")]
    pub server_url: String,
    #[serde(default)]
    pub token: String,
    #[serde(default)]
    pub user: UserProfile,
    #[serde(default)]
    pub visio_path: String,
    #[serde(default)]
    pub songbooks: BTreeMap<String, SongbookBind>,
    #[serde(default)]
    pub plans: BTreeMap<String, PlanBind>,
    #[serde(default)]
    pub pending_fbids: BTreeMap<String, String>,
    /// Пусто или `utf-8` — как раньше. `windows-1251` — планы и PlanDef.ini.
    #[serde(default)]
    pub plan_encoding: String,
    /// Пропущенные сборники и планы. Ключ — сервер и учётная запись.
    #[serde(default)]
    pub skipped: BTreeMap<String, SkippedSync>,
}

/// Сборники и планы, которые на этом компьютере решили не копировать.
#[derive(Clone, Debug, Default, Serialize, Deserialize, PartialEq, Eq)]
pub struct SkippedSync {
    #[serde(default)]
    pub songbooks: Vec<String>,
    #[serde(default)]
    pub plans: Vec<String>,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
pub struct UserProfile {
    #[serde(default)]
    pub user_id: String,
    #[serde(default)]
    pub device_id: String,
    #[serde(default)]
    pub dimension_id: String,
    #[serde(default)]
    pub email: String,
    #[serde(default)]
    pub role: String,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
pub struct GroupLink {
    #[serde(default)]
    pub section_id: String,
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub songs: Vec<String>,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
pub struct SongbookBind {
    #[serde(default)]
    pub server_id: String,
    #[serde(default)]
    pub book_fbid: String,
    #[serde(default)]
    pub cursor: i64,
    #[serde(default)]
    pub songs: Vec<Song>,
    #[serde(default)]
    pub groups: Vec<GroupLink>,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
pub struct PlanBind {
    #[serde(default)]
    pub server_id: String,
    #[serde(default)]
    pub dimension_id: String,
    #[serde(default)]
    pub version: i64,
    #[serde(default)]
    pub updated_at: String,
    #[serde(default)]
    pub scope: String,
    #[serde(default)]
    pub items: Vec<PlanItem>,
}

#[derive(Clone, Debug, Serialize)]
pub struct PublicSettings {
    pub server_url: String,
    pub email: String,
    pub logged_in: bool,
    pub visio_path: String,
    pub user_id: String,
    pub dimension_id: String,
    pub role: String,
    pub plan_windows1251: bool,
}

fn default_server() -> String {
    "http://localhost:8080".into()
}

impl Default for Config {
    fn default() -> Self {
        Self {
            server_url: default_server(),
            token: String::new(),
            user: UserProfile::default(),
            visio_path: String::new(),
            songbooks: BTreeMap::new(),
            plans: BTreeMap::new(),
            pending_fbids: BTreeMap::new(),
            plan_encoding: String::new(),
            skipped: BTreeMap::new(),
        }
    }
}

pub fn account_key(server_url: &str, user_id: &str) -> String {
    format!("{server_url}\n{user_id}")
}

impl Config {
    pub fn skipped_for(&self) -> SkippedSync {
        self.skipped
            .get(&account_key(&self.server_url, &self.user.user_id))
            .cloned()
            .unwrap_or_default()
    }
}

/// `kind` — `songbook` или `plan`. Пустой id не записывается.
pub fn set_skipped(cfg: &mut Config, kind: &str, id: &str, skip: bool) -> Result<(), String> {
    let id = id.trim();
    if id.is_empty() {
        return Err("Нет идентификатора".into());
    }
    if cfg.user.user_id.is_empty() {
        return Err("Сначала войдите в учётную запись".into());
    }
    let books = match kind {
        "songbook" => true,
        "plan" => false,
        _ => return Err("Неизвестный вид".into()),
    };
    let key = account_key(&cfg.server_url, &cfg.user.user_id);
    let mut entry = cfg.skipped.remove(&key).unwrap_or_default();
    let list = if books {
        &mut entry.songbooks
    } else {
        &mut entry.plans
    };
    list.retain(|item| item != id);
    if skip {
        list.push(id.to_string());
    }
    list.sort();
    if !entry.songbooks.is_empty() || !entry.plans.is_empty() {
        cfg.skipped.insert(key, entry);
    }
    Ok(())
}

impl Config {
    pub fn public(&self) -> PublicSettings {
        PublicSettings {
            server_url: self.server_url.clone(),
            email: self.user.email.clone(),
            logged_in: !self.token.is_empty(),
            visio_path: self.visio_path.clone(),
            user_id: self.user.user_id.clone(),
            dimension_id: self.user.dimension_id.clone(),
            role: self.user.role.clone(),
            plan_windows1251: crate::plans::normalize_plan_encoding(&self.plan_encoding)
                == "windows-1251",
        }
    }
}

pub fn load(dir: &Path) -> Result<Config, String> {
    let path = dir.join("desktop.json");
    if !path.exists() {
        return Ok(Config::default());
    }
    let raw = fs::read(&path).map_err(|err| err.to_string())?;
    serde_json::from_slice(&raw).map_err(|_| "Файл настроек повреждён".to_string())
}

pub fn save(dir: &Path, cfg: &Config) -> Result<(), String> {
    fs::create_dir_all(dir).map_err(|err| err.to_string())?;
    restrict_dir(dir);
    let path = dir.join("desktop.json");
    let mut raw = serde_json::to_vec_pretty(cfg).map_err(|err| err.to_string())?;
    raw.push(b'\n');
    fs::write(&path, &raw).map_err(|err| err.to_string())?;
    restrict_file(&path);
    Ok(())
}

/// A new server address drops the previous sign-in and the remembered sync base.
/// Those belong to the old server. The chosen VisioBible folder stays.
pub fn apply_server_url(cfg: &mut Config, url: &str) -> Result<(), String> {
    let url = remote::parse_server(url)?;
    if url != cfg.server_url {
        cfg.token.clear();
        cfg.user.user_id.clear();
        cfg.user.dimension_id.clear();
        cfg.user.email.clear();
        for book in cfg.songbooks.values_mut() {
            book.server_id.clear();
            book.cursor = 0;
            book.songs.clear();
            book.groups.clear();
        }
        for plan in cfg.plans.values_mut() {
            plan.server_id.clear();
            plan.items.clear();
        }
    }
    cfg.server_url = url;
    Ok(())
}

#[cfg(unix)]
fn restrict_dir(path: &Path) {
    use std::os::unix::fs::PermissionsExt;
    let _ = fs::set_permissions(path, fs::Permissions::from_mode(0o700));
}

#[cfg(not(unix))]
fn restrict_dir(_path: &Path) {}

#[cfg(unix)]
fn restrict_file(path: &Path) {
    use std::os::unix::fs::PermissionsExt;
    let _ = fs::set_permissions(path, fs::Permissions::from_mode(0o600));
}

#[cfg(not(unix))]
fn restrict_file(_path: &Path) {}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::paths::TmpDir;

    #[test]
    fn roundtrip_hides_nothing_but_limits_the_file() {
        let tmp = TmpDir::new("cfg");
        let cfg = Config {
            token: "secret-token".into(),
            user: UserProfile {
                email: "choir@example.com".into(),
                ..UserProfile::default()
            },
            visio_path: "/tmp/visio".into(),
            ..Config::default()
        };
        save(&tmp.path, &cfg).unwrap();
        let loaded = load(&tmp.path).unwrap();
        assert_eq!(loaded.token, "secret-token");
        assert_eq!(loaded.server_url, "http://localhost:8080");
        assert_eq!(loaded.user.email, "choir@example.com");
        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;
            let mode = fs::metadata(tmp.path.join("desktop.json"))
                .unwrap()
                .permissions()
                .mode()
                & 0o777;
            assert_eq!(mode, 0o600);
        }
    }

    #[test]
    fn changing_server_forgets_the_old_account() {
        let mut cfg = Config {
            token: "secret-token".into(),
            user: UserProfile {
                email: "choir@example.com".into(),
                device_id: "device".into(),
                ..UserProfile::default()
            },
            visio_path: "/tmp/visio".into(),
            ..Config::default()
        };
        cfg.songbooks.insert(
            "a.vbm".into(),
            SongbookBind {
                server_id: "book".into(),
                songs: vec![crate::visio::Song {
                    title: "Песня".into(),
                    ..crate::visio::Song::default()
                }],
                groups: vec![GroupLink {
                    section_id: "section".into(),
                    name: "Хор".into(),
                    songs: vec!["song".into()],
                }],
                ..SongbookBind::default()
            },
        );
        apply_server_url(&mut cfg, "https://example.com").unwrap();
        assert!(cfg.token.is_empty());
        assert!(cfg.user.email.is_empty());
        assert_eq!(cfg.user.device_id, "device");
        assert_eq!(cfg.visio_path, "/tmp/visio");
        assert!(cfg.songbooks["a.vbm"].server_id.is_empty());
        assert!(cfg.songbooks["a.vbm"].songs.is_empty());
        assert!(cfg.songbooks["a.vbm"].groups.is_empty());
        apply_server_url(&mut cfg, "https://example.com/").unwrap();
        assert_eq!(cfg.server_url, "https://example.com");
    }

    #[test]
    fn skips_belong_to_one_server_and_account() {
        let tmp = TmpDir::new("skips");
        let mut cfg = Config {
            server_url: "https://example.com".into(),
            user: UserProfile {
                user_id: "user-a".into(),
                ..UserProfile::default()
            },
            ..Config::default()
        };
        set_skipped(&mut cfg, "songbook", "book-1", true).unwrap();
        set_skipped(&mut cfg, "plan", "plan-1", true).unwrap();
        set_skipped(&mut cfg, "songbook", "book-1", true).unwrap();
        assert_eq!(cfg.skipped_for().songbooks, vec!["book-1".to_string()]);
        assert_eq!(cfg.skipped_for().plans, vec!["plan-1".to_string()]);
        save(&tmp.path, &cfg).unwrap();
        let loaded = load(&tmp.path).unwrap();
        assert_eq!(loaded.skipped_for().songbooks, vec!["book-1".to_string()]);

        cfg.user.user_id = "user-b".into();
        assert!(cfg.skipped_for().songbooks.is_empty());
        cfg.user.user_id = "user-a".into();
        cfg.server_url = "http://localhost:8080".into();
        assert!(cfg.skipped_for().plans.is_empty());
        cfg.server_url = "https://example.com".into();
        set_skipped(&mut cfg, "plan", "plan-1", false).unwrap();
        set_skipped(&mut cfg, "songbook", "book-1", false).unwrap();
        assert!(cfg.skipped.is_empty());
        assert!(set_skipped(&mut cfg, "other", "book-1", true).is_err());
    }
}
