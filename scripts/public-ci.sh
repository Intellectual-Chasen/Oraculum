#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)
cd "$ROOT"

go -C backend build ./...
go -C backend vet ./...
go -C backend test -race ./...
(
  cd backend
  golangci-lint config verify
  golangci-lint run
)

pnpm -C frontend install --frozen-lockfile
pnpm -C frontend run typecheck
pnpm -C frontend run lint
pnpm -C frontend run test
pnpm -C frontend run build

bash scripts/pipeline-binding-boundary.sh
bash scripts/user-guide-build.sh
