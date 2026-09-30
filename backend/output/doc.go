// Package output は端末出力と診断ログの文字列を無害化する。
//
// Hexagonal Layer: output。依存先は標準ライブラリと backend/core に限る。
//
// External Tool: 無し。外部プロセスを起動しない。
//
// Limitations: 元の \xNN 表記と変換結果を区別できない。無害化の対象は制御文字と
// 不正な UTF-8 byte に限る。ANSI escape の引数部分など、制御文字以外の有効な文字は保持する。
// Fprintf と Fprintln は string、名前付き string 型、[]byte、error、fmt.Stringer の
// 引数を包み、書式適用後の文字列を無害化する。他の型はそのまま渡す。
// Fprintf の format は呼び出し側が用意する文字列で、無害化の対象に含めない。
// 包んだ引数への %T は wrapper の型名を表示する。元の型名を表示する呼び出し側は、
// 型名を文字列として用意してから渡す。
// 包んだ引数への %p は wrapper の関数アドレスを表示する。
// NewSanitizingLogger が無害化するのは message と属性の値で、属性の key は slog の
// handler の quote に任せる。
package output
