package core

// caseIdMaxLength は案件の識別子の byte 数の上限である。
const caseIdMaxLength = 64

// ValidateCaseId は案件の識別子の文字列を確かめる。
//
// 案件は、取り込みを求める側が収集元に付ける区分である。同じ観測対象を 1 つのノードに
// 保ったまま、関係を導く関連付けの範囲と根拠の件数を案件ごとに分ける。
//
// 文字列は英数字と `.`、`_`、`-` の 1 byte 以上 caseIdMaxLength byte 以下である。値を URL の
// 問い合わせと画面にそのまま載せるので、区切りと制御文字を持たない文字列に限る。
func ValidateCaseId(item, value string) error {
	if value == "" {
		return itemError(item, ErrMissingRequiredItem)
	}
	if len(value) > caseIdMaxLength {
		return itemError(item, ErrInvalid)
	}
	for _, r := range value {
		isLetter := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		isDigit := r >= '0' && r <= '9'
		if !isLetter && !isDigit && r != '.' && r != '_' && r != '-' {
			return itemError(item, ErrInvalid)
		}
	}
	return nil
}
