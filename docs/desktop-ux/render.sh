#!/bin/sh
# Рендер PNG рядом со скриптом: sh render.sh  (нужен google-chrome)
cd "$(dirname "$0")" || exit 1
r(){ google-chrome --headless=new --no-sandbox --disable-gpu --hide-scrollbars --force-device-scale-factor=1 --window-size=1100,800 --virtual-time-budget=800 --screenshot="$1.png" "file://$(pwd)/index.html#$2" >/dev/null 2>&1; }
r s1-sync            s1
r s2-conflicts       s2
r s3-settings        s3
r s4-blocker         s4
r p1-plans           p1
r p2-add-item        p2
r u1-updates         u1
r l1-login           l1
r c1-close-confirm   c1
