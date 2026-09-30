#!/usr/bin/env bash
# pipeline-binding-boundary.sh — backend/pipeline/ の binding の境界を検査する。
#
# 規則: pipeline/ の binding_*.go 以外の production file は、binding_*.go が宣言した識別子を
# 名前で呼ばない。入力形式に依る値は ParserIdentity と port を通して渡す。
# depguard は import だけを見るため、同じ package の中で非 binding file が
# binding_*.go の宣言を名前で呼ぶ形を検出できない。その形は adapter を削除したときに
# 非 binding file の build が止まる依存であり、import と同じ境界違反である。
#
# 使い方: scripts/pipeline-binding-boundary.sh
# 出力: 違反を 1 行 1 件で stdout に印字。違反があれば exit 1、なければ exit 0。
set -euo pipefail

ROOT=$(git rev-parse --show-toplevel 2>/dev/null) || {
  echo "pipeline-binding-boundary.sh: git repo の中で実行されていません。検査未実行のため push しないこと。" >&2
  exit 1
}
cd "$ROOT"

PIPELINE_DIR="backend/pipeline"
if [ ! -d "$PIPELINE_DIR" ]; then
  echo "pipeline-binding-boundary.sh: $PIPELINE_DIR がありません。検査未実行のため push しないこと。" >&2
  exit 1
fi

mapfile -t BINDING_FILES < <(find "$PIPELINE_DIR" -maxdepth 1 -name 'binding_*.go' ! -name '*_test.go' | sort)
if [ "${#BINDING_FILES[@]}" -eq 0 ]; then
  echo "pipeline-binding-boundary.sh: $PIPELINE_DIR に binding_*.go がありません。検査未実行のため push しないこと。" >&2
  exit 1
fi

# binding_*.go が package の最上位で宣言した識別子を集める。
#
# method の名前は集めない。receiver の型の名前が別の宣言として集まり、method の名前は
# 相手の型を伴わずに呼べないためである。grouped な const / var の要素は、括弧の中で
# tab 1 つに字下げされ、名前から始まる行を取る。
extract_declarations() {
  awk '
    /^func [A-Za-z_]/ {
      name = $2
      sub(/[(].*$/, "", name)
      print name
      next
    }
    /^type [A-Za-z_]/ { print $2; next }
    /^(const|var) [A-Za-z_]/ { print $2; next }
    /^(const|var) \($/ { group = 1; next }
    group && /^\)/ { group = 0; next }
    group && /^\t[A-Za-z_][A-Za-z0-9_]*[ \t]/ {
      name = $1
      print name
      next
    }
  ' "$@" | sort -u
}

mapfile -t DECLARATIONS < <(extract_declarations "${BINDING_FILES[@]}")
if [ "${#DECLARATIONS[@]}" -eq 0 ]; then
  echo "pipeline-binding-boundary.sh: binding_*.go から宣言を 1 件も読めませんでした。検査未実行のため push しないこと。" >&2
  exit 1
fi

mapfile -t OTHER_FILES < <(
  find "$PIPELINE_DIR" -maxdepth 1 -name '*.go' ! -name 'binding_*.go' ! -name '*_test.go' | sort
)
if [ "${#OTHER_FILES[@]}" -eq 0 ]; then
  echo "pipeline-binding-boundary.sh: $PIPELINE_DIR に binding 以外の .go がありません。検査未実行のため push しないこと。" >&2
  exit 1
fi

PATTERN=$(printf '%s\n' "${DECLARATIONS[@]}" | paste -sd '|' -)

# 行コメントと二重引用符の文字列を外してから名前の一致を調べる。binding が宣言した識別子と
# 同じ文字列が、原資料の欄の名前として文字列に出る。file 名と行番号を残すため、
# grep へ渡す前に awk が `path:行番号:本文` の形へ直す。
#
# 外すのは行コメントと二重引用符の文字列だけである。block コメントは残って誤検出になり、
# `//` を含む文字列は行の残りごと外れて検出漏れになる。どちらかを実際に踏んだときに、
# Go の字句解析を使う形へ直す。
FOUND=$(awk '
  {
    line = $0
    gsub(/"[^"]*"/, "", line)
    sub(/\/\/.*/, "", line)
    print FILENAME ":" FNR ":" line
  }
' "${OTHER_FILES[@]}" | grep -Ew "($PATTERN)" || true)
if [ -n "$FOUND" ]; then
  printf '%s\n' "$FOUND" | while IFS= read -r line; do
    echo "[pipeline-binding-boundary] 非 binding file が binding_*.go の宣言を呼んでいます: ${line}"
  done
  echo "[pipeline-binding-boundary] 入力形式への依存は binding_*.go に置き、ParserIdentity か port を通して渡してください"
  exit 1
fi
exit 0
