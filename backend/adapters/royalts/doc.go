// Package royalts は Royal TS/TSX 文書 (拡張子 .rtsz) の接続項目を読む。
//
// Hexagonal Layer: adapters。依存先は標準ライブラリと backend/core である。
// External Tool: 無し。呼び出し側が開いた収集元を io.Reader で受け取る。
//
// 本 package が読める入力形式は Formats が宣言する。文書は BOM 付き UTF-8 / CRLF の平文
// XML であり、zip ではない。1 文書が複数の `<RoyalRDSConnection>` 要素を持ち得るため、
// 要素ごとに 1 レコードとして返す。
//
// Limitations: `<RoyalRDSConnection>` 以外の接続種別 (SSH や VNC など) は対応しない。
// `<CredentialPassword>` は暗号文であっても読まない。本 package の Go の型に該当する
// field を持たせておらず、値を保持も出力もしない。
package royalts
