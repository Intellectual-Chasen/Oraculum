// Package pipeline は取り込みの実行を持つ。
//
// Hexagonal Layer: pipeline (core と adapters に依存する)
//
// External Tool: 無し
//
// Limitations: 収集元 1 件の byte 列を全部メモリに保持する。
// 調査を開かない起動では、NewInMemoryOrdinals が実行ごとに取り込み通番を振る。
// 調査を開く起動は、調査に保存した通番を使う。
//
// pipeline は収集元を走査し、adapter を呼び、ImportStatus を完成させる。
// publicationState と withheldReason を決めるのはこの層である。
//
// adapter を import してよいのは binding_ で始まる file だけである。
// 他の file から backend/adapters/ を import しない。
//
// 端末への書き出しを持たない。診断の文字列の無害化は Config.Sanitize が受け取る関数が
// 行い、その関数を渡すのは cmd である。
package pipeline
