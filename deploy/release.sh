#!/bin/sh
# Регистрирует сборку программы на сервере без токена.
# Копирует файлы в контейнер api и вызывает flammebible-server release add.
set -eu

usage() {
	cat <<'USAGE'
Использование: deploy/release.sh [--version ВЕРСИЯ] --linux ФАЙЛ --windows ФАЙЛ
                                 [--macos ФАЙЛ] [--notes ТЕКСТ] [--publish]
                                 [--linux-sha256 ХЕШ] [--windows-sha256 ХЕШ]
                                 [--macos-sha256 ХЕШ]

Записывает файлы программы в том релизов и в базу. Токен администратора не нужен:
команда выполняется внутри контейнера api, у которого уже есть доступ к базе
и каталогу RELEASES_DIR.

Если --version не задан, берётся поле version из desktop/src-tauri/tauri.conf.json,
при его отсутствии — из desktop/src-tauri/Cargo.toml.

Файлы — пути на этой машине или адреса http(s). Без --publish версия остаётся
черновиком и публикуется отдельно, в админ-панели или повторным запуском
с теми же файлами и --publish.

Скрипт ищет docker-compose.yml рядом с каталогом deploy/. Другой каталог
задаётся переменной COMPOSE_DIR. Для Docker Compose передаётся --env-file:
сначала COMPOSE_ENV_FILE, затем /opt/flammebible/.env, затем .env в каталоге
над репозиторием и в самом репозитории. Без этого файла Compose подставляет
пароль базы по умолчанию.

Пример:

  ./deploy/release.sh --linux ./FlammeBible.AppImage --windows ./flammebible.exe \
      --notes "что нового" --publish
USAGE
}

if [ "${1:-}" = "-h" ] || [ "${1:-}" = "--help" ]; then
	usage
	exit 0
fi

version=""
notes=""
linux=""
windows=""
macos=""
linux_sha=""
windows_sha=""
macos_sha=""
publish=0

while [ "$#" -gt 0 ]; do
	case $1 in
	--version)
		version=$2
		shift 2
		;;
	--notes)
		notes=$2
		shift 2
		;;
	--linux)
		linux=$2
		shift 2
		;;
	--windows)
		windows=$2
		shift 2
		;;
	--macos)
		macos=$2
		shift 2
		;;
	--linux-sha256)
		linux_sha=$2
		shift 2
		;;
	--windows-sha256)
		windows_sha=$2
		shift 2
		;;
	--macos-sha256)
		macos_sha=$2
		shift 2
		;;
	--publish)
		publish=1
		shift
		;;
	-h|--help)
		usage
		exit 0
		;;
	*)
		echo "неизвестный аргумент: $1" >&2
		usage >&2
		exit 2
		;;
	esac
done

if [ -z "$linux" ] && [ -z "$windows" ] && [ -z "$macos" ]; then
	echo "нужен хотя бы один из --linux, --windows, --macos" >&2
	exit 2
fi

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
root=${COMPOSE_DIR:-$(CDPATH='' cd -- "$script_dir/.." && pwd)}
if [ ! -f "$root/docker-compose.yml" ]; then
	echo "нет $root/docker-compose.yml; укажите каталог с файлом в COMPOSE_DIR" >&2
	exit 2
fi

read_conf_version() {
	conf=$root/desktop/src-tauri/tauri.conf.json
	cargo=$root/desktop/src-tauri/Cargo.toml
	if [ -f "$conf" ] && command -v python3 >/dev/null 2>&1; then
		python3 -c 'import json,sys; print(json.load(open(sys.argv[1], encoding="utf-8"))["version"])' "$conf"
		return
	fi
	if [ -f "$conf" ]; then
		ver=$(sed -n 's/^[[:space:]]*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$conf" | sed -n '1p')
		if [ -n "$ver" ]; then
			printf '%s\n' "$ver"
			return
		fi
	fi
	if [ -f "$cargo" ]; then
		ver=$(sed -n 's/^[[:space:]]*version[[:space:]]*=[[:space:]]*"\([^"]*\)".*/\1/p' "$cargo" | sed -n '1p')
		if [ -n "$ver" ]; then
			printf '%s\n' "$ver"
			return
		fi
	fi
	return 1
}

if [ -z "$version" ]; then
	if ! version=$(read_conf_version); then
		echo "не задана --version, и не удалось прочитать её из desktop/src-tauri/tauri.conf.json или Cargo.toml" >&2
		exit 2
	fi
fi

# Файл переменных для Compose. На сервере он лежит рядом с каталогом repo/,
# а не внутри репозитория: без него пароль базы становится значением по умолчанию.
find_env_file() {
	if [ -n "${COMPOSE_ENV_FILE:-}" ]; then
		printf '%s\n' "$COMPOSE_ENV_FILE"
		return 0
	fi
	for candidate in /opt/flammebible/.env "$root/../.env" "$root/.env"; do
		if [ -f "$candidate" ]; then
			dir=$(CDPATH='' cd -- "$(dirname -- "$candidate")" && pwd)
			base=$(basename -- "$candidate")
			printf '%s\n' "$dir/$base"
			return 0
		fi
	done
	return 1
}

env_file=""
if ! env_file=$(find_env_file); then
	env_file=""
	echo "предупреждение: файл .env не найден, Docker Compose может подставить пароль базы по умолчанию" >&2
fi

compose() {
	if [ -n "$env_file" ]; then
		docker compose --project-directory "$root" --env-file "$env_file" "$@"
	else
		docker compose --project-directory "$root" "$@"
	fi
}

if ! compose exec -T api true >/dev/null 2>&1; then
	echo "контейнер api не запущен в $root; поднимите службы: docker compose up -d" >&2
	exit 1
fi

hosttmp=$(mktemp -d)
staging=/tmp/flammebible-release-$$
cleanup() {
	rm -rf "$hosttmp"
	compose exec -u root -T api rm -rf "$staging" >/dev/null 2>&1 || true
}
trap cleanup EXIT

compose exec -u root -T api mkdir -p "$staging"

copy_into() {
	src=$1
	name=$2
	fetched=$src
	case $src in
	http://*|https://*)
		fetched=$hosttmp/$name
		curl -fL --retry 3 --retry-delay 1 -o "$fetched" -- "$src"
		;;
	*)
		if [ ! -f "$src" ]; then
			echo "нет файла $src" >&2
			exit 2
		fi
		fetched=$src
		;;
	esac
	compose cp "$fetched" "api:$staging/$name"
}

if [ -n "$linux" ]; then
	copy_into "$linux" linux
fi
if [ -n "$windows" ]; then
	copy_into "$windows" windows
fi
if [ -n "$macos" ]; then
	copy_into "$macos" macos
fi

compose exec -u root -T api chown -R app:app "$staging"
compose exec -u root -T api chmod -R a+rX "$staging"

set -- release add --version "$version"
if [ -n "$linux" ]; then
	set -- "$@" --linux "$staging/linux"
fi
if [ -n "$windows" ]; then
	set -- "$@" --windows "$staging/windows"
fi
if [ -n "$macos" ]; then
	set -- "$@" --macos "$staging/macos"
fi
if [ -n "$notes" ]; then
	set -- "$@" --notes "$notes"
fi
if [ -n "$linux_sha" ]; then
	set -- "$@" --linux-sha256 "$linux_sha"
fi
if [ -n "$windows_sha" ]; then
	set -- "$@" --windows-sha256 "$windows_sha"
fi
if [ -n "$macos_sha" ]; then
	set -- "$@" --macos-sha256 "$macos_sha"
fi
if [ "$publish" -eq 1 ]; then
	set -- "$@" --publish
fi

compose exec -u app -T api flammebible-server "$@"
