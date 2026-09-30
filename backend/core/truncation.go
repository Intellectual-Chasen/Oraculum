package core

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// BothSideCounts は両側の件数と、その件数が対象とする範囲を持つ。
// 2 つの件数から比を作る項目を置かない。
type BothSideCounts struct {
	// LeftCount は起点の側の母数の件数である。
	LeftCount int64 `json:"leftCount"`
	// LeftScope は LeftCount が数えた範囲である。
	LeftScope string `json:"leftScope"`
	// RightCount は候補の側の母数の件数である。
	RightCount int64 `json:"rightCount"`
	// RightScope は RightCount が数えた範囲である。
	RightScope string `json:"rightScope"`
}

// Validate は項目の整合を確かめる。
func (b BothSideCounts) Validate() error {
	return firstProblem(
		requireNonNegative("BothSideCounts.leftCount", b.LeftCount),
		requirePresent("BothSideCounts.leftScope", b.LeftScope),
		requireNonNegative("BothSideCounts.rightCount", b.RightCount),
		requirePresent("BothSideCounts.rightScope", b.RightScope),
	)
}

// UnmarshalJSON は全項目を復元する。leftCount と rightCount を欠いた object は
// error を返す。2 項目は必須であり、欠けた件数を 0 へ復元すると、数えていない母数が
// 0 件の母数として読める形になる。
func (b *BothSideCounts) UnmarshalJSON(data []byte) error {
	type items BothSideCounts
	var decoded struct {
		items
		LeftCount  *int64 `json:"leftCount"`
		RightCount *int64 `json:"rightCount"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return fmt.Errorf("decoding BothSideCounts: %w", err)
	}
	problem := firstProblem(
		requireDecodedNumber("BothSideCounts.leftCount", decoded.LeftCount),
		requireDecodedNumber("BothSideCounts.rightCount", decoded.RightCount),
	)
	if problem != nil {
		return fmt.Errorf("decoding BothSideCounts: %w", problem)
	}
	value := BothSideCounts(decoded.items)
	value.LeftCount = *decoded.LeftCount
	value.RightCount = *decoded.RightCount
	*b = value
	return nil
}

// validateCountedSet は総数が、応答に入れた要素数と一致することを確かめる。
//
// **集合は全件を持つ。** 総数が要素数と食い違う応答は、数えた要素の一部を除いたまま
// 総数だけを申告しており、分析者が「一覧に出ていない要素は無い」と読めない。
func validateCountedSet(item string, totalCount int64, returnedCount int) error {
	if problem := requireNonNegative(item+"Count", totalCount); problem != nil {
		return problem
	}
	if totalCount != int64(returnedCount) {
		return itemError(item+"Count differs from the number of returned elements",
			ErrInconsistentValue)
	}
	return nil
}
