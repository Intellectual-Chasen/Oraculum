// Package core は port (interface) と型と pure な判定規則を持つ。
//
// Hexagonal Layer: core (依存なし)
//
// core は標準ライブラリと自身の package に依存する。I/O、時刻取得、外部 module を
// 導入しない。
// 機械で守っているのは backend/.golangci.yml の depguard `core-purity`。
package core
