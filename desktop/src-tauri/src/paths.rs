use std::path::PathBuf;

/// Same folder as the command-line client, so both programs share one place.
/// Windows: `%AppData%\flammebible`. Elsewhere: `$HOME/.config/flammebible`.
/// `FLAMMEBIBLE_CONFIG` overrides both.
pub fn config_dir() -> Result<PathBuf, String> {
    let home = std::env::var("HOME").or_else(|_| std::env::var("USERPROFILE"));
    settings_dir(
        std::env::consts::OS,
        &std::env::var("FLAMMEBIBLE_CONFIG").unwrap_or_default(),
        &std::env::var("APPDATA").unwrap_or_default(),
        home.as_deref().unwrap_or(""),
    )
}

pub fn settings_dir(os: &str, env: &str, appdata: &str, home: &str) -> Result<PathBuf, String> {
    if !env.is_empty() {
        return Ok(PathBuf::from(env));
    }
    if os == "windows" {
        if appdata.is_empty() {
            return Err("Не найден каталог AppData".into());
        }
        return Ok(PathBuf::from(appdata).join("flammebible"));
    }
    if home.is_empty() {
        return Err("Не найден домашний каталог".into());
    }
    Ok(PathBuf::from(home).join(".config").join("flammebible"))
}

#[cfg(test)]
pub(crate) struct TmpDir {
    pub path: std::path::PathBuf,
}

#[cfg(test)]
impl TmpDir {
    pub(crate) fn new(label: &str) -> Self {
        use std::sync::atomic::{AtomicU64, Ordering};
        static N: AtomicU64 = AtomicU64::new(0);
        let n = N.fetch_add(1, Ordering::Relaxed);
        let path = std::env::temp_dir().join(format!(
            "flammebible-desktop-{label}-{n}-{}",
            std::process::id()
        ));
        std::fs::create_dir_all(&path).unwrap();
        Self { path }
    }
}

#[cfg(test)]
impl Drop for TmpDir {
    fn drop(&mut self) {
        let _ = std::fs::remove_dir_all(&self.path);
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn windows_uses_appdata_and_env_wins() {
        let dir = settings_dir("windows", "", r"C:\Users\choir\AppData\Roaming", "").unwrap();
        assert_eq!(
            dir,
            PathBuf::from(r"C:\Users\choir\AppData\Roaming").join("flammebible")
        );
        let custom = settings_dir("linux", "/tmp/fb", "", "/home/choir").unwrap();
        assert_eq!(custom, PathBuf::from("/tmp/fb"));
    }

    #[test]
    fn linux_uses_config_under_home() {
        let dir = settings_dir("linux", "", "", "/home/choir").unwrap();
        assert_eq!(dir, PathBuf::from("/home/choir/.config/flammebible"));
        assert!(settings_dir("linux", "", "", "").is_err());
        assert!(settings_dir("windows", "", "", "/home/choir").is_err());
    }
}
