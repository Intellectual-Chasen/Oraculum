// Package winregistry は Windows の registry の hive (regf) と、同じ directory に置かれた
// transaction log (.LOG1 / .LOG2) を読む。
//
// Hexagonal Layer: adapters。依存先は標準ライブラリと backend/core である。
// External Tool: 無し。呼び出し側が主 file と log を連結した byte 列を io.Reader で渡し、
// 各 file の範囲を SetMembers で渡す。
//
// 本 package が読める入力形式は Formats が宣言する。hive の key 1 つを 1 件のレコードとして
// 返す。主 file の base block が書き出しの途中を示すとき (dirty)、Windows 8.1 以降の形式
// (HvLE) の log を適用した後の key と値を返す。log から来た cell の位置は log の file の
// byte を指す。
//
// Limitations: 旧形式 (DIRT) の log は適用しない。削除済みの cell、key の class 名、
// security descriptor (sk) は読まない。
package winregistry
