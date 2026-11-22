# Инструментарий сборки клиента: Rust 1.90, Node 20, зависимости Tauri и cargo-xwin.
# Исходники в образ не копируются — их монтирует deploy/build-release.sh.
FROM rust:1.90-slim-bookworm

ARG NODE_VERSION=20.19.5

ENV DEBIAN_FRONTEND=noninteractive \
    APPIMAGE_EXTRACT_AND_RUN=1 \
    CARGO_INCREMENTAL=0 \
    CARGO_TERM_COLOR=always

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        build-essential \
        ca-certificates \
        clang \
        curl \
        desktop-file-utils \
        file \
        libayatana-appindicator3-dev \
        libgtk-3-dev \
        librsvg2-dev \
        libssl-dev \
        libwebkit2gtk-4.1-dev \
        libxdo-dev \
        lld \
        llvm \
        pkg-config \
        python3 \
        patchelf \
        rsync \
        wget \
        xz-utils \
    && rm -rf /var/lib/apt/lists/*

RUN curl -fsSL "https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-x64.tar.xz" \
    | tar -xJ -C /usr/local --strip-components=1 \
    && node --version && npm --version

# cargo-xwin ставится в образ, реестр после установки чистится, чтобы не раздувать слои.
RUN cargo install --locked cargo-xwin \
    && rustup target add x86_64-pc-windows-msvc \
    && rm -rf /usr/local/cargo/registry /usr/local/cargo/git /usr/local/cargo/.package-cache \
    && cargo xwin --help >/dev/null

COPY build-client.sh set_version.py /usr/local/bin/
RUN chmod +x /usr/local/bin/build-client.sh \
    && chmod +r /usr/local/bin/set_version.py

WORKDIR /work
ENTRYPOINT ["/usr/local/bin/build-client.sh"]
