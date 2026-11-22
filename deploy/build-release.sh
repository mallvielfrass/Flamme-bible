#!/bin/sh
# Собирает клиент Linux и Windows в Docker и при желании публикует его
# через deploy/release.sh. Токен не нужен.
set -eu

# Весь сценарий читается целиком до запуска: сборка идёт около часа, и если
# за это время таймер деплоя обновит файл, sh дочитал бы уже другой текст.
{

image_name=${CLIENT_BUILD_IMAGE:-flammebible-client-build}
cache_volume=${CLIENT_BUILD_CACHE:-flammebible-client-cache}

usage() {
	cat <<'USAGE'
Использование: deploy/build-release.sh [--version ВЕРСИЯ] [--notes ТЕКСТ] [--publish]
                                       [--build-only] [--clean]

Собирает программу для Linux (AppImage) и Windows (переносимый exe) в Docker,
затем вызывает deploy/release.sh. Токен администратора не нужен.

Если --version не задан, берётся поле version из desktop/src-tauri/tauri.conf.json.
Этот номер записывается в рабочую копию внутри контейнера, чтобы собранная
программа сообщала ту же версию, что и релиз.

--publish  сразу опубликовать версию (иначе черновик, если загрузка включена)
--build-only  только собрать файлы, не вызывать release.sh
--clean  удалить образ сборки и том кэша, ничего не собирать

Ограничения для слабого сервера (2 ядра, около 4 ГБ памяти, около 9 ГБ диска):

  BUILD_JOBS=1        параллельность cargo, по умолчанию 1
  BUILD_MEMORY=2g     лимит памяти контейнера сборки
  BUILD_CPUS=1        сколько ядер отдать сборке
  COMPOSE_DIR         каталог с docker-compose.yml
  COMPOSE_ENV_FILE    явный путь к .env для публикации

Скомпилированные библиотеки (каталог target для Linux и Windows) хранятся
в томе кэша, поэтому повторная сборка пересобирает только саму программу.
На диске это занимает около 9–12 ГБ: образ 2,5–3,5 ГБ, target 4–6 ГБ,
кэш Cargo/npm и Windows SDK 1,5–2 ГБ.

  KEEP_TARGET=0       не хранить target: около 4–5 ГБ, но каждая сборка
                      компилирует всё заново

Первая сборка на двух ядрах обычно занимает около часа, иногда до полутора.
Повторные с сохранённым target — несколько минут.

Если памяти мало, остановите контейнер модели (профиль semantic / embed)
на время сборки и при необходимости добавьте файл подкачки на 2 ГБ.
После сборки модель можно вернуть.

Чтобы освободить место после сборки или если диск закончился:

  ./deploy/build-release.sh --clean
  docker builder prune -f

Следующая сборка после --clean снова скачает зависимости.

Пример, из /opt/flammebible:

  ./repo/deploy/build-release.sh --version 1.2.0 --notes "что нового" --publish
USAGE
}

version=""
notes=""
publish=0
build_only=0
clean_only=0

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
	--publish)
		publish=1
		shift
		;;
	--build-only)
		build_only=1
		shift
		;;
	--clean)
		clean_only=1
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

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
root=${COMPOSE_DIR:-$(CDPATH='' cd -- "$script_dir/.." && pwd)}
if [ ! -f "$root/docker-compose.yml" ]; then
	echo "нет $root/docker-compose.yml; укажите каталог с файлом в COMPOSE_DIR" >&2
	exit 2
fi
if [ ! -f "$root/deploy/client-build/Dockerfile" ]; then
	echo "нет $root/deploy/client-build/Dockerfile" >&2
	exit 2
fi

if [ "$clean_only" -eq 1 ]; then
	echo "удаляю образ $image_name и том $cache_volume"
	docker rm -f flammebible-client-build >/dev/null 2>&1 || true
	docker image rm -f "$image_name" >/dev/null 2>&1 || true
	docker volume rm -f "$cache_volume" >/dev/null 2>&1 || true
	echo "готово. чтобы сбросить ещё и кэш сборщика образов: docker builder prune -f"
	exit 0
fi

if [ -z "$version" ]; then
	conf=$root/desktop/src-tauri/tauri.conf.json
	cargo=$root/desktop/src-tauri/Cargo.toml
	if [ -f "$conf" ] && command -v python3 >/dev/null 2>&1; then
		version=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1], encoding="utf-8"))["version"])' "$conf")
	elif [ -f "$conf" ]; then
		version=$(sed -n 's/^[[:space:]]*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$conf" | sed -n '1p')
	fi
	if [ -z "$version" ] && [ -f "$cargo" ]; then
		version=$(sed -n 's/^[[:space:]]*version[[:space:]]*=[[:space:]]*"\([^"]*\)".*/\1/p' "$cargo" | sed -n '1p')
	fi
	if [ -z "$version" ]; then
		echo "не задана --version, и не удалось прочитать её из desktop/src-tauri/tauri.conf.json или Cargo.toml" >&2
		exit 2
	fi
fi

if ! command -v docker >/dev/null 2>&1; then
	echo "нужен docker" >&2
	exit 1
fi

echo "собираю образ $image_name"
DOCKER_BUILDKIT=1 docker build -t "$image_name" "$root/deploy/client-build"

outdir=$(mktemp -d)
# shellcheck disable=SC2317 # вызывается из trap
cleanup() {
	rm -rf "$outdir"
}
trap cleanup EXIT

echo "сборка клиента $version (цель — /out в контейнере)"
# Том кэша хранит зависимости и каталог target (если KEEP_TARGET не 0).
docker volume create "$cache_volume" >/dev/null
docker rm -f flammebible-client-build >/dev/null 2>&1 || true
docker run --rm \
	--name flammebible-client-build \
	--memory="${BUILD_MEMORY:-2g}" \
	--cpus="${BUILD_CPUS:-1}" \
	-e CARGO_BUILD_JOBS="${BUILD_JOBS:-1}" \
	-e CARGO_HOME=/cache/cargo \
	-e KEEP_TARGET="${KEEP_TARGET:-1}" \
	-e CARGO_INCREMENTAL=0 \
	-e CARGO_PROFILE_RELEASE_DEBUG=0 \
	-e CARGO_PROFILE_RELEASE_STRIP=symbols \
	-e CARGO_PROFILE_RELEASE_INCREMENTAL=false \
	-e XWIN_CACHE_DIR=/cache/xwin \
	-e npm_config_cache=/cache/npm \
	-e APPIMAGE_EXTRACT_AND_RUN=1 \
	-e CLIENT_VERSION="$version" \
	-v "$cache_volume":/cache \
	-v "$root":/src:ro \
	-v "$outdir":/out \
	"$image_name" --version "$version"

linux=$outdir/flammebible-linux
windows=$outdir/flammebible-windows.exe
if [ ! -f "$linux" ] || [ ! -f "$windows" ]; then
	echo "сборка не дала оба файла: $linux и $windows" >&2
	exit 1
fi

if [ -f "$outdir/linux-format.txt" ]; then
	fmt=$(cat "$outdir/linux-format.txt")
	if [ "$fmt" = "raw" ]; then
		echo "внимание: Linux собран как обычный исполняемый файл, не AppImage" >&2
	fi
fi

echo "готовы файлы:"
echo "  Linux   $linux  ($(wc -c < "$linux") байт)"
echo "  Windows $windows  ($(wc -c < "$windows") байт)"

if [ "$build_only" -eq 1 ]; then
	keep=$(mktemp -d -t flammebible-client-XXXXXX)
	cp -- "$linux" "$keep/flammebible-linux"
	cp -- "$windows" "$keep/flammebible-windows.exe"
	if [ -f "$outdir/linux-format.txt" ]; then
		cp -- "$outdir/linux-format.txt" "$keep/linux-format.txt"
	fi
	chmod a+r "$keep"/flammebible-linux "$keep"/flammebible-windows.exe
	echo "файлы оставлены в $keep"
	exit 0
fi

release=$root/deploy/release.sh
if [ ! -x "$release" ]; then
	echo "нет исполняемого $release" >&2
	exit 1
fi

set -- --version "$version" --linux "$linux" --windows "$windows"
if [ -n "$notes" ]; then
	set -- "$@" --notes "$notes"
fi
if [ "$publish" -eq 1 ]; then
	set -- "$@" --publish
fi
echo "публикую через deploy/release.sh"
"$release" "$@"
exit
}
