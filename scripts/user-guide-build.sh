#!/usr/bin/env bash
# 利用者ガイドの PDF を user-guide/build/ に作る。Typst と書体は版と sha256 を固定して
# user-guide/build/tools/ に取得し、2 回目以降は取得済みのものを使う。
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
build="$root/user-guide/build"
tools="$build/tools"
mkdir -p "$tools"

typst_version=v0.15.1
typst_archive=typst-x86_64-unknown-linux-musl.tar.xz
typst_sha256=a6d077d0a95eed5a2eba715b2dae06be954f624ccbf85758a03f389ded33118c
font_archive=16_NotoSansJP.zip
font_sha256=2bbdd2c20f30670b39ca735c96d75f1fdabdb348103e43b820cf17701fd22b18

fetch() { # url file sha256
    if [[ ! -f "$tools/$2" ]] || ! echo "$3  $tools/$2" | sha256sum -c --status; then
        curl -fsSL -o "$tools/$2" "$1"
        echo "$3  $tools/$2" | sha256sum -c --quiet
    fi
}

fetch "https://github.com/typst/typst/releases/download/$typst_version/$typst_archive" "$typst_archive" "$typst_sha256"
fetch "https://github.com/notofonts/noto-cjk/releases/download/Sans2.004/$font_archive" "$font_archive" "$font_sha256"
[[ -x "$tools/typst" ]] || tar -xJf "$tools/$typst_archive" -C "$tools" --strip-components=1
[[ -d "$tools/fonts" ]] || python3 -m zipfile -e "$tools/$font_archive" "$tools/fonts"

"$tools/typst" compile --root "$root/user-guide" --font-path "$tools/fonts" --ignore-system-fonts \
    "$root/user-guide/main.typ" "$build/oraculum-user-guide.pdf"
echo "$build/oraculum-user-guide.pdf"
