package core

import (
	"encoding/base64"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

// EncodedCommandDecoding は 1 本のコマンド行に埋め込まれた、符号化された文字列の復号の
// 結果である。
//
// **原資料の文字列を持たない。** 原資料の文字列の置き場は RawAndNormalized の rawText であり、本型が
// 持つのは復号した文字列と、復号が成立したかである。
type EncodedCommandDecoding struct {
	// Encoded はコマンド行が符号化された文字列を持つことを表す。
	Encoded bool
	// Decoded は符号化された文字列を 1 つ以上復号できたことを表す。
	Decoded bool
	// CommandLine は復号できた文字列を復号後の文字列で置き換えたコマンド行である。
	// Decoded が偽のときは空の文字列である。
	CommandLine string
}

// encodedCommandSwitchName は、次の文字列が Base64 で符号化されたコマンド行であることを
// 宣言する switch の名前である。
const encodedCommandSwitchName = "encodedcommand"

// isEncodedCommandSwitch は文字列が encodedCommandSwitchName の switch かを返す。
//
// PowerShell は switch の名前を先頭から省略して書ける。接頭辞の一致で判定し、
// 先頭の区切り記号は - と / の両方を受け付ける。
//
// 既知の制限: encodedCommandSwitchName の接頭辞をすべて switch として受け付ける,
// PowerShell が省略した switch の名前を -EncodedCommand として解釈するため、
// 接頭辞に該当する文字列を符号化以外の意味で使うコマンド行を想定しない,
// 接頭辞に該当する文字列を符号化以外の意味で使うコマンド行を収集元で確認したとき、
// 受け付ける文字列をその形に絞る
func isEncodedCommandSwitch(token string) bool {
	name, found := strings.CutPrefix(strings.ToLower(token), "-")
	if !found {
		name, found = strings.CutPrefix(strings.ToLower(token), "/")
	}
	if !found || name == "" {
		return false
	}
	return strings.HasPrefix(encodedCommandSwitchName, name)
}

// base64UnitLength は Base64 が 1 単位に使う文字数である。
const base64UnitLength = 4

// minimumEncodedCommandLength は、switch を伴わない文字列を符号化された文字列として
// 扱う最小の長さである。
//
// UTF-16LE の 1 文字は 2 byte であり、Base64 は 3 byte を 4 文字で表す。24 文字は
// 復号すると 18 byte、UTF-16LE で 9 文字に相当する。
const minimumEncodedCommandLength = 24

// DecodeEncodedCommandLine はコマンド行に埋め込まれた Base64 の文字列を復号する。
//
// **入力形式を 1 つも知らない。** 受け取るのはコマンド行 1 本の文字列だけであり、
// どの収集元のどの欄から来たかは呼ぶ側が持つ。
//
// 復号の対象は、空白で区切った文字列の中にある Base64 の文字だけから成る部分である。
// 符号化された文字列の前後に付いた引用符・単引用符・セミコロンは復号の対象から外れる。
// 採る条件は、その文字列が符号化を宣言する switch に続くかで分かれる。
//
//	switch に続く文字列     : base64UnitLength 以上の長さで、復号結果が isReadableText を満たす
//	switch を伴わない文字列 : minimumEncodedCommandLength 以上の長さで、
//	                        復号結果が isPlainCommandText を満たす
//
// 宣言が無い文字列は本 package が符号化された文字列だと判断するため、厳しい条件を当てる。
// 判断を誤ると符号化されていない文字列が別の文字列へ置き換わる。
//
// 復号できた部分は復号後の文字列で置き換え、残りを原文字列のまま並べた行を CommandLine に
// 返す。switch があって続く文字列を復号できないときは、Encoded が真で Decoded が偽になる。
// 呼ぶ側はこの組み合わせを導出未確定として扱う。
func DecodeEncodedCommandLine(commandLine string) EncodedCommandDecoding {
	tokens := strings.Split(commandLine, " ")
	result := EncodedCommandDecoding{}
	switchSeen := false
	for index, token := range tokens {
		if isEncodedCommandSwitch(token) {
			switchSeen = true
			continue
		}
		minimumLength, accept := minimumEncodedCommandLength, isPlainCommandText
		if switchSeen {
			switchSeen = false
			result.Encoded = true
			minimumLength, accept = base64UnitLength, isReadableText
		}
		decoded, ok := decodeBase64Runs(token, minimumLength, accept)
		if !ok {
			continue
		}
		tokens[index] = decoded
		result.Encoded = true
		result.Decoded = true
	}
	// switch がコマンド行の末尾に出るとき、続く文字列が無い。
	if switchSeen {
		result.Encoded = true
	}
	if result.Decoded {
		result.CommandLine = strings.Join(tokens, " ")
	}
	return result
}

// decodeBase64Runs は文字列の中の Base64 の文字が続く部分を復号した文字列へ置き換える。
// ok が偽になるのは、条件を満たす部分を 1 つも復号できなかったときである。
func decodeBase64Runs(
	token string, minimumLength int, accept func(string) bool,
) (string, bool) {
	var replaced strings.Builder
	replaced.Grow(len(token))
	decodedAny := false
	for start := 0; start < len(token); {
		end := start
		for end < len(token) && isBase64Byte(token[end]) {
			end++
		}
		if end == start {
			replaced.WriteByte(token[start])
			start++
			continue
		}
		run := token[start:end]
		decoded, ok := "", false
		if end-start >= minimumLength {
			decoded, ok = decodeUtf16Base64(run, accept)
		}
		if ok {
			replaced.WriteString(decoded)
			decodedAny = true
		} else {
			replaced.WriteString(run)
		}
		start = end
	}
	if !decodedAny {
		return "", false
	}
	return replaced.String(), true
}

// isBase64Byte は Base64 の文字列を組む byte かを返す。
func isBase64Byte(b byte) bool {
	switch {
	case b >= 'A' && b <= 'Z', b >= 'a' && b <= 'z', b >= '0' && b <= '9':
		return true
	case b == '+', b == '/', b == '=':
		return true
	}
	return false
}

// decodeUtf16Base64 は Base64 の文字列を UTF-16LE の文字列として復号し、accept が受理した
// 結果だけを返す。
//
// ok が偽になるのは、Base64 として復号できない文字列、byte 数が奇数の復号結果、
// 空の復号結果、accept が受理しない復号結果である。
//
// **UTF-16LE 以外の符号化を試さない。** 対象は PowerShell の -EncodedCommand が定める
// 符号化であり、同 switch は UTF-16LE の Base64 だけを受け取る。
func decodeUtf16Base64(token string, accept func(string) bool) (string, bool) {
	raw, err := base64.StdEncoding.DecodeString(token)
	if err != nil || len(raw) == 0 || len(raw)%2 != 0 {
		return "", false
	}
	units := make([]uint16, 0, len(raw)/2)
	for index := 0; index < len(raw); index += 2 {
		units = append(units, uint16(raw[index])|uint16(raw[index+1])<<8)
	}
	decoded := string(utf16.Decode(units))
	if !utf8.ValidString(decoded) || !accept(decoded) {
		return "", false
	}
	return decoded, true
}

// isReadableText は、復号結果が分析者の読める文字列であるかを返す。
//
// 改行と水平タブを通し、他の制御文字と置換文字を拒む。**制御文字を含む復号結果を
// 通すと、任意の byte 列が復号結果として画面と log に出る。**
func isReadableText(text string) bool {
	for _, r := range text {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			continue
		case r == utf8.RuneError:
			return false
		case unicode.IsControl(r):
			return false
		}
	}
	return true
}

// isPlainCommandText は、復号結果が符号化を宣言しない文字列の復号結果として受理できる形かを
// 返す。ASCII の印字可能文字と改行と水平タブだけから成り、ASCII の英字を 1 文字以上持つ
// 文字列を受理する。
//
// **isReadableText だけでは足りない。** Base64 の文字だけから成る英単語と 16 進の文字列は、
// UTF-16LE として復号すると CJK の文字列になる。CJK は制御文字ではないため
// isReadableText を通り、符号化されていない文字列が別の文字列へ置き換わる。
//
// 既知の制限: 宣言の無い文字列の復号結果を ASCII の印字可能文字だけに限る,
// 本条件を外すと、32 文字の 16 進 (NTLM hash など) と Base64 の文字だけから成る英単語が
// CJK の文字列へ置き換わる。encoded_command_test.go のコマンド行で再現できる,
// ASCII の外の文字を含む符号化されたコマンド行を収集元で確認したとき、受理する文字の
// 範囲を広げる。宣言の無い文字列の側を広げると 16 進と英単語が再び対象に入る
func isPlainCommandText(text string) bool {
	letters := 0
	for _, r := range text {
		if r == '\n' || r == '\r' || r == '\t' {
			continue
		}
		if r < ' ' || r > '~' {
			return false
		}
		if unicode.IsLetter(r) {
			letters++
		}
	}
	return letters > 0
}
