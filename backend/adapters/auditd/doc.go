// Package auditd は Linux の auditd が書き出す監査ログを読む。
//
// 出力元は Linux の auditd である。1 つの事象が複数行に分かれ、同じ事象の行は
// `msg=audit(<秒>.<ミリ秒>:<連番>)` の値が一致する。本 package は連続する同じ値の行を
// 1 レコードにまとめて返す。
//
// 対応する auditd のバージョンと設定は未確認である。読んだ収集元に auditd のバージョンを示す記述が無い。
//
// Hexagonal Layer: adapters。依存先は標準ライブラリと backend/core である。
// backend/pipeline と backend/api と他の adapter を import しない。
//
// External Tool: 無し。外部プロセスを起動しない。
//
// Limitations:
//   - 同じ事象の行を集める範囲を、先頭の行から一定の行数に限る (scan.go の
//     eventLineWindow)。その範囲より後ろに現れた同じ鍵の行は、別のレコードになる。
//   - 監査ログは監査規則 (auditd.rules) の本体を持たない。`key=` が指す規則を
//     読めないため、事象がどの規則で記録されたかを判定しない。
//   - 欄の意味を語彙へ写すのは、SYSCALL と EXECVE と PATH と PROCTITLE と CWD の
//     `type` に限る。他の `type` の行も key=value の並びとして読み、原資料の key を
//     項目の名前に持つ。
//   - 0x1D の後ろの値が auditd のどの表から取り出されたかを確認していない。原資料の文字列として
//     保ち、値の出どころを判定しない。
package auditd
