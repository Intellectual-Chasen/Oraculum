// Package winevent は Windows イベントログの 1 件を読み、取り込みの項目へ直す。
//
// Hexagonal Layer: adapters。依存先は標準ライブラリ、backend/core、EVTX を読むライブラリ
// (www.velocidex.com/golang/evtx) である。
// External Tool: 無し。呼び出し側が開いた収集元を io.Reader で受け取る。
//
// 本 package が読める入力形式は Formats が宣言する。読み取りの形式ごとの走査器は、
// 1 件を形式に依らない Event へ直す。意味の対応は semantic_mapping.go の 1 か所が持ち、
// 読み取りの形式ごとに複製しない。
//
// Limitations: XML の走査器は UTF-8 の file だけを読む。UTF-16 で書いた file は ASCII の
// `<Event` を 2 byte ずつで書くため `<Event` の byte 列に一致せず、file 全体が件の外の byte の
// 失敗 1 件になる。CSV の走査器は UTF-8 で
// 日本語の見出しを書いた file だけを読む。ほかの見出しの file は見出しの失敗になる。
// EVTX の走査器の原文は、ライブラリが組んだ構造から書いた XML である (renderEventXML)。
// ライブラリは属性と子要素を区別せず、xmlns の属性と、名前ごとの値へ直した EventData の
// `<Binary>` を除く。
package winevent
