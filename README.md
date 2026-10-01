<img width="2303" height="1032" alt="Header" src="https://github.com/user-attachments/assets/fc26bc53-a6a7-41af-94ad-90bc86f4f886" />

Oraculum is a digital forensics investigation tool. It imports endpoint and network records, preserves their source context, and presents related observations for review.

## Requirements

- Go 1.26 or later
- Node.js 24
- pnpm 12.2.1
- Typst and Noto Sans JP are downloaded by the user-guide build script.

## Build and run

Install frontend dependencies and build the application:

```bash
pnpm -C frontend install --frozen-lockfile
pnpm -C frontend run build
go -C backend build ./...
```

Start the server with relative paths to your source records:

```bash
go -C backend run ./cmd/oraculum-server \
  --attack-rules rules/attack \
  'squid_combined:../oraculum-data/proxy/access.log' \
  'infotrace_mark_ii:../oraculum-data/endpoint.log'
```

The server listens on `127.0.0.1:8080`. Open that address in a browser. See the command help for supported input formats and options.

## User guide

Build the PDF guide with:

```bash
bash scripts/user-guide-build.sh
```

The generated PDF is written to `user-guide/build/oraculum-user-guide.pdf`.

## Verification

Install frontend dependencies, Go, Node.js, pnpm, and `golangci-lint`, then run:

```bash
bash scripts/public-ci.sh
```
