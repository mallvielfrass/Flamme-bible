use std::fs::{self, File};
use std::io::Write;
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicBool, Ordering};

use serde::{Deserialize, Serialize};
use tauri::Emitter;

use crate::remote::{ReleaseInfo, Remote};
use crate::updater;

const MAX_RELEASE_BYTES: i64 = 512 * 1024 * 1024;

static BUSY: AtomicBool = AtomicBool::new(false);

struct BusyGuard;

impl Drop for BusyGuard {
    fn drop(&mut self) {
        BUSY.store(false, Ordering::SeqCst);
    }
}

fn busy_guard() -> Result<BusyGuard, String> {
    if BUSY.swap(true, Ordering::SeqCst) {
        return Err("Подождите, обновление ещё выполняется".into());
    }
    Ok(BusyGuard)
}

#[derive(Serialize)]
pub struct UpdateOffer {
    pub current: String,
    pub latest: String,
    pub notes: String,
    pub available: bool,
    pub platform: String,
    pub platform_label: String,
    pub size: i64,
    pub sha256: String,
    pub portable: bool,
    pub staged: bool,
    pub armed: bool,
    pub staged_path: String,
}

#[derive(Clone, Serialize)]
pub struct UpdateProgress {
    pub received: u64,
    pub total: u64,
    pub percentage: u32,
}

#[derive(Serialize)]
pub struct DownloadDone {
    pub path: String,
    pub portable: bool,
}

#[derive(Serialize, Deserialize)]
struct Marker {
    version: String,
    sha256: String,
}

struct Place {
    partial: PathBuf,
    final_path: PathBuf,
    portable: bool,
}

enum Apply {
    None,
    Swapped,
}

pub fn is_dev_binary(path: &Path) -> bool {
    let text = path
        .to_string_lossy()
        .replace('\\', "/")
        .to_ascii_lowercase();
    text.contains("/target/debug/") || text.contains("/target/release/")
}

pub fn can_swap(os: &str, exe: &Path) -> bool {
    os == "windows" && !is_dev_binary(exe)
}

pub fn staged_path(exe: &Path) -> PathBuf {
    with_suffix(exe, ".new")
}

pub fn marker_path(exe: &Path) -> PathBuf {
    with_suffix(exe, ".update.json")
}

fn partial_suffix(path: &Path) -> PathBuf {
    with_suffix(path, ".partial")
}

fn backup_path(exe: &Path) -> PathBuf {
    with_suffix(exe, ".old")
}

fn with_suffix(path: &Path, suffix: &str) -> PathBuf {
    let mut name = path
        .file_name()
        .map(|name| name.to_os_string())
        .unwrap_or_default();
    name.push(suffix);
    path.with_file_name(name)
}

fn place(
    os: &str,
    exe: &Path,
    config_dir: &Path,
    version: &str,
    platform: &str,
) -> Result<Place, String> {
    if can_swap(os, exe) {
        if exe.file_name().is_none() {
            return Err("Не найден файл программы".into());
        }
        return Ok(Place {
            partial: partial_suffix(exe),
            final_path: staged_path(exe),
            portable: true,
        });
    }
    let final_path = updater::update_path(config_dir, version, platform)?;
    Ok(Place {
        partial: partial_suffix(&final_path),
        final_path,
        portable: false,
    })
}

pub fn commit_verified(
    partial: &Path,
    final_path: &Path,
    header_sha: &str,
    expected_sha: &str,
) -> Result<(), String> {
    let got = updater::hash_file(partial)?;
    if let Err(err) = updater::verify_hashes(&got, header_sha, expected_sha) {
        let _ = fs::remove_file(partial);
        return Err(err);
    }
    if let Some(parent) = final_path.parent() {
        if !parent.as_os_str().is_empty() {
            fs::create_dir_all(parent).map_err(|err| err.to_string())?;
        }
    }
    if final_path.exists() {
        fs::remove_file(final_path).map_err(|err| err.to_string())?;
    }
    if let Err(err) = fs::rename(partial, final_path) {
        let _ = fs::remove_file(partial);
        return Err(err.to_string());
    }
    mark_executable(final_path)
}

pub fn arm(exe: &Path, version: &str, expected_sha: &str) -> Result<(), String> {
    if !updater::valid_version(version) {
        return Err("Номер версии не подходит".into());
    }
    let staged = staged_path(exe);
    if !staged.is_file() {
        return Err("Сначала скачайте и проверьте файл".into());
    }
    let got = updater::hash_file(&staged)?;
    if let Err(err) = updater::verify_hashes(&got, expected_sha, expected_sha) {
        let _ = fs::remove_file(&staged);
        return Err(err);
    }
    let marker = Marker {
        version: version.to_string(),
        sha256: updater::normalize_sha(expected_sha)?,
    };
    let bytes = serde_json::to_vec(&marker).map_err(|err| err.to_string())?;
    write_atomic(&marker_path(exe), &bytes)
}

fn apply(exe: &Path) -> Result<Apply, String> {
    let marker = marker_path(exe);
    if !marker.is_file() {
        return Ok(Apply::None);
    }
    let text = fs::read_to_string(&marker).map_err(|err| err.to_string())?;
    let parsed: Marker = match serde_json::from_str(&text) {
        Ok(value) => value,
        Err(_) => {
            let _ = fs::remove_file(&marker);
            return Err("Файл замены повреждён".into());
        }
    };
    let staged = staged_path(exe);
    let got = match updater::hash_file(&staged) {
        Ok(value) => value,
        Err(_) => {
            let _ = fs::remove_file(&marker);
            return Err("Файл обновления не найден".into());
        }
    };
    if let Err(err) = updater::verify_hashes(&got, &parsed.sha256, &parsed.sha256) {
        let _ = fs::remove_file(&marker);
        let _ = fs::remove_file(&staged);
        return Err(err);
    }
    swap_files(exe, &staged, &backup_path(exe))?;
    let _ = fs::remove_file(&marker);
    Ok(Apply::Swapped)
}

fn swap_files(current: &Path, staged: &Path, backup: &Path) -> Result<(), String> {
    if backup.exists() {
        fs::remove_file(backup).map_err(|err| err.to_string())?;
    }
    rename_retry(current, backup)?;
    if let Err(err) = rename_retry(staged, current) {
        if rename_retry(backup, current).is_err() {
            let _ = fs::copy(backup, current);
        }
        return Err(err);
    }
    Ok(())
}

fn rename_retry(from: &Path, to: &Path) -> Result<(), String> {
    let mut last = String::from("Не удалось заменить файл");
    for attempt in 0..20 {
        match fs::rename(from, to) {
            Ok(()) => return Ok(()),
            Err(err) if err.kind() == std::io::ErrorKind::NotFound => return Err(err.to_string()),
            Err(err) => {
                last = err.to_string();
                if attempt + 1 == 20 {
                    break;
                }
                std::thread::sleep(std::time::Duration::from_millis(50));
            }
        }
    }
    Err(last)
}

pub fn apply_pending_update() -> bool {
    let Ok(exe) = std::env::current_exe() else {
        return false;
    };
    if !can_swap(std::env::consts::OS, &exe) {
        return false;
    }
    match apply(&exe) {
        Ok(Apply::None) => {
            let _ = fs::remove_file(backup_path(&exe));
            false
        }
        Ok(Apply::Swapped) => relaunch(&exe),
        Err(_) => false,
    }
}

fn relaunch(exe: &Path) -> bool {
    let args: Vec<String> = std::env::args().skip(1).collect();
    let _ = std::process::Command::new(exe).args(&args).spawn();
    true
}

fn stage_state(
    os: &str,
    exe: &Path,
    config_dir: &Path,
    version: &str,
    platform: &str,
    expected_sha: &str,
) -> Result<(bool, bool, String), String> {
    if expected_sha.is_empty() || !updater::valid_version(version) {
        return Ok((false, false, String::new()));
    }
    let place = place(os, exe, config_dir, version, platform)?;
    if !place.final_path.is_file() {
        return Ok((false, false, String::new()));
    }
    let got = updater::hash_file(&place.final_path)?;
    if !updater::hashes_equal(&got, expected_sha) {
        return Ok((false, false, String::new()));
    }
    let path = place.final_path.display().to_string();
    let armed = place.portable && marker_matches(exe, expected_sha);
    Ok((true, armed, path))
}

fn marker_matches(exe: &Path, expected_sha: &str) -> bool {
    let Ok(text) = fs::read_to_string(marker_path(exe)) else {
        return false;
    };
    let Ok(marker) = serde_json::from_str::<Marker>(&text) else {
        return false;
    };
    updater::hashes_equal(&marker.sha256, expected_sha)
}

fn offer_from(dir: &Path, info: &ReleaseInfo) -> Result<UpdateOffer, String> {
    let platform = updater::this_platform();
    let current = updater::current_version();
    let file = info.file(platform);
    let available = updater::version_is_newer(current, &info.version)
        && info.status == "released"
        && file.is_some();
    let exe = std::env::current_exe().unwrap_or_else(|_| dir.join("flammebible"));
    let os = std::env::consts::OS;
    let (sha, size) = match &file {
        Some(file) => (file.sha256.clone(), file.size),
        None => (String::new(), 0),
    };
    let (staged, armed, staged_path) = stage_state(os, &exe, dir, &info.version, platform, &sha)?;
    Ok(UpdateOffer {
        current: current.to_string(),
        latest: info.version.clone(),
        notes: info.notes.clone(),
        available,
        platform: platform.to_string(),
        platform_label: updater::platform_label(platform).to_string(),
        size,
        sha256: sha,
        portable: can_swap(os, &exe),
        staged,
        armed,
        staged_path,
    })
}

#[tauri::command]
pub async fn check_update() -> Result<UpdateOffer, String> {
    let (dir, cfg) = super::load_config()?;
    let info = Remote::new(&cfg.server_url, "")?.latest_release().await?;
    offer_from(&dir, &info)
}

#[tauri::command]
pub async fn download_update(app: tauri::AppHandle) -> Result<DownloadDone, String> {
    let _guard = busy_guard()?;
    let (dir, cfg) = super::load_config()?;
    let client = Remote::download_client(&cfg.server_url)?;
    let info = client.latest_release().await?;
    let platform = updater::this_platform();
    if info.status != "released"
        || !updater::version_is_newer(updater::current_version(), &info.version)
    {
        return Err("Новой версии нет".into());
    }
    if !updater::valid_version(&info.version) {
        return Err("Номер версии не подходит".into());
    }
    let file = info
        .file(platform)
        .ok_or("Для этой системы нет файла обновления")?;
    if file.platform != platform {
        return Err("Для этой системы нет файла обновления".into());
    }
    if file.size > MAX_RELEASE_BYTES {
        return Err("Файл обновления слишком большой".into());
    }
    let expected = u64::try_from(file.size).map_err(|_| "Размер файла обновления не совпал")?;
    let exe = std::env::current_exe().unwrap_or_else(|_| dir.join("flammebible"));
    let place = place(std::env::consts::OS, &exe, &dir, &info.version, platform)?;
    if place.portable {
        let _ = fs::remove_file(marker_path(&exe));
    }
    if let Some(parent) = place.partial.parent() {
        if !parent.as_os_str().is_empty() {
            fs::create_dir_all(parent).map_err(|err| err.to_string())?;
        }
    }
    let emit = app.clone();
    let url = client.release_url(&info.version, platform);
    let header = updater::stream_to(
        client.http(),
        &url,
        &place.partial,
        expected,
        move |received, total| {
            let _ = emit.emit(
                "update-progress",
                UpdateProgress {
                    received,
                    total,
                    percentage: updater::percent(received, total),
                },
            );
        },
    )
    .await?;
    commit_verified(&place.partial, &place.final_path, &header, &file.sha256)?;
    Ok(DownloadDone {
        path: place.final_path.display().to_string(),
        portable: place.portable,
    })
}

#[tauri::command]
pub async fn install_update() -> Result<String, String> {
    let _guard = busy_guard()?;
    let exe = std::env::current_exe().map_err(|_| "Не найден файл программы".to_string())?;
    if !can_swap(std::env::consts::OS, &exe) {
        return Err("На этой системе файл нужно заменить вручную".into());
    }
    let (_dir, cfg) = super::load_config()?;
    let info = Remote::new(&cfg.server_url, "")?.latest_release().await?;
    let platform = updater::this_platform();
    if info.status != "released"
        || !updater::version_is_newer(updater::current_version(), &info.version)
    {
        return Err("Новой версии нет".into());
    }
    let file = info
        .file(platform)
        .ok_or("Для этой системы нет файла обновления")?;
    arm(&exe, &info.version, &file.sha256)?;
    Ok("Замена назначена. Закройте программу и откройте её снова.".into())
}

fn write_atomic(path: &Path, bytes: &[u8]) -> Result<(), String> {
    let tmp = partial_suffix(path);
    {
        let mut file = File::create(&tmp).map_err(|err| err.to_string())?;
        file.write_all(bytes).map_err(|err| err.to_string())?;
        file.sync_all().map_err(|err| err.to_string())?;
    }
    fs::rename(&tmp, path).map_err(|err| err.to_string())
}

#[cfg(unix)]
fn mark_executable(path: &Path) -> Result<(), String> {
    use std::os::unix::fs::PermissionsExt;
    let mut perms = fs::metadata(path)
        .map_err(|err| err.to_string())?
        .permissions();
    perms.set_mode(0o755);
    fs::set_permissions(path, perms).map_err(|err| err.to_string())
}

#[cfg(not(unix))]
fn mark_executable(_path: &Path) -> Result<(), String> {
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::paths::TmpDir;

    #[test]
    fn windows_portable_sits_beside_the_exe() {
        let exe = Path::new("/home/choir/Desktop/FlammeBible.exe");
        let place = place("windows", exe, Path::new("/tmp/cfg"), "0.2.0", "windows").unwrap();
        assert!(place.portable);
        assert_eq!(
            place.final_path,
            Path::new("/home/choir/Desktop/FlammeBible.exe.new")
        );
        assert_eq!(
            place.partial,
            Path::new("/home/choir/Desktop/FlammeBible.exe.partial")
        );
        let dev = Path::new("/home/choir/app/target/release/flammebible-desktop.exe");
        assert!(is_dev_binary(dev));
        assert!(!can_swap("windows", dev));
        assert!(!can_swap("linux", exe));
    }

    #[test]
    fn other_systems_save_under_the_config_dir() {
        let exe = Path::new("/home/choir/FlammeBible");
        let saved = place("linux", exe, Path::new("/tmp/cfg"), "0.2.0", "linux").unwrap();
        assert!(!saved.portable);
        assert_eq!(
            saved.final_path,
            Path::new("/tmp/cfg/updates/0.2.0/flammebible-linux")
        );
        assert!(place("linux", exe, Path::new("/tmp/cfg"), "../0.2.0", "linux").is_err());
    }

    #[test]
    fn commit_drops_a_file_that_fails_the_checksum() {
        let dir = TmpDir::new("commit");
        let partial = dir.path.join("file.partial");
        let final_path = dir.path.join("file.bin");
        fs::write(&partial, b"nope").unwrap();
        let hash = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad";
        assert!(commit_verified(&partial, &final_path, hash, hash).is_err());
        assert!(!partial.exists());
        assert!(!final_path.exists());
    }

    #[test]
    fn commit_renames_a_matching_file() {
        let dir = TmpDir::new("commit-ok");
        let partial = dir.path.join("file.partial");
        let final_path = dir.path.join("file.bin");
        fs::write(&partial, b"abc").unwrap();
        let hash = updater::hash_file(&partial).unwrap();
        commit_verified(&partial, &final_path, &hash, &hash).unwrap();
        assert_eq!(fs::read(&final_path).unwrap(), b"abc");
        assert!(!partial.exists());
    }

    #[test]
    fn arm_rejects_a_different_hash_and_apply_needs_a_marker() {
        let dir = TmpDir::new("arm");
        let exe = dir.path.join("FlammeBible.exe");
        fs::write(&exe, b"old").unwrap();
        let staged = staged_path(&exe);
        fs::write(&staged, b"new").unwrap();
        let err = arm(&exe, "0.2.0", &"ab".repeat(32)).unwrap_err();
        assert!(err.contains("контрольной суммой"));
        assert!(!marker_path(&exe).exists());
        assert!(!staged_path(&exe).exists());
        fs::write(&exe, b"old").unwrap();
        assert!(matches!(apply(&exe).unwrap(), Apply::None));
        assert_eq!(fs::read(&exe).unwrap(), b"old");
    }

    #[test]
    fn apply_swaps_on_the_next_start_and_rolls_back() {
        let dir = TmpDir::new("apply");
        let exe = dir.path.join("FlammeBible.exe");
        fs::write(&exe, b"old").unwrap();
        fs::write(staged_path(&exe), b"new").unwrap();
        let hash = updater::hash_file(&staged_path(&exe)).unwrap();
        arm(&exe, "0.2.0", &hash).unwrap();
        let (staged, armed, _) =
            stage_state("windows", &exe, &dir.path, "0.2.0", "windows", &hash).unwrap();
        assert!(staged && armed);
        assert!(matches!(apply(&exe).unwrap(), Apply::Swapped));
        assert_eq!(fs::read(&exe).unwrap(), b"new");
        assert_eq!(fs::read(backup_path(&exe)).unwrap(), b"old");
        assert!(!marker_path(&exe).exists());
        assert!(!staged_path(&exe).exists());

        let broken = dir.path.join("other.exe");
        fs::write(&broken, b"keep").unwrap();
        let err = swap_files(
            &broken,
            &dir.path.join("missing.new"),
            &backup_path(&broken),
        )
        .unwrap_err();
        assert!(!err.is_empty());
        assert_eq!(fs::read(&broken).unwrap(), b"keep");
        assert!(!backup_path(&broken).exists());
    }

    #[test]
    fn staged_report_ignores_a_different_hash() {
        let dir = TmpDir::new("stage");
        let exe = dir.path.join("FlammeBible.exe");
        fs::write(&exe, b"old").unwrap();
        let place = place("windows", &exe, &dir.path, "0.2.0", "windows").unwrap();
        fs::write(&place.final_path, b"new").unwrap();
        let hash = updater::hash_file(&place.final_path).unwrap();
        let (staged, armed, path) =
            stage_state("windows", &exe, &dir.path, "0.2.0", "windows", &hash).unwrap();
        assert!(staged);
        assert!(!armed);
        assert_eq!(path, place.final_path.display().to_string());
        let (staged, _, _) = stage_state(
            "windows",
            &exe,
            &dir.path,
            "0.2.0",
            "windows",
            &"cd".repeat(32),
        )
        .unwrap();
        assert!(!staged);
    }
}
