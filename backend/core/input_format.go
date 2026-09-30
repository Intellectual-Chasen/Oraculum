package core

// FormatSpecInput は入力形式が欄の並びの指定を取るかである。
type FormatSpecInput string

// FormatSpecInput の値。
const (
	// FormatSpecInputRejected は、欄の並びを入力形式そのものが定める状態である。
	// 取り込みの指定が渡した並びを受け付けない。
	FormatSpecInputRejected FormatSpecInput = "rejected"
	// FormatSpecInputRequired は、欄の並びを取り込みの指定から受け取る状態である。
	// 指定の無い取り込みを受け付けない。
	FormatSpecInputRequired FormatSpecInput = "required"
)

// IsKnown は FormatSpecInput が定義の中の値であるかを返す。
func (f FormatSpecInput) IsKnown() bool {
	switch f {
	case FormatSpecInputRejected, FormatSpecInputRequired:
		return true
	default:
		return false
	}
}

// InputFormat は読める入力形式 1 つの宣言である。
//
// **値を宣言するのは入力形式を読む adapter である。** 本 package は宣言の形だけを定め、
// どの形式が読めるかを知らない。取り込みの実行は、渡された収集元の形式の識別子で宣言を
// 探し、宣言した adapter へ byte 列を渡す。
type InputFormat struct {
	// Key は入力形式を指す識別子である。取り込みを求める側が明示する値であり、
	// SourceIdentity.FormatKey になる。
	Key FormatKey
	// ParserID は収集元 1 件を読む走査器を指す識別子である。parserVersion の材料になる。
	ParserID string
	// PositionKind はこの形式のレコードの位置の指し方である。
	PositionKind PositionKind
	// SpecInput は欄の並びの指定を取るかである。
	SpecInput FormatSpecInput
	// FormatSpec は入力形式そのものが定める欄の並びである。
	// SpecInput が required の形式では空であり、並びは取り込みの指定が持つ。
	// 欄の並びを持たない入力形式でも空である。
	FormatSpec string
}

// Validate は宣言の必須条件を検査する。
func (f InputFormat) Validate() error {
	problem := firstProblem(
		requirePresent("InputFormat.key", string(f.Key)),
		requirePresent("InputFormat.parserId", f.ParserID),
		requireKnownEnum("InputFormat.positionKind", f.PositionKind),
		requireKnownEnum("InputFormat.specInput", f.SpecInput),
	)
	if problem != nil {
		return problem
	}
	// 並びを取り込みの指定から受け取る形式は、自身では並びを定めない。
	if f.SpecInput == FormatSpecInputRequired && f.FormatSpec != "" {
		return itemError("InputFormat.formatSpec", ErrInconsistentValue)
	}
	return nil
}

// FormatSpecFor は取り込みの指定を宣言に突き合わせ、収集元を読む欄の並びを返す。
//
// **収集元を開く前に呼ぶ。** 指定が宣言と食い違う起動で収集元の byte を 1 つも読まない。
func (f InputFormat) FormatSpecFor(requested *string) (string, error) {
	switch f.SpecInput {
	case FormatSpecInputRejected:
		if requested != nil {
			return "", itemError("InputFormat.formatSpec", ErrUnexpectedItem)
		}
		return f.FormatSpec, nil
	case FormatSpecInputRequired:
		if requested == nil || *requested == "" {
			return "", itemError("InputFormat.formatSpec", ErrMissingRequiredItem)
		}
		return *requested, nil
	default:
		return "", unknownEnumError("InputFormat.specInput", string(f.SpecInput))
	}
}
