#!/bin/sh
# Сборка клиента внутри образа. Каталог target после каждого шага удаляется.
set -eu

version=${CLIENT_VERSION:-}
while [ "$#" -gt 0 ]; do
	case $1 in
	--version)
		version=$2
		shift 2
		;;
	*)
		echo "неизвестный аргумент: $1" >&2
		exit 2
		;;
	esac
done

if [ -z "$version" ]; then
	echo "нужна версия: --version или CLIENT_VERSION" >&2
	exit 2
fi

if [ ! -d /src/desktop ]; then
	echo "в контейнер не смонтирован исходный код (/src/desktop)" >&2
	exit 2
fi

mkdir -p /work /out /cache/cargo /cache/npm /cache/xwin

echo "копирую исходники (без target, node_modules и .git)"
rsync -a --delete \
	--exclude '.git/' \
	--exclude 'node_modules/' \
	--exclude 'desktop/src-tauri/target/' \
	--exclude 'desktop/dist/' \
	--exclude 'web/node_modules/' \
	--exclude 'web/dist/' \
	/src/ /work/

python3 /usr/local/bin/set_version.py /work "$version"

export CARGO_HOME="${CARGO_HOME:-/cache/cargo}"
export CARGO_TARGET_DIR="${CARGO_TARGET_DIR:-/work/target}"
export XWIN_CACHE_DIR="${XWIN_CACHE_DIR:-/cache/xwin}"
export npm_config_cache="${npm_config_cache:-/cache/npm}"
export CARGO_INCREMENTAL=0
export CARGO_PROFILE_RELEASE_DEBUG="${CARGO_PROFILE_RELEASE_DEBUG:-0}"
export CARGO_PROFILE_RELEASE_STRIP="${CARGO_PROFILE_RELEASE_STRIP:-symbols}"
export CARGO_PROFILE_RELEASE_INCREMENTAL=false
export APPIMAGE_EXTRACT_AND_RUN=1
export CARGO_BUILD_JOBS="${CARGO_BUILD_JOBS:-1}"

cd /work/desktop
echo "npm ci"
npm ci

echo "сборка интерфейса"
npm run build

# Интерфейс уже собран, повторный beforeBuildCommand не нужен.
tauri_config='{"build":{"beforeBuildCommand":""}}'

echo "сборка Linux (AppImage)"
linux_ok=0
if npx tauri build --bundles appimage --config "$tauri_config"; then
	appimage=$(find "$CARGO_TARGET_DIR/release/bundle/appimage" -maxdepth 1 -type f -name '*.AppImage' 2>/dev/null | sed -n '1p' || true)
	if [ -n "$appimage" ] && [ -f "$appimage" ]; then
		cp -- "$appimage" /out/flammebible-linux
		chmod 755 /out/flammebible-linux
		echo "appimage" >/out/linux-format.txt
		linux_ok=1
		echo "Linux: AppImage $(wc -c < /out/flammebible-linux) байт"
	fi
fi

if [ "$linux_ok" -ne 1 ]; then
	raw=$CARGO_TARGET_DIR/release/flammebible-desktop
	if [ -f "$raw" ]; then
		cp -- "$raw" /out/flammebible-linux
		chmod 755 /out/flammebible-linux
		echo "raw" >/out/linux-format.txt
		echo "внимание: вместо AppImage записан обычный исполняемый файл (на компьютере пользователя нужен webkit2gtk-4.1)" >&2
	else
		echo "не удалось собрать файл Linux" >&2
		rm -rf "$CARGO_TARGET_DIR"
		exit 1
	fi
fi

echo "удаляю target после Linux, чтобы не держать два каталога сборки"
rm -rf "$CARGO_TARGET_DIR"

echo "сборка Windows (переносимый exe через cargo-xwin)"
npx tauri build --runner cargo-xwin --target x86_64-pc-windows-msvc --no-bundle --config "$tauri_config"
win=$CARGO_TARGET_DIR/x86_64-pc-windows-msvc/release/flammebible-desktop.exe
if [ ! -f "$win" ]; then
	echo "не найден $win" >&2
	rm -rf "$CARGO_TARGET_DIR"
	exit 1
fi
cp -- "$win" /out/flammebible-windows.exe
echo "Windows: exe $(wc -c < /out/flammebible-windows.exe) байт"

echo "удаляю target"
rm -rf "$CARGO_TARGET_DIR"
chmod a+r /out/flammebible-linux /out/flammebible-windows.exe /out/linux-format.txt
echo "готово: версия $version"
