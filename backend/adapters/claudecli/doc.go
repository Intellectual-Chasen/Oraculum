// Package claudecli は、分析者本人がログインした Claude の CLI (`claude`) を子 process として
// 起動し、AI 支援の会話 1 件を行う。
//
// Hexagonal Layer: adapters。依存先は標準ライブラリ、backend/core、backend/tooling である。
// External Tool: `claude` を backend/tooling で起動する。起動の option は、組み込みの tool を
// 持たず (`--restricted --tools ""`)、Oraculum の MCP の tool だけを使う組
// (`--mcp-config` の file、`--strict-mcp-config`、`--allowedTools "mcp__oraculum__*"`、
// `--permission-mode dontAsk`) に固定する。会話は `-p --input-format stream-json
// --output-format stream-json --verbose` で 1 つの process に発言を続けて送る。
//
// **分析者の認証情報を読まず、保存せず、送らない。** 子 process に渡す環境変数は許可の一覧に
// 限り、`ANTHROPIC_API_KEY` などの API key を渡さない。課金先がサブスクリプションから変わる
// 経路を閉じる。MCP の bearer secret は argv に載せず、会話ごとの一時 directory の mode 0600 の
// file で渡す。
//
// Limitations: stream-json の event のうち、応答の文 (text)、tool の呼び出し (tool_use) の名前、
// 発言の終わり (result) だけを読み、他の event は読み飛ばす。CLI のバージョンで event の形が変わった
// ときは、応答の文が出ないか、発言の終わりを読めずに失敗になる。CLI の実機の挙動 (組み込みの
// tool を本当に使えないこと、サブスクリプションで動くこと) は分析者のログインで確かめる事項で
// あり、本 package の test は偽の CLI で確かめる。
package claudecli
