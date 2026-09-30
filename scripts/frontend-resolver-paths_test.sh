#!/usr/bin/env bash
# frontend-resolver-paths.sh の検出範囲と、prepush から Go 差分判定より前に
# 呼ばれることを確認する。失敗時は exit 1 とし、呼び出し元は push を止める。
set -euo pipefail

ROOT=$(git rev-parse --show-toplevel 2>/dev/null) || {
  echo "frontend-resolver-paths_test.sh: git repo の中で実行されていません。" >&2
  exit 1
}
CHECKER="$ROOT/scripts/frontend-resolver-paths.sh"
TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

new_repo() {
  local name=$1
  local repo="$TMP_DIR/$name"
  mkdir -p "$repo"
  git -C "$repo" init -q
  printf '%s\n' "$repo"
}

add_index_path() {
  local repo=$1
  local path=$2
  local blob
  blob=$(printf '' | git -C "$repo" hash-object -w --stdin)
  git -C "$repo" update-index --add --cacheinfo 100644 "$blob" "$path"
}

expect_ok() {
  local repo=$1
  if ! output=$(bash "$CHECKER" "$repo" 2>&1); then
    echo "FAIL: 成功する入力で検査が失敗しました: $output" >&2
    exit 1
  fi
  if [ -n "$output" ]; then
    echo "FAIL: 成功する入力で予期しない出力がありました: $output" >&2
    exit 1
  fi
}

expect_collision() {
  local repo=$1
  shift
  local output
  if output=$(bash "$CHECKER" "$repo" 2>&1); then
    echo "FAIL: 衝突する入力を検出できませんでした。" >&2
    exit 1
  fi
  for expected in "$@"; do
    case "$output" in
      *"$expected"*) ;;
      *)
        echo "FAIL: 衝突結果に '$expected' がありません: $output" >&2
        exit 1
        ;;
    esac
  done
}

clean_repo=$(new_repo clean)
add_index_path "$clean_repo" "frontend/a/Panel.tsx"
add_index_path "$clean_repo" "frontend/b/panel.ts"
add_index_path "$clean_repo" "frontend/a/panel.vue"
expect_ok "$clean_repo"

component_repo=$(new_repo component)
add_index_path "$component_repo" "frontend/src/ListFilter.tsx"
add_index_path "$component_repo" "frontend/src/listFilter.ts"
expect_collision "$component_repo" "ListFilter.tsx" "listFilter.ts"

directory_repo=$(new_repo directory)
add_index_path "$directory_repo" "frontend/src/Navigation/Bookmarks.tsx"
add_index_path "$directory_repo" "frontend/src/navigation/bookmarks.ts"
expect_collision "$directory_repo" "Navigation/Bookmarks.tsx" "navigation/bookmarks.ts"

extension_repo=$(new_repo extensions)
add_index_path "$extension_repo" "frontend/src/Resolver.tsx"
for extension in mjs js mts ts jsx json; do
  add_index_path "$extension_repo" "frontend/src/resolver.$extension"
done
expect_collision "$extension_repo" \
  "resolver.mjs" "resolver.js" "resolver.mts" "resolver.ts" "resolver.jsx" "Resolver.tsx" "resolver.json"

if output=$(bash "$CHECKER" "$TMP_DIR/not-a-repo" 2>&1); then
  echo "FAIL: git repo でない入力を失敗として扱いませんでした。" >&2
  exit 1
fi
case "$output" in
  *"git repo ではありません"*) ;;
  *)
    echo "FAIL: 一覧を取得できない場合の説明が不足しています: $output" >&2
    exit 1
    ;;
esac

prepush="$ROOT/scripts/prepush-checks.sh"
self_test_line=$(grep -nF 'frontend-resolver-paths_test.sh' "$prepush" | head -1 | cut -d: -f1)
checker_line=$(grep -nF 'frontend-resolver-paths.sh' "$prepush" | head -1 | cut -d: -f1)
go_diff_line=$(grep -nF "DIFF=\$(git diff" "$prepush" | head -1 | cut -d: -f1)
if [ -z "$self_test_line" ] || [ -z "$checker_line" ] || [ -z "$go_diff_line" ] \
  || [ "$self_test_line" -ge "$go_diff_line" ] || [ "$checker_line" -ge "$go_diff_line" ]; then
  echo "FAIL: frontend path 検査が prepush の Go 差分判定より前に接続されていません。" >&2
  exit 1
fi

echo "PASS frontend-resolver-paths_test.sh"
