package core

// MatchedKind は、絞り込みに合うノードの種別 1 つ分の件数である。
type MatchedKind struct {
	Kind  NodeKind `json:"kind"`
	Count int64    `json:"count"`
}

// Validate は種別が定義の中の値であり、件数が 1 以上であることを確かめる。
func (k MatchedKind) Validate() error {
	if problem := requireKnownEnum("MatchedKind.kind", k.Kind); problem != nil {
		return problem
	}
	if k.Count < 1 {
		// 件数 0 の種別を持つと、「その種別が合った」と読める要素が増える。
		return itemError("MatchedKind.count", ErrInvalid)
	}
	return nil
}

// EdgeKindCount は、関係の種別 1 つ分のエッジの本数である。
type EdgeKindCount struct {
	Kind  EdgeKind `json:"kind"`
	Count int64    `json:"count"`
}

// Validate は種別が定義の中の値であり、本数が 1 以上であることを確かめる。
func (c EdgeKindCount) Validate() error {
	if problem := requireKnownEnum("EdgeKindCount.kind", c.Kind); problem != nil {
		return problem
	}
	if c.Count < 1 {
		return itemError("EdgeKindCount.count", ErrInvalid)
	}
	return nil
}
