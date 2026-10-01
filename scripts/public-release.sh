#!/usr/bin/env bash
# Build a Linux archive. Missing build inputs or a failed command stop without publishing an archive.
set -euo pipefail

if [ "$#" -ne 1 ]; then
  printf 'usage: scripts/public-release.sh <output.zip>\n' >&2
  exit 1
fi

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)
output=$1
case "$output" in
  /*) ;;
  *) printf 'output path must be absolute\n' >&2; exit 1 ;;
esac

for path in "$root/frontend/dist/index.html" "$root/user-guide/build/oraculum-user-guide.pdf" \
  "$root/backend/rules/attack" "$root/LICENSE"; do
  [ -e "$path" ] || { printf 'required build input is missing: %s\n' "${path#"$root"/}" >&2; exit 1; }
done

stage=$(mktemp -d)
trap 'rm -rf -- "$stage"' EXIT
bundle="$stage/oraculum-linux-amd64"
mkdir -p "$bundle/frontend" "$bundle/backend/rules"

GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
  go -C "$root/backend" build -trimpath -o "$bundle/oraculum-server" ./cmd/oraculum-server
cp -a "$root/frontend/dist" "$bundle/frontend/dist"
cp -a "$root/backend/rules/attack" "$bundle/backend/rules/attack"
cp -a "$root/user-guide" "$bundle/user-guide"
rm -rf -- "$bundle/user-guide/build/tools"
cp "$root/LICENSE" "$bundle/LICENSE"

mkdir -p "$(dirname "$output")"
python3 -m zipfile -c "$output" "$bundle"
