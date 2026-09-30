// Package investigationdb は調査 1 件の SQLite file を作成し、開き、読み書きする。
//
// Hexagonal Layer: adapters。依存先は標準ライブラリ、backend/core、modernc.org/sqlite である。
// External Tool: 無し。SQLite は CGO を使わない modernc.org/sqlite を process の中で動かす。
//
// file が持つのは、調査の識別子、取り込みの基準 directory、取り込みの指定
// (収集元ごとの取得元・入力形式・欄の並び・案件・sha256・取り込みの通番)、分析者の所見と
// その全改訂、分析者が与えた端末の割当、AI 支援の
// 記録 (提供者ごとの送信の許可の全改訂、会話、受け渡しの監査記録、AI 提案) である。値は表の列に置き、
// 入れ子の値は子の表に分ける。原資料の byte 列、パース結果、グラフは持たない。
//
// AI 支援の表は、最初に AI 支援の記録を書く transaction が作る。AI 支援の表を持たない file も
// 開き、記録が無いものとして読む。
//
// 開いた file は、閉じるまで 1 つの接続が排他で握る (locking_mode=EXCLUSIVE)。別の process は
// 同じ調査を開けない。書き込みは transaction 1 つで行い、途中で process が止まった書き込みは
// WAL の journal が巻き戻す。
//
// Limitations: 本 package が読む列を持たない file は、読み取りの失敗で開けない。本 package が
// 知らない列や表を足した file は開き、その列を読まずに書く。保存形式の移行を行わず、file を削除
// しない。読み戻した所見と割当と AI 支援の記録は core の Validate を通してから返し、通らない行が
// あれば file 全体を開かない。
package investigationdb
