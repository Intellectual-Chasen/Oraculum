// Package apache は Apache HTTP Server のアクセスログとエラーログのレコードを文字列へ分け、
// 時刻と要求行を解釈する。
//
// Hexagonal Layer: adapters。依存先は標準ライブラリと backend/core である。
// External Tool: 無し。呼び出し側が開いた収集元を io.Reader で受け取る。
//
// 本 package が読める入力形式は Formats が宣言する。AccessReader はアクセスログを、
// ErrorReader はエラーログを読む。どちらも欄の並びを固定で持ち、取り込みの指定からは
// 受け取らない。
//
// Limitations: アクセスログは combined の 9 項目
// (%h %l %u %t "%r" %>s %b "%{Referer}i" "%{User-Agent}i") に固定する。他の LogFormat の
// 指定は対応しない。エラーログは
// [%{u}t] [%-m:%l] [pid %P:tid %T] [client %a] %M の並びに固定する。
// 改行は LF と CR LF。文字コードを自動判定せず、byte 列を保持する。
//
// **Squid の指定子の解釈を流用しない。** %h / %b / %{...}i は Apache 自身の並びとして
// 独立に文字列に分割し、診断の説明も Apache を名乗る。
package apache
