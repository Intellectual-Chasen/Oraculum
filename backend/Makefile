# backend (Go) の検査。repo root の Makefile から `make -C backend <target>` で呼ばれる。
#
# **tool のバージョンの定義元はここである。** CI の workflow に同じ値が書いてあるので、変えるときは
# 同じ commit で両方直す:
#   GOLANGCI_LINT_VERSION → .github/workflows/ci.yml
#   GOVULNCHECK_VERSION   → .github/workflows/ci.yml
#   どちらも              → .devcontainer/post-create.sh

SHELL := /usr/bin/env bash
.SHELLFLAGS := -euo pipefail -c

GOLANGCI_LINT_VERSION ?= v2.11.4
GOVULNCHECK_VERSION   ?= v1.6.0
PREFIX ?= /usr/local
DESTDIR ?=

.PHONY: help
help: ## この一覧を出す
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) \
	  | awk 'BEGIN {FS = ":.*?## "} {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

.PHONY: verify
verify: build vet test lint vuln ## build → vet → test -race → lint → govulncheck

.PHONY: build
build: ## go build ./...
	go build ./...

.PHONY: install-server
install-server: ## server と外部 ATT&CK rules を PREFIX 下へ配置する
	mkdir -p "$(DESTDIR)$(PREFIX)/bin" "$(DESTDIR)$(PREFIX)/share/oraculum/attack-rules"
	go build -o "$(DESTDIR)$(PREFIX)/bin/oraculum-server" ./cmd/oraculum-server
	cp rules/attack/*.yaml "$(DESTDIR)$(PREFIX)/share/oraculum/attack-rules/"

.PHONY: vet
vet: ## go vet ./...
	go vet ./...

# -race は必須。競合は「たまに失敗する test」としてしか現れず、
# ここで押さえないと再現に一番時間を食う型の不具合になる。
.PHONY: test
test: ## go test -race ./...
	go test -race ./...

# config verify を先に走らせる。`run` は config の schema を検査しないので、key の綴りを
# 間違えた設定が「指摘ゼロ」と同じ顔で通る。
.PHONY: lint
lint: ## golangci-lint config verify → golangci-lint run
	golangci-lint config verify
	golangci-lint run

.PHONY: vuln
vuln: ## govulncheck ./...
	govulncheck ./...

.PHONY: tools
tools: ## lint / vuln の tool をバージョンを固定して入れる (devcontainer の外で使うとき用)
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	go install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)
