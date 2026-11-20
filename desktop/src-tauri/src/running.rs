//! Запущена ли VisioBible. Список процессов читает `sysinfo`:
//! на Linux это `/proc/<pid>/cmdline`.

use std::ffi::OsString;

use sysinfo::{ProcessRefreshKind, ProcessesToUpdate, System, UpdateKind};

pub const CLOSE_MESSAGE: &str = "Закройте VisioBible и нажмите „Повторить“";

const EXE: &str = "visiobible.exe";

/// `true`, если процесс VisioBible уже есть.
/// Пустой снимок значит, что процессы прочитать не удалось: тогда пишем в лог и не блокируем запись.
pub fn visiobible_running() -> bool {
    let mut system = System::new();
    let updated = system.refresh_processes_specifics(
        ProcessesToUpdate::All,
        true,
        ProcessRefreshKind::nothing()
            .with_cmd(UpdateKind::Always)
            .without_tasks(),
    );
    if updated == 0 {
        eprintln!("не удалось прочитать список процессов");
        return false;
    }
    let windows = cfg!(windows);
    system.processes().values().any(|process| {
        let name = process.name().to_string_lossy();
        if windows {
            matches_visiobible(true, &name, "")
        } else {
            matches_visiobible(false, &name, &join_cmdline(process.cmd()))
        }
    })
}

/// Windows: имя процесса — `VisioBible.exe`.
/// Иначе имя или командная строка содержат `visiobible.exe` (так Wine показывает программу).
fn matches_visiobible(windows: bool, name: &str, cmdline: &str) -> bool {
    if windows {
        file_name(name).eq_ignore_ascii_case(EXE)
    } else {
        contains_exe(name) || contains_exe(cmdline)
    }
}

fn file_name(path: &str) -> &str {
    path.rsplit(['\\', '/']).next().unwrap_or(path)
}

fn contains_exe(value: &str) -> bool {
    value.to_ascii_lowercase().contains(EXE)
}

fn join_cmdline(cmd: &[OsString]) -> String {
    let mut out = String::new();
    for part in cmd {
        if !out.is_empty() {
            out.push('\0');
        }
        out.push_str(&part.to_string_lossy());
    }
    out
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn matches_name_or_cmdline_ignoring_case() {
        assert!(matches_visiobible(true, "VisioBible.exe", ""));
        assert!(matches_visiobible(
            true,
            r"C:\Program Files\VisioBible\VISIOBIBLE.EXE",
            ""
        ));
        assert!(!matches_visiobible(
            true,
            "wine",
            "C:\\VisioBible\\VisioBible.exe"
        ));
        assert!(!matches_visiobible(true, "notepad.exe", "visiobible.exe"));

        assert!(matches_visiobible(false, "VisioBible.exe", ""));
        assert!(matches_visiobible(
            false,
            "wine",
            "wine\0C:\\Program Files\\VisioBible\\VisioBible.exe\0"
        ));
        assert!(matches_visiobible(
            false,
            "wine-preloader",
            "/usr/lib/wine/wine64 VISIOBIBLE.EXE"
        ));
        assert!(!matches_visiobible(false, "wine", "notepad.exe"));
        assert!(!matches_visiobible(false, "visiobible", ""));
    }
}
