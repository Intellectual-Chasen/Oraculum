#!/usr/bin/env bash
# Vite が拡張子を補って解決する frontend の file に、大文字小文字だけでは
# 区別できない path が無いか調べる。衝突または一覧取得失敗時は exit 1 とし、
# 呼び出し元は push を止める。衝突は 1 組につき 1 行を stdout に出す。
set -euo pipefail

ROOT="${1:-}"
if [ -z "$ROOT" ]; then
  ROOT=$(git rev-parse --show-toplevel 2>/dev/null) || {
    echo "frontend-resolver-paths.sh: git repo の中で実行されていません。検査未実行のため push しないこと。" >&2
    exit 1
  }
fi

if ! git -C "$ROOT" rev-parse --show-toplevel >/dev/null 2>&1; then
  echo "frontend-resolver-paths.sh: '$ROOT' は git repo ではありません。検査未実行のため push しないこと。" >&2
  exit 1
fi

TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT
TRACKED="$TMP_DIR/tracked"
KEYED="$TMP_DIR/keyed"
SORTED="$TMP_DIR/sorted"

if ! git -C "$ROOT" ls-files -z -- frontend >"$TRACKED"; then
  echo "frontend-resolver-paths.sh: frontend の追跡 file 一覧を取得できません。検査未実行のため push しないこと。" >&2
  exit 1
fi

while IFS= read -r -d '' path; do
  filename=${path##*/}
  case "$filename" in
    *.[mM][jJ][sS]|*.[jJ][sS]|*.[mM][tT][sS]|*.[tT][sS]|*.[jJ][sS][xX]|*.[tT][sS][xX]|*.[jJ][sS][oO][nN]) ;;
    *) continue ;;
  esac

  stem=${path%.*}
  key=$(printf '%s' "$stem" | LC_ALL=C tr '[:upper:]' '[:lower:]')
  printf '%s\t%s\n' "$key" "$path" >>"$KEYED"
done <"$TRACKED"

if [ ! -s "$KEYED" ]; then
  exit 0
fi

LC_ALL=C sort -t $'\t' -k1,1 -k2,2 "$KEYED" >"$SORTED"

previous_key=""
previous_path=""
found=0
while IFS=$'\t' read -r key path; do
  if [ "$key" = "$previous_key" ]; then
    echo "[frontend-resolver-paths] Vite の module 解決が衝突します: $previous_path / $path"
    found=1
  fi
  previous_key=$key
  previous_path=$path
done <"$SORTED"

exit "$found"
