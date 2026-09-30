package core

import (
	"encoding/json"
	"fmt"
)

// ObservationKindStatus は観測の種別の意味の状態を持つ。
type ObservationKindStatus string

// ObservationKindStatus の値。**根拠の強さが違う状態を 1 つの値へまとめない。**
// 入力形式が意味を定める種別と、収集元のレコードの出方から意味を推定した種別を、
// 分析者が値だけで読み分けられるようにする。
const (
	// ObservationKindStatusDetermined は、入力形式が意味を定めている状態である。
	ObservationKindStatusDetermined ObservationKindStatus = "determined"
	// ObservationKindStatusInferred は、入力形式が意味を定めず、収集元のレコードの
	// 出方から意味を推定した状態である。推定の根拠は adapter が持つ。
	ObservationKindStatusInferred ObservationKindStatus = "inferred"
	// ObservationKindStatusUndetermined は観測の種別の意味が確定しない状態である。
	ObservationKindStatusUndetermined ObservationKindStatus = "undetermined"
)

// IsKnown は ObservationKindStatus が定義の中の値であるかを返す。
func (o ObservationKindStatus) IsKnown() bool {
	switch o {
	case ObservationKindStatusDetermined, ObservationKindStatusInferred,
		ObservationKindStatusUndetermined:
		return true
	default:
		return false
	}
}

// ObservationKind は 1 レコードの観測の種別と、その意味の状態を持つ。
//
// 2 つ以上の key の値を 1 つの文字列につないだ要素を作らない。
type ObservationKind struct {
	// Raw は観測の種別を持つレコードの項目の集合である。要素の名前と個数は入力形式が決め、
	// 欄を持たない形式では要素数 0 の集合になる。
	Raw []RecordField `json:"raw"`
	// Status は観測の種別の意味の状態である。Raw の要素数が 1 以上で、各要素の Text の
	// ValueState がすべて present のとき必須で、他のときは出ない。
	Status ObservationKindStatus `json:"status,omitempty"`
	// Meaning は推定した意味である。Status が inferred のとき必須で、他の状態では出ない。
	Meaning string `json:"meaning,omitempty"`
}

// Validate は項目の整合と、Status と Meaning の出現条件を確かめる。
func (k ObservationKind) Validate() error {
	// validateRaw が返す error は既に項目名を持つため、包み直さない。
	if problem := k.validateRaw(); problem != nil {
		return problem
	}
	if problem := k.validateMeaning(); problem != nil {
		return problem
	}
	if k.statusRequired() {
		return requireKnownEnum("ObservationKind.status", k.Status)
	}
	// 欄が無い状態と、欄があって意味が確定しない状態を同じ値で表さない。
	if k.Status != "" {
		return itemError("ObservationKind.status is present while ObservationKind.raw "+
			"does not carry a value for every element", ErrInconsistentValue)
	}
	return nil
}

// validateMeaning は推定した意味の出現条件を確かめる。
//
// **形式が意味を定める種別に意味の文字列を持たせない。** 持たせると、応答の文字列と
// 形式の定めという 2 つの定義元ができる。inferred の種別は、分析者が推定の当否を
// 確かめられるよう意味を必ず持つ。
func (k ObservationKind) validateMeaning() error {
	if k.Status == ObservationKindStatusInferred {
		return requirePresent("ObservationKind.meaning", k.Meaning)
	}
	if k.Meaning != "" {
		return itemError("ObservationKind.meaning is present while ObservationKind.status "+
			"is not inferred", ErrInconsistentValue)
	}
	return nil
}

// validateRaw は観測の種別を持つ項目の名前が重ならないことを確かめる。
func (k ObservationKind) validateRaw() error {
	seenNames := make(map[string]struct{}, len(k.Raw))
	for index, field := range k.Raw {
		if err := field.Validate(); err != nil {
			return itemError("ObservationKind.raw at "+formatIndex(index), err)
		}
		if field.Kind != RecordFieldKindText {
			return itemError("ObservationKind.raw at "+formatIndex(index)+
				" is not a text field", ErrInconsistentValue)
		}
		if _, duplicate := seenNames[field.Name]; duplicate {
			return itemError("ObservationKind.raw has a repeated name "+field.Name,
				ErrDuplicateElement)
		}
		seenNames[field.Name] = struct{}{}
	}
	return nil
}

// statusRequired は Status が必須になる条件が成り立つかを返す。
func (k ObservationKind) statusRequired() bool {
	if len(k.Raw) == 0 {
		return false
	}
	for _, field := range k.Raw {
		if field.Text == nil || field.Text.ValueState != ValueStatePresent {
			return false
		}
	}
	return true
}

// rawValue は名前で指した項目の原資料の文字列を返す。
func (k ObservationKind) rawValue(name string) (string, bool) {
	for _, field := range k.Raw {
		if field.Name != name || field.Text == nil {
			continue
		}
		return field.Text.RawTextValue()
	}
	return "", false
}

// Matches は 1 レコードの観測の種別が、候補として数えるレコードを選ぶ条件に一致するかを返す。
//
// 比べるのは、選ぶ条件が挙げた欄すべての原資料の value の文字列である。**要素数の一致も
// 条件にする。** 選ぶ条件が挙げた欄がレコードの欄の一部でしかないとき、その一致は
// 「同じ種別である」という主張にならない。
func (k ObservationKind) Matches(selector ObservationKindSelector) bool {
	if len(k.Raw) != len(selector.Items) {
		return false
	}
	for _, item := range selector.Items {
		value, found := k.rawValue(item.Name)
		if !found || value != item.Value {
			return false
		}
	}
	return true
}

// MarshalJSON は Raw を要素数 0 の場合も集合として出す。
func (k ObservationKind) MarshalJSON() ([]byte, error) {
	// items は ObservationKind の method を持たないため、この Marshal は再帰しない。
	type items ObservationKind
	copied := items(k)
	copied.Raw = emptyIfNil(copied.Raw)
	encoded, err := json.Marshal(copied)
	if err != nil {
		return nil, fmt.Errorf("marshaling ObservationKind: %w", err)
	}
	return encoded, nil
}

// ObservationKindSelectorItem は観測の種別を持つ欄 1 つと、選ぶ value の文字列である。
type ObservationKindSelectorItem struct {
	// Name は観測の種別を持つレコードの項目の名前である。原資料の key の文字列である。
	Name string `json:"name"`
	// Value は候補として数えるレコードを選ぶ value の文字列である。
	Value string `json:"value"`
}

// Validate は全項目に値があることを確かめる。
func (i ObservationKindSelectorItem) Validate() error {
	return firstProblem(
		requirePresent("ObservationKindSelectorItem.name", i.Name),
		requirePresent("ObservationKindSelectorItem.value", i.Value),
	)
}

// ContentReplacementSelector は、ファイルまたはレジストリの値の内容を置き換える記録を選ぶ条件で
// ある。観測の種別が Kind に該当するレコードのうち、Fields が挙げた欄すべての原資料の文字列が
// value と一致するレコードを選ぶ。Fields が空のときは観測の種別だけで選ぶ。
//
// **欄の名前と value の文字列の定義元は入力形式を読む adapter である。**
type ContentReplacementSelector struct {
	Kind   ObservationKindSelector
	Fields []ObservationKindSelectorItem
}

// ObservationKindSelector は候補として数えるレコードを選ぶ観測の種別 1 件を持つ。
type ObservationKindSelector struct {
	// Items は選ぶ観測の種別を組む欄と value の組である。要素数は 1 以上である。
	Items []ObservationKindSelectorItem `json:"items"`
}

// Validate は要素が 1 件以上あり、各要素に値があり、欄の名前が重ならないことを確かめる。
func (s ObservationKindSelector) Validate() error {
	if len(s.Items) == 0 {
		return itemError("ObservationKindSelector.items", ErrMissingRequiredItem)
	}
	seenNames := make(map[string]struct{}, len(s.Items))
	for index, item := range s.Items {
		if err := item.Validate(); err != nil {
			return itemError("ObservationKindSelector.items at "+formatIndex(index), err)
		}
		if _, duplicate := seenNames[item.Name]; duplicate {
			return itemError("ObservationKindSelector.items has a repeated name "+item.Name,
				ErrDuplicateElement)
		}
		seenNames[item.Name] = struct{}{}
	}
	return nil
}
