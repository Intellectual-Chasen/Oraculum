package core

// SearchExpressionErrorReason は、検索式を退けた理由の種別である。
type SearchExpressionErrorReason string

// SearchExpressionErrorReason の値。
const (
	// SearchExpressionErrorReasonEmptyExpression は、式が空白だけである理由である。
	SearchExpressionErrorReasonEmptyExpression SearchExpressionErrorReason = "empty_expression"
	// SearchExpressionErrorReasonTooLong は、式の byte 数が上限を超える理由である。
	SearchExpressionErrorReasonTooLong SearchExpressionErrorReason = "too_long"
	// SearchExpressionErrorReasonTooManyTerms は、式の条件の数が上限を超える理由である。
	SearchExpressionErrorReasonTooManyTerms SearchExpressionErrorReason = "too_many_terms"
	// SearchExpressionErrorReasonNestingTooDeep は、括弧と否定の入れ子の深さが上限を超える理由である。
	SearchExpressionErrorReasonNestingTooDeep SearchExpressionErrorReason = "nesting_too_deep"
	// SearchExpressionErrorReasonUnexpectedCharacter は、どの文字列も始められない文字がある理由である。
	SearchExpressionErrorReasonUnexpectedCharacter SearchExpressionErrorReason = "unexpected_character"
	// SearchExpressionErrorReasonUnterminatedString は、引用符で始めた文字列が閉じていない理由である。
	SearchExpressionErrorReasonUnterminatedString SearchExpressionErrorReason = "unterminated_string"
	// SearchExpressionErrorReasonInvalidEscape は、引用符で囲んだ文字列の中の `\` の後が `"` と `\` の
	// どちらでもない理由である。
	SearchExpressionErrorReasonInvalidEscape SearchExpressionErrorReason = "invalid_escape"
	// SearchExpressionErrorReasonMissingField は、比較の演算子の前に欄が無い理由である。
	SearchExpressionErrorReasonMissingField SearchExpressionErrorReason = "missing_field"
	// SearchExpressionErrorReasonMissingValue は、比較の演算子の後に値が無い理由である。
	SearchExpressionErrorReasonMissingValue SearchExpressionErrorReason = "missing_value"
	// SearchExpressionErrorReasonMissingOperand は、論理の演算子の前後か括弧の中に条件が無い理由である。
	SearchExpressionErrorReasonMissingOperand SearchExpressionErrorReason = "missing_operand"
	// SearchExpressionErrorReasonUnclosedParenthesis は、開いた括弧が閉じていない理由である。
	SearchExpressionErrorReasonUnclosedParenthesis SearchExpressionErrorReason = "unclosed_parenthesis"
	// SearchExpressionErrorReasonUnmatchedParenthesis は、閉じる括弧に対応する開いた括弧が無い
	// 理由である。
	SearchExpressionErrorReasonUnmatchedParenthesis SearchExpressionErrorReason = "unmatched_parenthesis"
	// SearchExpressionErrorReasonValueNotOrdered は、大小の比較の値が 10 進の数でも、UTC からのずれを
	// 持つ RFC 3339 の時刻でもない理由である。
	SearchExpressionErrorReasonValueNotOrdered SearchExpressionErrorReason = "value_not_ordered"
)

// SearchExpressionErrorReasons は SearchExpressionErrorReason の値の一覧を返す。
func SearchExpressionErrorReasons() []SearchExpressionErrorReason {
	return []SearchExpressionErrorReason{
		SearchExpressionErrorReasonEmptyExpression, SearchExpressionErrorReasonTooLong,
		SearchExpressionErrorReasonTooManyTerms, SearchExpressionErrorReasonNestingTooDeep,
		SearchExpressionErrorReasonUnexpectedCharacter, SearchExpressionErrorReasonUnterminatedString,
		SearchExpressionErrorReasonInvalidEscape, SearchExpressionErrorReasonMissingField,
		SearchExpressionErrorReasonMissingValue, SearchExpressionErrorReasonMissingOperand,
		SearchExpressionErrorReasonUnclosedParenthesis, SearchExpressionErrorReasonUnmatchedParenthesis,
		SearchExpressionErrorReasonValueNotOrdered,
	}
}

// IsKnown は SearchExpressionErrorReason が定義の中の値であるかを返す。
func (r SearchExpressionErrorReason) IsKnown() bool {
	for _, known := range SearchExpressionErrorReasons() {
		if r == known {
			return true
		}
	}
	return false
}

// SearchExpressionError は、検索式を退けた理由と、式の中の誤りの範囲である。
//
// **位置は式の先頭から数えた Unicode の符号位置の個数で表す。** 画面と server で文字列の
// 添字の単位が異なるため、byte の位置を持たない。
type SearchExpressionError struct {
	// Reason は式を退けた理由である。
	Reason SearchExpressionErrorReason `json:"reason"`
	// Offset は誤りの範囲の始まりである。
	Offset int `json:"offset"`
	// Length は誤りの範囲の符号位置の個数である。式の終わりで条件や値が足りない誤りは 0 である。
	Length int `json:"length"`
}

// Validate は項目の整合を確かめる。
func (e SearchExpressionError) Validate() error {
	if problem := requireKnownEnum("SearchExpressionError.reason", e.Reason); problem != nil {
		return problem
	}
	if e.Offset < 0 {
		return itemError("SearchExpressionError.offset", ErrNegativeCount)
	}
	if e.Length < 0 {
		return itemError("SearchExpressionError.length", ErrNegativeCount)
	}
	return nil
}
