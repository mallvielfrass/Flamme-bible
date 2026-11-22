#!/usr/bin/env python3
"""Пишет номер версии в файлы клиента. Только рабочая копия, не репозиторий."""
import json
import re
import sys
from pathlib import Path


def set_json_version(path: Path, version: str) -> None:
    data = json.loads(path.read_text(encoding="utf-8"))
    data["version"] = version
    path.write_text(json.dumps(data, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")


def set_cargo_version(path: Path, version: str) -> None:
    lines = path.read_text(encoding="utf-8").splitlines(keepends=True)
    out = []
    in_package = False
    done = False
    for line in lines:
        stripped = line.strip()
        if stripped.startswith("[") and stripped.endswith("]"):
            in_package = stripped == "[package]"
        if in_package and not done and re.match(r"^version\s*=", line):
            out.append(f'version = "{version}"\n')
            done = True
            continue
        out.append(line)
    if not done:
        raise SystemExit(f"в {path} нет version в [package]")
    path.write_text("".join(out), encoding="utf-8")


def main() -> None:
    if len(sys.argv) != 3:
        raise SystemExit("использование: set-version.py КАТАЛОГ_РЕПОЗИТОРИЯ ВЕРСИЯ")
    root = Path(sys.argv[1])
    version = sys.argv[2].strip()
    if not version:
        raise SystemExit("пустая версия")
    set_json_version(root / "desktop" / "package.json", version)
    set_json_version(root / "desktop" / "src-tauri" / "tauri.conf.json", version)
    set_cargo_version(root / "desktop" / "src-tauri" / "Cargo.toml", version)


if __name__ == "__main__":
    main()
