package output

import (
	"io"
	"log/slog"
)

// sanitizingReplaceAttr は slog.HandlerOptions.ReplaceAttr 用に文字列と error の値を無害化する。
func sanitizingReplaceAttr(_ []string, a slog.Attr) slog.Attr {
	if a.Value.Kind() == slog.KindString {
		a.Value = slog.StringValue(Sanitize(a.Value.String()))
	} else if err, ok := a.Value.Any().(error); ok {
		a.Value = slog.StringValue(Sanitize(err.Error()))
	}
	return a
}

// NewSanitizingLogger は message と属性を無害化してから w へ書く logger を作る。
//
// slog の既定の handler は属性の値と key を quote する一方、message を与えられたまま書く。
// 外部由来の文字列が message に入ると、1 件の記録が複数行に割れて後続の行を偽装できる。
func NewSanitizingLogger(w io.Writer) *slog.Logger {
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{ReplaceAttr: sanitizingReplaceAttr}))
}
