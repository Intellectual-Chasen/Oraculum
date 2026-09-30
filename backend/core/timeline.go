package core

import "time"

// TimelineEntry は時系列の 1 行である。
//
// **原資料の値を持たない。** 値は recordRef を渡した `/api/v0/records` が返す。
type TimelineEntry struct {
	// GraphEvidence は行が指すレコードの位置と事象の時刻と観測の種別である。
	GraphEvidence
	// Terminal はレコードを出した端末のノードである。レコードが端末の識別鍵に入る値を
	// 持たないときは出ない。
	//
	// **分析者が与えた端末の割当から導いた端末を持たない。** 割当は IP とその保持期間から
	// の推論であり、レコードが記録した端末と意味が違う。
	Terminal *GraphNode `json:"terminal,omitempty"`
	// Account はレコードが記録したアカウントのノードである。レコードがアカウントの
	// 識別鍵に入る値を持たないときは出ない。
	Account *GraphNode `json:"account,omitempty"`
	// TimeFieldName は、行の時刻を記録した欄の名前である。1 つのレコードが実行の履歴のように
	// 複数の事象の時刻を持つとき、時系列はレコード 1 件を時刻ごとの行で並べ、eventTime はその
	// 行の時刻を持ち、すべての行が欄の名前を持つ。事象の時刻を 1 つだけ持つレコードの行では出ない。
	TimeFieldName string `json:"timeFieldName,omitempty"`
	// AccountRoles は指定したアカウントがこのレコードで担う役割である。アカウントを起点にした要求だけが持つ。
	AccountRoles []EdgeKind `json:"accountRoles,omitempty"`
	// OtherAccounts は同じレコードが役割付きで指す別のアカウントである。
	OtherAccounts []TimelineAccount `json:"otherAccounts,omitempty"`
	// SourceAddress は記録に現れた接続元アドレスの原文である。
	SourceAddress string `json:"sourceAddress,omitempty"`
}

// TimelineAccount は同じレコードに現れたアカウントと役割である。
type TimelineAccount struct {
	Role EdgeKind  `json:"role"`
	Node GraphNode `json:"node"`
}

// Validate は項目の整合を確かめる。
func (e TimelineEntry) Validate() error {
	if err := e.GraphEvidence.Validate(); err != nil {
		return err
	}
	if e.TimeFieldName != "" && e.EventTime == nil {
		return itemError("TimelineEntry.timeFieldName", ErrMissingRequiredItem)
	}
	for _, role := range e.AccountRoles {
		if !timelineAccountRole(role) {
			return itemError("TimelineEntry.accountRoles", ErrUnknownEnumValue)
		}
	}
	for _, account := range e.OtherAccounts {
		if !timelineAccountRole(account.Role) || account.Node.Kind != NodeKindAccount {
			return itemError("TimelineEntry.otherAccounts", ErrUnknownEnumValue)
		}
		if err := account.Node.Validate(); err != nil {
			return itemError("TimelineEntry.otherAccounts", err)
		}
	}
	for _, item := range []struct {
		name string
		node *GraphNode
	}{
		{"TimelineEntry.terminal", e.Terminal},
		{"TimelineEntry.account", e.Account},
	} {
		if item.node == nil {
			continue
		}
		if err := item.node.Validate(); err != nil {
			return itemError(item.name, err)
		}
	}
	return nil
}

func timelineAccountRole(role EdgeKind) bool {
	return role == EdgeKindRecordSubjectAccount || role == EdgeKindRecordTargetAccount || role == EdgeKindRecordNamesObject
}

// CoverageState は、要求が与えた期間と収集元の収録範囲の重なり方である。
type CoverageState string

const (
	// CoverageStateCovered は、要求の期間が収録範囲に収まる状態である。
	CoverageStateCovered CoverageState = "covered"
	// CoverageStatePartiallyCovered は、要求の期間が収録範囲の外へはみ出す状態である。
	CoverageStatePartiallyCovered CoverageState = "partially_covered"
	// CoverageStateOutsideRecording は、要求の期間が収録範囲と重ならない状態である。
	CoverageStateOutsideRecording CoverageState = "outside_recording"
	// CoverageStateRecordingRangeUnknown は、収集元が絶対時刻として読める根拠を
	// 1 件も持たない状態である。
	CoverageStateRecordingRangeUnknown CoverageState = "recording_range_unknown"
	// CoverageStateRangeNotRequested は、要求が期間を指定していない状態である。
	CoverageStateRangeNotRequested CoverageState = "range_not_requested"
	// CoverageStateUndetermined は、収録範囲の両端または要求の時刻を絶対時刻として
	// 比べられない状態である。
	CoverageStateUndetermined CoverageState = "undetermined"
)

// KnownCoverageStates は契約が定める収録範囲の状態を、本関数が並べた順で返す。
func KnownCoverageStates() []CoverageState {
	return []CoverageState{
		CoverageStateCovered,
		CoverageStatePartiallyCovered,
		CoverageStateOutsideRecording,
		CoverageStateRecordingRangeUnknown,
		CoverageStateRangeNotRequested,
		CoverageStateUndetermined,
	}
}

// IsKnown は契約が定める値かを返す。
func (s CoverageState) IsKnown() bool {
	for _, known := range KnownCoverageStates() {
		if s == known {
			return true
		}
	}
	return false
}

// Validate は契約の外の値を退ける。
func (s CoverageState) Validate() error {
	if !s.IsKnown() {
		return ErrUnknownEnumValue
	}
	return nil
}

// SourceCoverage は収集元 1 件の収録範囲と、要求の期間との重なり方である。
type SourceCoverage struct {
	// SourceId は収集元の識別子である。
	SourceId string `json:"sourceId"`
	// SourceFileName は分析者が読む収集元の file 名である。
	//
	// **識別子だけを返さない。** SourceId は内容の digest から作る不透明な文字列であり、
	// 分析者が収録範囲の行をどの収集元のものと読むかを決められない。
	SourceFileName string `json:"sourceFileName"`
	// ObservedRangeFirst と ObservedRangeLast は収録範囲の両端である。
	// 絶対時刻として読める根拠を 1 件も持たない収集元では、2 つとも出ない。
	//
	// **母集団は解析に失敗したレコードを含む。** 収録範囲は「この収集元がいつからいつまで
	// を記録したか」であり、こちらが読めたかとは別の事実である。
	ObservedRangeFirst *Timestamp `json:"observedRangeFirst,omitempty"`
	ObservedRangeLast  *Timestamp `json:"observedRangeLast,omitempty"`
	// State は要求の期間と収録範囲の重なり方である。
	State CoverageState `json:"state"`
	// MatchedRecordCount は、この収集元のレコードのうち絞り込みを通った件数である。
	//
	// **母集団は取り込みに成功し公開されているレコードだけである。** ObservedRangeFirst
	// と ObservedRangeLast の母集団と違う。
	//
	// 0 件を「記録が無い」と読み替えない。「記録が無い」は State が
	// outside_recording または recording_range_unknown であることが示す。State が
	// covered または partially_covered で本件数が 0 のときは「記録はあり、事象が無い」
	// である。
	MatchedRecordCount int64 `json:"matchedRecordCount"`
	// MatchedRowCount は、この収集元の時系列の行のうち絞り込みを通った行の数である。
	// ほかの事象の時刻を持つレコードは時刻ごとに行を持つため、MatchedRecordCount 以上になる。
	MatchedRowCount int64 `json:"matchedRowCount"`
}

// Validate は項目の整合を確かめる。
func (c SourceCoverage) Validate() error {
	if c.SourceId == "" {
		return itemError("SourceCoverage.sourceId", ErrMissingRequiredItem)
	}
	if c.SourceFileName == "" {
		return itemError("SourceCoverage.sourceFileName", ErrMissingRequiredItem)
	}
	if err := c.State.Validate(); err != nil {
		return itemError("SourceCoverage.state", err)
	}
	if err := requireNonNegative(
		"SourceCoverage.matchedRecordCount", c.MatchedRecordCount); err != nil {
		return err
	}
	if err := requireNonNegative(
		"SourceCoverage.matchedRowCount", c.MatchedRowCount); err != nil {
		return err
	}
	if (c.ObservedRangeFirst == nil) != (c.ObservedRangeLast == nil) {
		return itemError(
			"SourceCoverage.observedRangeFirst and SourceCoverage.observedRangeLast",
			ErrInconsistentValue)
	}
	for _, item := range []struct {
		name string
		at   *Timestamp
	}{
		{"SourceCoverage.observedRangeFirst", c.ObservedRangeFirst},
		{"SourceCoverage.observedRangeLast", c.ObservedRangeLast},
	} {
		if item.at == nil {
			continue
		}
		if err := item.at.Validate(); err != nil {
			return itemError(item.name, err)
		}
	}
	if c.ObservedRangeFirst == nil && c.State != CoverageStateRecordingRangeUnknown &&
		c.State != CoverageStateRangeNotRequested {
		return itemError("SourceCoverage.state", ErrInconsistentValue)
	}
	return nil
}

// EvaluateCoverage は、収録範囲と要求の期間の重なり方を返す。
//
// **読むのは収録範囲の両端と要求の両端だけである。** 合致したレコードの件数を受けない。
// 種別の絞り込みで 0 件になった収集元に「記録が無い」を付けると、記録の不在と事象の
// 不在が同じ表現になる。
//
// 比較の単位は呼び出し元が与える。根拠のレコードを絞る判定 (pipeline の
// RecordFilter) と同じ単位で切り捨ててから比べる。単位が割れていると、行として期間内に
// 入るレコードを持つ収集元に outside_recording が付く。
//
// recordedKnown が偽になるのは、収集元が絶対時刻として読める根拠を 1 件も持たないとき
// である。
func EvaluateCoverage(
	recorded TimeRange, recordedKnown bool, from, to *time.Time, unit time.Duration,
) CoverageState {
	if from == nil && to == nil {
		return CoverageStateRangeNotRequested
	}
	if !recordedKnown {
		return CoverageStateRecordingRangeUnknown
	}
	if unit <= 0 {
		unit = time.Second
	}
	first, firstOk := recorded.From.Instant()
	last, lastOk := recorded.To.Instant()
	if !firstOk || !lastOk {
		return CoverageStateUndetermined
	}
	first, last = first.Truncate(unit), last.Truncate(unit)
	// 与えられていない端は収録範囲の端まで開いているものとして比べる。
	wantFrom, wantTo := first, last
	if from != nil {
		wantFrom = from.Truncate(unit)
	}
	if to != nil {
		wantTo = to.Truncate(unit)
	}
	if wantTo.Before(first) || wantFrom.After(last) {
		return CoverageStateOutsideRecording
	}
	if wantFrom.Before(first) || wantTo.After(last) {
		return CoverageStatePartiallyCovered
	}
	return CoverageStateCovered
}
