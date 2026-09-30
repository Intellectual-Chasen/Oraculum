package core

import (
	"strconv"
	"strings"
)

// logonIdDerivation は Logon ID の正規化値の導き方である。分析者が画面で読む値であるため
// 日本語で書く。
const logonIdDerivation = "0x に続く 16 進の Logon ID を、小文字で先頭の 0 を除いた文字列にした"

// logonIdPrefix は Logon ID の文字列が 16 進であることを示す接頭辞である。原資料の文字列の大文字と
// 小文字を問わない。
const logonIdPrefix = "0x"

// LogonIdValue は Logon ID の値を組む。rawText は原資料の文字列、text は原資料の文字列から
// 引用符を外した文字列である。
//
// **比べる形は、0x に続く小文字の 16 進で、先頭に 0 を置かない文字列である。** 同じ
// ログオンセッションを、Windows イベントログは 0 を詰めた 16 桁で、別の入力形式は 0 を
// 詰めずに書く。値 0 は 0x0 である。
//
// 0x に 64 bit までの 16 進の数字が続く形でない文字列は、定義の範囲の外の値として原資料の文字列だけを
// 保つ。
// **読めない文字列を比べる値にしない。**
func LogonIdValue(rawText, text string) RawAndNormalized {
	if normalized, readable := normalizedLogonId(text); readable {
		// 状態は present、原資料の文字列と正規化値と導き方を持つため検査が失敗しない。
		value, _ := NewNormalizedValue(ValueStatePresent, rawText, normalized, logonIdDerivation)
		return value
	}
	// 状態は out_of_definition で原資料の文字列を持つため検査が失敗しない。
	value, _ := NewRawValue(ValueStateOutOfDefinition, rawText)
	return value
}

// normalizedLogonId は Logon ID の文字列を比べる形へ直す。
func normalizedLogonId(text string) (string, bool) {
	if len(text) <= len(logonIdPrefix) || !strings.EqualFold(text[:len(logonIdPrefix)], logonIdPrefix) {
		return "", false
	}
	// ParseUint は符号と区切りの文字列を 16 進の数字として受け付けない。
	number, err := strconv.ParseUint(text[len(logonIdPrefix):], 16, 64)
	if err != nil {
		return "", false
	}
	return logonIdPrefix + strconv.FormatUint(number, 16), true
}

// LogonSessionRejectionReason は、Logon ID が一致したログオンと操作を関係にしなかった理由で
// ある。
type LogonSessionRejectionReason string

// LogonSessionRejectionReason の値。
const (
	// LogonSessionRejectionOtherTerminal は、ログオンと操作を記録した端末が違う組である。
	// Logon ID は端末の中でだけ一意である。
	LogonSessionRejectionOtherTerminal LogonSessionRejectionReason = "other_terminal"
	// LogonSessionRejectionOtherSource は、同じ端末のログオンと操作を、別の収集元が記録した組で
	// ある。Logon ID は 1 つの収集元の中でだけ比べる。同じ記録を写した 2 つの収集元のレコードを
	// 同じセッションの操作として結ばない。
	LogonSessionRejectionOtherSource LogonSessionRejectionReason = "other_source"
	// LogonSessionRejectionLogonAfterOperation は、ログオンの時刻が操作の時刻より後の組で
	// ある。
	LogonSessionRejectionLogonAfterOperation LogonSessionRejectionReason = "logon_after_operation"
	// LogonSessionRejectionTimeNotComparable は、2 つの時刻の前後を比べられない組である。
	// 片方の時刻を読めないときと、時点を持つ時刻と UTC からのずれを持たない壁時計の日時の
	// 組が該当する。
	LogonSessionRejectionTimeNotComparable LogonSessionRejectionReason = "time_not_comparable"
)

// IsKnown は契約が定める値かを返す。
func (r LogonSessionRejectionReason) IsKnown() bool {
	switch r {
	case LogonSessionRejectionOtherTerminal, LogonSessionRejectionOtherSource,
		LogonSessionRejectionLogonAfterOperation, LogonSessionRejectionTimeNotComparable:
		return true
	default:
		return false
	}
}

// LogonSessionRejection は、操作のレコードと Logon ID が一致しながら、関係にしなかった
// ログオンのレコード 1 件と、その理由である。
type LogonSessionRejection struct {
	// Reason は関係にしなかった理由である。
	Reason LogonSessionRejectionReason `json:"reason"`
	// Logon はログオンのレコードである。
	Logon GraphEvidence `json:"logon"`
}

// Validate は項目の整合を確かめる。
func (r LogonSessionRejection) Validate() error {
	if err := requireKnownEnum("LogonSessionRejection.reason", r.Reason); err != nil {
		return err
	}
	if err := r.Logon.Validate(); err != nil {
		return itemError("LogonSessionRejection.logon", err)
	}
	return nil
}
