// Package assist は、分析者の端末で動く AI 支援の中継 (`oraculum-assist`) である。
//
// Hexagonal Layer: assist。依存先は標準ライブラリ、backend/core、backend/output、
// github.com/modelcontextprotocol/go-sdk である。pipeline、api、adapters を import せず、
// 外部 process を直接起動しない。提供者 (Claude の CLI など) は Provider の port を満たす
// adapter を cmd が登録する。
// External Tool: 無し。提供者の CLI の起動は、登録された adapter が backend/tooling で行う。
//
// 中継は次を行う。
//   - browser の画面と `/api/v0/` を、起動引数の Oraculum server の 1 つの origin へ転送する。
//     会話の endpoint (`/api/v0/conversations`) への browser からの要求は転送しない。
//   - `/assist/` の下で会話を答える。会話の本文と event は中継のメモリだけに置く。
//   - 会話ごとの bearer secret で守る MCP の待ち受けで、LLM に Oraculum の tool を渡す。tool は
//     server の会話の endpoint を専用の client で呼ぶ。
//
// **調査の状態と判定 (検索の条件の検証、根拠の検査、許可) を持たない。** 判定は server が行う。
//
// Limitations: 会話と event は中継を止めると消える。browser との接続が切れても応答が終わるまで
// 提供者を止めない。`Host` と `Origin` の検査は loopback の待ち受けを前提とし、端末の他の利用者
// から中継を守らない。
package assist
