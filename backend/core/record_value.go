package core

// ComparableValue はレコードの 1 項目の値から、文字列の一致を比べる値を返す。
//
// 正規化値を持つ値は正規化値を返し、原資料の文字列だけを持つ値は原資料の文字列を返す。導出値は正規化値
// だけを持つため、同じ経路で比べられる。
//
// ok が偽になるのは、値がある状態と導出できた状態のどちらでもない値である。
// absent と item_absent と no_body と out_of_definition と derivation_undetermined を
// 文字列の一致の判定に使わない。値の不在を表す文字列どうしが一致して、値が一致した候補と
// 同じ扱いになるためである。
func (r RawAndNormalized) ComparableValue() (string, bool) {
	switch r.ValueState {
	case ValueStatePresent, ValueStateDerived:
	default:
		return "", false
	}
	if normalized, ok := r.NormalizedValue(); ok {
		return normalized, true
	}
	return r.RawTextValue()
}
