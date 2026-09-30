// Package prefetch は Windows の Prefetch の file (拡張子 .pf) を読む。
//
// Hexagonal Layer: adapters。依存先は標準ライブラリと backend/core である。
// External Tool: 無し。呼び出し側が開いた収集元を io.Reader で受け取る。
//
// 本 package が読める入力形式は Formats が宣言する。1 つの file を 1 件のレコードとして返す。
// 形式の番号 17 / 23 / 26 / 30 / 31 を読み、`MAM` の見出しを持つ file は LZXpress Huffman
// (MS-XCA 2.2.4) で展開してから読む。
//
// Limitations: 参照した file と実行ファイルの path は `\VOLUME{…}\…` の形のまま保持し、
// ドライブ文字へ直さない。volume とドライブ文字の対応は Prefetch の外 (registry の
// MountedDevices) が持つ。
package prefetch
