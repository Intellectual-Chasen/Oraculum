package output

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Sanitize は制御文字の UTF-8 byte と不正な UTF-8 byte を小文字の \xNN 表記へ変換する。
func Sanitize(text string) string {
	// 既知の制限: 元の \xNN 表記と変換結果を区別できない, 元の文字列 \x0a と改行が
	// 同じ出力になることを TestSanitize で確認, 診断表示から原資料を復元する要件が生じたら見直す
	const hex = "0123456789abcdef"
	var result strings.Builder
	for len(text) > 0 {
		r, size := utf8.DecodeRuneInString(text)
		if unicode.IsControl(r) || (r == utf8.RuneError && size == 1) {
			for _, b := range []byte(text[:size]) {
				result.WriteString(`\x`)
				result.WriteByte(hex[b>>4])
				result.WriteByte(hex[b&0x0f])
			}
		} else {
			result.WriteString(text[:size])
		}
		text = text[size:]
	}
	return result.String()
}
