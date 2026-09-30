package core

import (
	"encoding/json"
	"fmt"
	"slices"
)

// GraphNode はグラフのノード 1 つを持つ。
//
// **Id は不透明な文字列である。** 値を組み立てるのは backend であり、画面は受け取った値を
// そのまま要求へ返す。
type GraphNode struct {
	// Id はノードを指す不透明な識別子である。
	//
	// **値を導く材料は識別鍵の組だけである。** 収集元の識別子も解析実行への参照も材料に
	// 入らない。**同じ識別鍵を持つノードは、取り込み直しても解析実行が変わっても同じ値に
	// なる。** 値を作るのは取り込みの実行である。
	Id string `json:"id"`
	// Kind はノードの種別である。
	Kind NodeKind `json:"kind"`
	// KeyForm は識別鍵の形である。
	KeyForm NodeKeyForm `json:"keyForm"`
	// Identity は識別鍵の値である。要素数は 1 以上である。
	Identity []NodeIdentityValue `json:"identity"`
	// Label は分析者が読む表示名である。**表示名を持たないノードは item_absent を持つ。**
	// 空文字列で表示名の不在を表さない。
	Label RawAndNormalized `json:"label"`
	// Observation はノードを記録したレコードの種類である。参照だけのノードは referenced を
	// 持つ。
	Observation NodeObservation `json:"observation"`
	// CreationRecord は対象の生成を記録したレコードが根拠にあるかである。
	// **Observation と別の軸である。**
	CreationRecord NodeCreationRecord `json:"creationRecord"`
	// OnUnknownTerminal は、ファイルのノードが、端末を名乗らない収集元を記録した名前の分からない
	// 端末 (NodeKeyFormRecordingSource と NodeKeyFormRecordingSourceHostname の端末) に置かれたこと
	// である。その収集元に端末を割り当てると、同じ端末の他の収集元の同じ path のファイルと 1 つの
	// ノードになる。ファイル以外の種別のノードでは出ない。
	OnUnknownTerminal bool `json:"onUnknownTerminal,omitempty"`
	// IntervalStart は、PID の区間のノード (NodeKeyFormTerminalProcessInterval) の区間を開いた
	// レコードの時刻である。識別の値の最後の値は、この時刻の正規化値である。**時刻は収集元の
	// 時刻の解釈を持つ。** 地方時の文字列の識別の値を、どのずれで読んだかを示す。ほかの形の
	// ノードでは出ない。
	IntervalStart *Timestamp `json:"intervalStart,omitempty"`
	// AccountName は、同じアカウントとしてまとめる鍵である。アカウントのノードのうち、鍵を
	// 決められたノードだけが持つ。名前のノードは自身の識別鍵から、SID のノードは SID と共に
	// 記録した名前が 1 組だけのときに、その名前から作る。
	AccountName *AccountNameKey `json:"accountName,omitempty"`
	// AccountNameWithheld は、アカウントのノードが AccountName を持たない理由である。
	// AccountName と同時に出ない。
	AccountNameWithheld AccountNameWithheldReason `json:"accountNameWithheld,omitempty"`
}

// Validate は項目の整合と、2 つの軸と種別の対応を確かめる。
func (n GraphNode) Validate() error {
	problem := firstProblem(
		requirePresent("GraphNode.id", n.Id),
		requireKnownEnum("GraphNode.kind", n.Kind),
		requireKnownEnum("GraphNode.keyForm", n.KeyForm),
		requireKnownEnum("GraphNode.observation", n.Observation),
		requireKnownEnum("GraphNode.creationRecord", n.CreationRecord),
	)
	if problem != nil {
		return problem
	}
	// 生成を記録した根拠を持てない種別に present と absent を置かない。置くと、その種別の
	// ノードが生成のレコードを持ち得るという主張になる。
	if n.Kind.CarriesCreationRecord() == (n.CreationRecord == NodeCreationRecordItemAbsent) {
		return itemError("GraphNode.creationRecord "+string(n.CreationRecord)+" on the kind "+
			string(n.Kind), ErrInconsistentValue)
	}
	if len(n.Identity) == 0 {
		return itemError("GraphNode.identity", ErrMissingRequiredItem)
	}
	if problem := validateElements("GraphNode.identity", n.Identity); problem != nil {
		return problem
	}
	if err := n.Label.Validate(); err != nil {
		return itemError("GraphNode.label", err)
	}
	if n.IntervalStart != nil {
		if n.KeyForm != NodeKeyFormTerminalProcessInterval {
			return itemError("GraphNode.intervalStart on the key form "+string(n.KeyForm), ErrUnexpectedItem)
		}
		if err := n.IntervalStart.Validate(); err != nil {
			return itemError("GraphNode.intervalStart", err)
		}
	}
	return n.validateAccountName()
}

// validateAccountName は、まとめる鍵と、鍵を持たない理由がアカウントのノードだけにあり、
// 2 つが同時に出ないことを確かめる。
func (n GraphNode) validateAccountName() error {
	if n.AccountName == nil && n.AccountNameWithheld == "" {
		return nil
	}
	if n.Kind != NodeKindAccount {
		return itemError("GraphNode.accountName on the kind "+string(n.Kind), ErrUnexpectedItem)
	}
	if n.AccountName != nil && n.AccountNameWithheld != "" {
		return itemError("GraphNode.accountNameWithheld with GraphNode.accountName", ErrInconsistentValue)
	}
	if n.AccountName != nil {
		if err := requirePresent("GraphNode.accountName.name", n.AccountName.Name); err != nil {
			return err
		}
		if n.AccountName.NodeCount < 1 {
			return itemError("GraphNode.accountName.nodeCount", ErrInconsistentValue)
		}
		return nil
	}
	return requireKnownEnum("GraphNode.accountNameWithheld", n.AccountNameWithheld)
}

// AccountNameKey は、同じアカウントとしてまとめるノードが共に持つ鍵である。
//
// **案件をまたいでまとめない。** 鍵は案件と、ドメインとログイン名の組である。
type AccountNameKey struct {
	// CaseId は鍵の案件である。案件を区別しない取り込みでは出ない。
	CaseId string `json:"caseId,omitempty"`
	// Name は小文字にした `ドメイン\ログイン名` である。ドメインは最初のラベル (最初の `.` の前)
	// であり、ドメインの短い名前と FQDN は同じ値になる。ホスト名を変えた端末のローカル
	// アカウントでは、端末の今と過去の名前のうち最小の文字列をドメインに使う。UPN の形の名前は
	// `@` の前をログイン名に使う。
	Name string `json:"name"`
	// NodeCount は、グラフ全体で同じ鍵を持つノードの数である。1 以上である。
	NodeCount int64 `json:"nodeCount"`
}

// AccountNameWithheldReason は、アカウントのノードが AccountNameKey を持たない理由である。
type AccountNameWithheldReason string

// AccountNameWithheldReason の値。
const (
	// AccountNameWithheldNoNameRecorded は、SID と共にドメインとログイン名を記録したレコードが無い
	// SID のノードである。
	AccountNameWithheldNoNameRecorded AccountNameWithheldReason = "no_name_recorded"
	// AccountNameWithheldMultipleNames は、SID と共に記録したドメインとログイン名が 2 組以上ある
	// SID のノードである。途中で名前を変えたアカウントに当たる。
	AccountNameWithheldMultipleNames AccountNameWithheldReason = "multiple_names"
	// AccountNameWithheldMultipleCases は、根拠のレコードが 2 つ以上の案件にあるノードである。
	AccountNameWithheldMultipleCases AccountNameWithheldReason = "multiple_cases"
)

// IsKnown は AccountNameWithheldReason が定義の中の値であるかを返す。
func (r AccountNameWithheldReason) IsKnown() bool {
	switch r {
	case AccountNameWithheldNoNameRecorded, AccountNameWithheldMultipleNames, AccountNameWithheldMultipleCases:
		return true
	}
	return false
}

// NodeSelection は応答がノードを持つ理由である。
type NodeSelection string

// NodeSelection の値。
const (
	// NodeSelectionMatched は要求の絞り込みに合ったノードである。
	NodeSelectionMatched NodeSelection = "matched"
	// NodeSelectionEdgeEndpoint は応答が返したエッジの端点であるために持つノードである。
	// 絞り込みに合うノードの上限の外にあっても応答に入る。
	NodeSelectionEdgeEndpoint NodeSelection = "edge_endpoint"
)

// IsKnown は NodeSelection が定義の中の値であるかを返す。
func (s NodeSelection) IsKnown() bool {
	return s == NodeSelectionMatched || s == NodeSelectionEdgeEndpoint
}

// ValueMatchForm は検索の文字列が一致した値の形である。
type ValueMatchForm string

// ValueMatchForm の値。
const (
	// ValueMatchFormRawText は原資料の文字列で一致したことを表す。
	ValueMatchFormRawText ValueMatchForm = "raw_text"
	// ValueMatchFormNormalized は正規化値で一致したことを表す。
	ValueMatchFormNormalized ValueMatchForm = "normalized"
)

// IsKnown は ValueMatchForm が定義の中の値であるかを返す。
func (f ValueMatchForm) IsKnown() bool {
	return f == ValueMatchFormRawText || f == ValueMatchFormNormalized
}

// NodeValueMatch は値の検索が 1 つの欄に一致したことを表す。
//
// **どの欄のどちらの値で一致したかを分析者へ返す。** 同じ文字列が原資料の文字列にあるのか
// 正規化値にあるのかで、原文を読むときに一致する位置が変わる。
//
// 欄ごとに一致した値を 1 件だけ持つ。同じ欄のすべての値はノードの属性として
// `/api/v0/nodes/{id}` が返す。
type NodeValueMatch struct {
	// Semantic は一致した欄の語彙の項目である。語彙に写していない欄では出ない。
	Semantic SemanticKey `json:"semantic,omitempty"`
	// Name は一致した欄の原資料の key の文字列である。語彙に写していない欄でだけ出る。
	Name string `json:"name,omitempty"`
	// Form は原資料の文字列と正規化値のどちらで一致したかである。
	Form ValueMatchForm `json:"form"`
	// Value は、この欄とこの形で一致した値のうち、ノードの属性の並びで最初の値である。値は
	// Form の形の文字列である。同じ欄に一致した値が 2 件以上あっても 1 件だけを持つ。
	Value string `json:"value"`
}

// Validate は欄の名前と一致した値の形を確かめる。
//
// **語彙の項目と原資料の key のちょうど一方を求める。** 判定は NodeAttribute と同じである。
func (m NodeValueMatch) Validate() error {
	if (m.Semantic == "") == (m.Name == "") {
		return itemError("NodeValueMatch.semantic and NodeValueMatch.name", ErrInconsistentValue)
	}
	return firstProblem(
		requireKnownEnumWhenPresent("NodeValueMatch.semantic", m.Semantic),
		requireKnownEnum("NodeValueMatch.form", m.Form),
	)
}

// SubgraphNode は部分グラフが持つノード 1 つである。
type SubgraphNode struct {
	GraphNode
	// Selection は応答がこのノードを持つ理由である。
	Selection NodeSelection `json:"selection"`
	// ValueMatches は値の検索が一致した欄である。
	//
	// 検索の文字列を与えない要求では出ない。与えた要求では、Selection が matched の
	// ノードが 1 件以上を持つ。応答が返したエッジの端点として入ったノードは検索に
	// 一致したとは限らないため、持たないことがある。
	ValueMatches []NodeValueMatch `json:"valueMatches,omitempty"`
	// Terminals は、このノードの根拠のレコードが名乗った端末である。
	//
	// **母集団は Selection ごとに違う。** Selection が matched のノードでは、
	// 絞り込みを通った根拠が名乗った端末である。Selection が edge_endpoint の
	// ノードでは、そのノード自身の根拠を絞り込みで判定していないため、根拠の全件が
	// 名乗った端末である。
	//
	// 空になるのは、レコードが端末の識別鍵に入る値を持たない場合と、絞り込みの外にある
	// 場合である。
	//
	// 分析者が与えた端末の割当から導いた端末を持たない。割当は接続元 IP とその保持期間からの
	// 推論であり、レコードが名乗った端末と意味が違う。
	//
	// 端末のノード自身も、自分の根拠が名乗った端末を持つ。種別ごとに母集団を変えると、
	// 列の空欄が何を表すかが行ごとに変わる。
	Terminals []GraphNode `json:"terminals,omitempty"`
	// Record は、レコードのノードが指すレコード 1 件の位置と時刻と事象の種別である。
	// 要求がレコードの要約を求め (recordSummary)、Selection が matched のレコードのノードだけが持つ。
	Record *RecordSummary `json:"record,omitempty"`
}

// RecordSummary はレコード 1 件の位置と時刻と事象の種別である。
type RecordSummary struct {
	// RecordRef はレコードの位置である。
	RecordRef RecordLocator `json:"recordRef"`
	// EventTime はレコードが記録した事象の時刻である。原資料の文字列と正規化値を持つ。
	// 時刻を読めなかったレコードでは出ない。
	EventTime *Timestamp `json:"eventTime,omitempty"`
	// EventCategory と EventAction は事象の種別の組である。Windows イベントログのレコード
	// (WindowsEvent が真) ではプロバイダの名前とイベント ID である。持たないレコードでは出ない。
	EventCategory string `json:"eventCategory,omitempty"`
	EventAction   string `json:"eventAction,omitempty"`
	WindowsEvent  bool   `json:"windowsEvent,omitempty"`
	// Channel は Windows イベントログのチャネルの名前である。持たないレコードでは出ない。
	Channel string `json:"channel,omitempty"`
	// RecordHeaderId は EVTX のレコードの見出しが記録したレコードの番号の文字列である。
	// EventRecordId は XML の EventRecordID の文字列である。どちらも持たないレコードでは出ない。
	RecordHeaderId string `json:"recordHeaderId,omitempty"`
	EventRecordId  string `json:"eventRecordId,omitempty"`
}

// Validate は位置と時刻を確かめる。
func (s RecordSummary) Validate() error {
	if err := s.RecordRef.Validate(); err != nil {
		return itemError("RecordSummary.recordRef", err)
	}
	if s.EventTime != nil {
		if err := s.EventTime.Validate(); err != nil {
			return itemError("RecordSummary.eventTime", err)
		}
	}
	return nil
}

// Validate はノードと持つ理由と、検索が一致した欄と、名乗った端末を確かめる。
//
// GraphNode.Validate が返す error は既に項目名を持つため、包み直さない。
func (n SubgraphNode) Validate() error {
	problem := firstProblem(
		n.GraphNode.Validate(),
		requireKnownEnum("SubgraphNode.selection", n.Selection),
	)
	if problem != nil {
		return problem
	}
	if err := validateElements("SubgraphNode.valueMatches", n.ValueMatches); err != nil {
		return err
	}
	for _, terminal := range n.Terminals {
		if terminal.Kind != NodeKindTerminal {
			return itemError("SubgraphNode.terminals", ErrInconsistentValue)
		}
	}
	if n.Record != nil {
		if n.Kind != NodeKindRecord {
			return itemError("SubgraphNode.record", ErrInconsistentValue)
		}
		if err := n.Record.Validate(); err != nil {
			return err
		}
	}
	return validateElements("SubgraphNode.terminals", n.Terminals)
}

// ValueCount は、集計の対象にした欄に観測した値 1 件と、その件数と時刻の両端である。
//
// **数えるのはレコードである。** 同じ値を対象のノードとレコードのノードの両方が属性に
// 持つため、ノードを数えると 1 つの観測を 2 回数える。RecordCount はその値を観測した
// レコードの異なりの件数である。
type ValueCount struct {
	// Value は観測した値である。正規化値を持つ値は正規化値、原資料の文字列だけを持つ値は原資料の文字列で
	// ある (RawAndNormalized.ComparableValue)。**外部由来の文字列であり、出力境界で
	// 無害化してから画面へ出す。**
	Value string `json:"value"`
	// RecordCount はこの値を観測したレコードの件数である。
	RecordCount int64 `json:"recordCount"`
	// FirstEventTime と LastEventTime は、この値を観測したレコードの時刻の両端である。
	//
	// **絶対時刻として読める根拠を 1 件も持たない値では出ない。** 時刻を読めない状態を
	// 既定の日付で埋めない。
	FirstEventTime *Timestamp `json:"firstEventTime,omitempty"`
	LastEventTime  *Timestamp `json:"lastEventTime,omitempty"`
	// Intervals は、この値を観測したレコードを時刻の順に並べたときの、隣り合う 2 件の時刻の
	// 差の分布である。**時点を持つレコードが 2 件未満の値では出ない。**
	Intervals *EventIntervals `json:"intervals,omitempty"`
}

// Validate は項目の整合を確かめる。
func (c ValueCount) Validate() error {
	problem := firstProblem(
		requirePresent("ValueCount.value", c.Value),
		requireNonNegative("ValueCount.recordCount", c.RecordCount),
	)
	if problem != nil {
		return problem
	}
	for _, bound := range []struct {
		name      string
		timestamp *Timestamp
	}{
		{"ValueCount.firstEventTime", c.FirstEventTime},
		{"ValueCount.lastEventTime", c.LastEventTime},
	} {
		if bound.timestamp == nil {
			continue
		}
		if err := bound.timestamp.Validate(); err != nil {
			return itemError(bound.name, err)
		}
	}
	// 片方だけを持つ両端を作らない。2 つは同じ根拠の集合から決まる。
	if (c.FirstEventTime == nil) != (c.LastEventTime == nil) {
		return itemError("ValueCount.firstEventTime and ValueCount.lastEventTime",
			ErrInconsistentValue)
	}
	if c.Intervals != nil {
		return c.Intervals.validate(c.RecordCount)
	}
	return nil
}

// IntervalBoundsMilliseconds は、EventIntervals.BinCounts の階級の境界である。階級 i は
// [境界 i-1, 境界 i) であり、最初の階級の下端は 0、最後の階級は上端を持たない。
var IntervalBoundsMilliseconds = []int64{
	1_000, 5_000, 10_000, 30_000, 60_000, 300_000, 600_000, 1_800_000, 3_600_000, 21_600_000, 86_400_000,
}

// EventIntervals は、レコードの時刻の隣り合う差の分布である。
//
// **時点を持つレコードだけを並べる。** UTC からのずれが決まらない時刻のレコードは入らない。
// 端末や接続元で系列を分けない。差はミリ秒であり、ミリ秒より細かい桁を切り捨てる。
//
// 四分位と中央値は、差を小さい順に並べた n 個の d[0..n-1] の d[⌊p×(n-1)⌋] である
// (p は 1/4、1/2、3/4)。n が偶数のときの中央値は中央の 2 つのうち小さいほうである。
type EventIntervals struct {
	// TimedRecordCount は差を取ったレコードの件数である。差の件数は TimedRecordCount-1 である。
	TimedRecordCount          int64 `json:"timedRecordCount"`
	MinMilliseconds           int64 `json:"minMilliseconds"`
	LowerQuartileMilliseconds int64 `json:"lowerQuartileMilliseconds"`
	MedianMilliseconds        int64 `json:"medianMilliseconds"`
	UpperQuartileMilliseconds int64 `json:"upperQuartileMilliseconds"`
	MaxMilliseconds           int64 `json:"maxMilliseconds"`
	// BinCounts は IntervalBoundsMilliseconds の階級ごとの差の件数である。要素数は境界の数 + 1
	// であり、件数 0 の階級も並べる。
	BinCounts []int64 `json:"binCounts"`
	// CoarsePrecision は、差を取った時刻に、秒か秒より粗い精度の時刻を含むかである。
	// 含むとき、1 秒未満の差と 0 の差は同じ秒に記録された 2 件を表しうる。
	CoarsePrecision bool `json:"coarsePrecision,omitempty"`
}

// validate は、分布の整合を、値を観測したレコードの件数 recordCount とともに確かめる。
func (i EventIntervals) validate(recordCount int64) error {
	if i.TimedRecordCount < 2 || i.TimedRecordCount > recordCount {
		return itemError("EventIntervals.timedRecordCount", ErrInconsistentValue)
	}
	ordered := []int64{i.MinMilliseconds, i.LowerQuartileMilliseconds, i.MedianMilliseconds,
		i.UpperQuartileMilliseconds, i.MaxMilliseconds}
	if ordered[0] < 0 || !slices.IsSorted(ordered) {
		return itemError("EventIntervals quantiles", ErrInconsistentValue)
	}
	if len(i.BinCounts) != len(IntervalBoundsMilliseconds)+1 {
		return itemError("EventIntervals.binCounts", ErrInconsistentValue)
	}
	var sum int64
	for _, count := range i.BinCounts {
		if count < 0 {
			return itemError("EventIntervals.binCounts", ErrInconsistentValue)
		}
		sum += count
	}
	if sum != i.TimedRecordCount-1 {
		return itemError("EventIntervals.binCounts", ErrInconsistentValue)
	}
	return nil
}

// GraphEvidence は関係またはノードの根拠のレコード 1 件である。
//
// **原資料の値を持たない。** 値は recordRef を渡した `/api/v0/records` が返す。
type GraphEvidence struct {
	// RecordRef は根拠のレコードの位置である。
	RecordRef RecordLocator `json:"recordRef"`
	// EventTime はレコードが記録した事象の時刻である。時刻を読めなかったレコードでは出ない。
	EventTime *Timestamp `json:"eventTime,omitempty"`
	// ObservationKind はレコードの観測の種別と、その意味の状態である。
	ObservationKind ObservationKind `json:"observationKind"`
	// EventKind はレコードの事象の分類と動作の組である。根拠のレコードを絞る条件の
	// eventCategory と eventAction にそのまま渡せる。分類を持たないレコードでは出ない。
	EventKind *EventKindPair `json:"eventKind,omitempty"`
}

// EventKindPair は事象の分類と動作の組である。Action が空の組は、動作の欄を持たない
// レコードの組である。
type EventKindPair struct {
	Category string `json:"category"`
	Action   string `json:"action"`
}

// Validate は項目の整合を確かめる。
func (e GraphEvidence) Validate() error {
	if err := e.RecordRef.Validate(); err != nil {
		return itemError("GraphEvidence.recordRef", err)
	}
	if e.EventTime != nil {
		if err := e.EventTime.Validate(); err != nil {
			return itemError("GraphEvidence.eventTime", err)
		}
	}
	if err := e.ObservationKind.Validate(); err != nil {
		return itemError("GraphEvidence.observationKind", err)
	}
	if e.EventKind != nil && e.EventKind.Category == "" {
		return itemError("GraphEvidence.eventKind.category", ErrMissingRequiredItem)
	}
	return nil
}

// GraphEdge はグラフのエッジ 1 本を持つ。
//
// **同じ (kind, sourceNodeId, targetNodeId) の観測を 1 本にまとめる。** イベント 1 件ごとの
// 根拠は Evidence の要素が保ち、まとめる前の件数は EvidenceCount が持つ。
type GraphEdge struct {
	// Id はエッジを指す不透明な識別子である。
	Id string `json:"id"`
	// Kind は関係の種別である。
	Kind EdgeKind `json:"kind"`
	// State は関係の状態である。
	State RelationState `json:"state"`
	// SourceNodeId は起点のノードの識別子である。
	SourceNodeId string `json:"sourceNodeId"`
	// TargetNodeId は終点のノードの識別子である。
	TargetNodeId string `json:"targetNodeId"`
	// ApplicableRange は根拠のレコードの時刻の両端である。絶対時刻として読める根拠を
	// 1 件も持たないエッジでは出ない。**時刻を読めない状態を既定の日付で埋めない。**
	//
	// 根拠のレコードを 1 件も持たず、端末の割当から作ったエッジでは、割当の適用期間の両端である
	// (AssignmentOrigins が出る)。
	ApplicableRange *TimeRange `json:"applicableRange,omitempty"`
	// AssignmentOrigins は、エッジを作った端末の割当の由来である。同じ由来を 2 度置かない。
	// 端末の割当から作っていないエッジでは出ない。
	AssignmentOrigins []TerminalAssignmentOrigin `json:"assignmentOrigins,omitempty"`
	// EvidenceCount は根拠のレコードの件数である。
	//
	// **根拠の中身をこの型が持たない。** `/api/v0/graph` は図を組む材料だけを返し、
	// 根拠の中身は `/api/v0/edges/{id}` が自分の応答型で持つ。
	EvidenceCount int64 `json:"evidenceCount"`
	// EvidenceByCase は EvidenceCount を根拠のレコードの案件ごとに分けた件数である。
	// 案件を区別しない取り込みでは出ない。
	EvidenceByCase []CaseEvidenceCount `json:"evidenceByCase,omitempty"`
	// IndistinguishableCandidateCount は、関連付けがこのエッジの候補を、互いに区別できない候補の組
	// (CandidateStage.IndistinguishableGroups) に入れた起点での、組の候補が指すノードの数の最大である。
	// このエッジの終点を含む。2 以上の値だけが出る。どの起点でも、別のノードを指す候補と同じ組に
	// 入らないエッジでは出ない。
	IndistinguishableCandidateCount int64 `json:"indistinguishableCandidateCount,omitempty"`
	// CandidateTier は、関連付けが終点のレコードに挙がった候補を並べた、このエッジの区分である。
	// 候補を区分で並べる関係 (EdgeKindLogonChain) だけが持つ。応答のエッジの並びは、区分を持たない
	// エッジを先に置き、区分を持つエッジを区分の番号の順に続ける。
	CandidateTier *EdgeCandidateTier `json:"candidateTier,omitempty"`
}

// EdgeCandidateTier は、候補のエッジ 1 本を並べた区分である。ログオンの連鎖では、アカウントが
// 一致する候補を上に、次にログオンの種別の区分 (対話と画面の遠隔操作、その他、ネットワークの順) で
// 並べる (EdgeCandidateTally)。
type EdgeCandidateTier struct {
	// Tier は区分の番号である。1 が並びの最も上の区分である。
	Tier int `json:"tier"`
	// Conditions は区分を決めた条件である。アカウントの条件、ログオンの種別の条件の順である。
	Conditions []EdgePairConditionKey `json:"conditions"`
}

// validate は区分の番号が 1 以上であることと、条件が 1 つ以上の既知の値であることを確かめる。
func (t EdgeCandidateTier) validate() error {
	if t.Tier < 1 {
		return itemError("GraphEdge.candidateTier.tier", ErrInvalid)
	}
	if len(t.Conditions) == 0 {
		return itemError("GraphEdge.candidateTier.conditions", ErrMissingRequiredItem)
	}
	for _, condition := range t.Conditions {
		if err := requireKnownEnum("GraphEdge.candidateTier.conditions", condition); err != nil {
			return err
		}
	}
	return nil
}

// CaseEvidenceCount は案件 1 つに属する根拠のレコードの件数である。
//
// 並びは案件の識別子の昇順である。根拠を 1 件も持たない案件は要素にしない。
type CaseEvidenceCount struct {
	// CaseId は収集元に付けた案件である。文字列は ValidateCaseId が定める。
	CaseId string `json:"caseId"`
	// EvidenceCount はその案件の収集元から来た根拠のレコードの件数である。1 以上である。
	EvidenceCount int64 `json:"evidenceCount"`
}

// validateEvidenceByCase は案件ごとの件数の文字列と並びと、和が全体の件数に等しいことを確かめる。
func validateEvidenceByCase(item string, counts []CaseEvidenceCount, total int64) error {
	if counts == nil {
		return nil
	}
	var sum int64
	for index, count := range counts {
		if err := ValidateCaseId(item+".caseId", count.CaseId); err != nil {
			return err
		}
		if count.EvidenceCount < 1 {
			return itemError(item+".evidenceCount", ErrInvalid)
		}
		if index > 0 && counts[index-1].CaseId >= count.CaseId {
			return itemError(item+".caseId", ErrInconsistentValue)
		}
		sum += count.EvidenceCount
	}
	if sum != total {
		return itemError(item, ErrInconsistentValue)
	}
	return nil
}

// Validate は項目の整合を確かめる。
func (e GraphEdge) Validate() error {
	problem := firstProblem(
		requirePresent("GraphEdge.id", e.Id),
		requireKnownEnum("GraphEdge.kind", e.Kind),
		requireKnownEnum("GraphEdge.state", e.State),
		requirePresent("GraphEdge.sourceNodeId", e.SourceNodeId),
		requirePresent("GraphEdge.targetNodeId", e.TargetNodeId),
		requireNonNegative("GraphEdge.evidenceCount", e.EvidenceCount),
		validateEvidenceByCase("GraphEdge.evidenceByCase", e.EvidenceByCase, e.EvidenceCount),
	)
	if problem == nil && (e.IndistinguishableCandidateCount == 1 || e.IndistinguishableCandidateCount < 0) {
		problem = itemError("GraphEdge.indistinguishableCandidateCount", ErrInvalid)
	}
	if problem == nil && e.CandidateTier != nil {
		problem = e.CandidateTier.validate()
	}
	if problem != nil {
		return problem
	}
	if e.ApplicableRange != nil {
		if err := e.ApplicableRange.Validate(); err != nil {
			return itemError("GraphEdge.applicableRange", err)
		}
	}
	for _, origin := range e.AssignmentOrigins {
		if err := requireKnownEnum("GraphEdge.assignmentOrigins", origin); err != nil {
			return err
		}
	}
	return nil
}

// EdgeEvidenceAccount は、根拠の区分のレコードに現れたアカウント 1 つである。
//
// **ノードを丸ごと持つ。** 分析者が関係の詳細からアカウントへ辿るには識別子と表示名と
// 識別鍵の値が要り、両端のノードを持つのと同じ形で渡す。
type EdgeEvidenceAccount struct {
	// Node はアカウントのノードである。
	Node GraphNode `json:"node"`
	// EvidenceCount はそのアカウントが現れた、区分の中の根拠のレコードの件数である。
	EvidenceCount int64 `json:"evidenceCount"`
}

// Validate は項目の整合を確かめる。
func (a EdgeEvidenceAccount) Validate() error {
	if err := a.Node.Validate(); err != nil {
		return itemError("EdgeEvidenceAccount.node", err)
	}
	if a.Node.Kind != NodeKindAccount {
		return itemError("EdgeEvidenceAccount.node carries the kind "+string(a.Node.Kind)+
			" beside the kind "+string(NodeKindAccount), ErrInconsistentValue)
	}
	if a.EvidenceCount < 1 {
		return itemError("EdgeEvidenceAccount.evidenceCount", ErrInvalid)
	}
	return nil
}

// EdgeEvidenceSelector は根拠の区分 1 つを要求で指す値である。
//
// **値を組むのは backend である。** 画面は応答が返した値をそのまま要求へ返す。比べられる
// 値を作る規則は adapter と core が持ち、画面が同じ規則を持つと 2 つがずれる。
//
// 観測の種別を持たない HTTP の要求の区分では、EventCategory と EventAction が空の文字列である。
// 要求は空の種別を項目に載せない。種別の項目を持たず HTTP の状態を持つ要求は、観測の種別を
// 持たないレコードだけを通す。
type EdgeEvidenceSelector struct {
	// EventCategory は観測の種別の category の比べられる値である。
	EventCategory string `json:"eventCategory"`
	// EventAction は観測の種別の action の比べられる値である。
	EventAction string `json:"eventAction"`
	// DestinationPort は接続先 port の比べられる値である。
	// 接続先 port の欄を持たない区分では出ない。
	DestinationPort string `json:"destinationPort,omitempty"`
	// DestinationPortAbsent は、接続先 port の欄を持たないレコードの区分であることを表す。
	// **欄が無いことを、値が無いことと同じ文字列にしない。** 接続先 port を持つ区分では出ない。
	DestinationPortAbsent bool `json:"destinationPortAbsent,omitempty"`
	// LogonType はログオンの種別のコードの比べられる値である。
	// ログオンの種別を持たない区分では出ない。
	LogonType string `json:"logonType,omitempty"`
	// LogonTypeAbsent は、ログオンの種別を持たないレコードの区分であることを表す。
	// **種別の値が無いことを、コード 0 と同じ文字列にしない。** 種別を持つ区分では出ない。
	LogonTypeAbsent bool `json:"logonTypeAbsent,omitempty"`
	// HttpStatus は HTTP の要求のレコードが記録した HTTP の状態の比べられる値である。
	// HttpStatusAbsent は、HTTP の状態の値を持たない HTTP の要求のレコードの区分であることを
	// 表す。HTTP の要求でないレコードの区分では、どちらも出ず、状態で絞らない。
	HttpStatus       string `json:"httpStatus,omitempty"`
	HttpStatusAbsent bool   `json:"httpStatusAbsent,omitempty"`
}

// Validate は項目の整合と、接続先 port とログオンの種別のそれぞれで、値と不在が排他で
// あることを確かめる。HTTP の状態は、値と不在を同時に持たないことを確かめる。
func (s EdgeEvidenceSelector) Validate() error {
	// HTTP の要求の区分は、観測の種別を持たない入力形式 (Proxy のログ) でも HTTP の状態で指せる。
	if s.HttpStatus == "" && !s.HttpStatusAbsent || s.EventCategory != "" || s.EventAction != "" {
		problem := firstProblem(
			requirePresent("EdgeEvidenceSelector.eventCategory", s.EventCategory),
			requirePresent("EdgeEvidenceSelector.eventAction", s.EventAction),
		)
		if problem != nil {
			return problem
		}
	}
	if s.HttpStatus != "" && s.HttpStatusAbsent {
		return itemError("EdgeEvidenceSelector takes either an httpStatus or the flag "+
			"saying the value is absent", ErrInconsistentValue)
	}
	if (s.DestinationPort != "") == s.DestinationPortAbsent {
		return itemError("EdgeEvidenceSelector takes either a destinationPort or the flag "+
			"saying the field is absent", ErrInconsistentValue)
	}
	if (s.LogonType != "") == s.LogonTypeAbsent {
		return itemError("EdgeEvidenceSelector takes either a logonType or the flag "+
			"saying the value is absent", ErrInconsistentValue)
	}
	return nil
}

// EdgeEvidenceGroup は、エッジ 1 本の根拠を観測の種別と接続先 port とログオンの種別で分けた
// 1 区分である。
//
// **エッジを分けずに根拠を分ける。** 関係の同一性は種別と両端のノードであり、接続先 port を
// 同一性へ足すと、接続先 port を持たないレコードの根拠が行き先を失う。1 本のエッジが
// 遠隔ログインと SMB と WinRM を同時に持つ入力では、用いた手段を区分が分ける。
//
// 件数はエッジの根拠の全数から数え、ページに載らない根拠も区分の件数に入る。
//
// 区分を分ける鍵は、欄の不在と、欄はあるが値を比べられない状態を別の区分にする。
// 2 つを同じ区分にまとめると、片方の集団に他方の理由が付く。
type EdgeEvidenceGroup struct {
	// ObservationKind は区分に属するレコードの観測の種別である。
	ObservationKind ObservationKind `json:"observationKind"`
	// DestinationPort は区分に属するレコードが記録した接続先 port である。
	// 接続先 port の欄を持たない区分と、欄の値を比べられない区分では出ない。
	DestinationPort *RecordField `json:"destinationPort,omitempty"`
	// DestinationPortAbsence は接続先 port が出ない理由である。
	// **欠測を 0 で埋めない。** 接続先 port を持つ区分では出ない。
	DestinationPortAbsence string `json:"destinationPortAbsence,omitempty"`
	// LogonType は区分に属するレコードが記録したログオンの種別のコードである。
	// 種別を持たない区分と、種別の値を比べられない区分では出ない。
	LogonType *RecordField `json:"logonType,omitempty"`
	// LogonTypeAbsence はログオンの種別が出ない理由である。
	// **欠測を 0 で埋めない。** 種別を持つ区分では出ない。
	LogonTypeAbsence string `json:"logonTypeAbsence,omitempty"`
	// HttpStatus は区分に属する HTTP の要求のレコードが記録した HTTP の状態である。
	// HttpStatusAbsence は HTTP の要求のレコードの区分で HTTP の状態が出ない理由である。
	// HTTP の要求でないレコードの区分では、どちらも出ない。
	HttpStatus        *RecordField `json:"httpStatus,omitempty"`
	HttpStatusAbsence string       `json:"httpStatusAbsence,omitempty"`
	// Selector は区分を要求で指す値である。指せない区分では出ない。
	Selector *EdgeEvidenceSelector `json:"selector,omitempty"`
	// SelectorAbsence は区分を要求で指せない理由である。指せる区分では出ない。
	// **指せないことを応答が出す。** 画面が押せる操作を出して 0 件を返す形にしない。
	SelectorAbsence string `json:"selectorAbsence,omitempty"`
	// Accounts は区分に属するレコードに現れたアカウントである。要素数 0 の場合も集合である。
	Accounts []EdgeEvidenceAccount `json:"accounts"`
	// Authentications は区分に属するレコードが記録した認証の方式である。要素数 0 の場合も
	// 集合である。
	Authentications []EdgeEvidenceAuthentication `json:"authentications"`
	// EvidenceCount は区分に属する根拠のレコードの件数である。
	EvidenceCount int64 `json:"evidenceCount"`
}

// EdgeEvidenceAuthentication は、根拠の区分のレコードが記録した認証の方式 1 つである。
type EdgeEvidenceAuthentication struct {
	// Value は、その方式を記録した最初のレコードの欄である。比べられる値が同じ欄を 1 つにまとめる。
	Value RecordField `json:"value"`
	// EvidenceCount はその方式を記録した、区分の中の根拠のレコードの件数である。
	EvidenceCount int64 `json:"evidenceCount"`
}

// Validate は項目の整合を確かめる。
func (a EdgeEvidenceAuthentication) Validate() error {
	if err := a.Value.Validate(); err != nil {
		return itemError("EdgeEvidenceAuthentication.value", err)
	}
	if a.EvidenceCount < 1 {
		return itemError("EdgeEvidenceAuthentication.evidenceCount", ErrInvalid)
	}
	return nil
}

// Validate は項目の整合と、値と理由の組がそれぞれ排他であることを確かめる。
func (g EdgeEvidenceGroup) Validate() error {
	if err := g.ObservationKind.Validate(); err != nil {
		return itemError("EdgeEvidenceGroup.observationKind", err)
	}
	if problem := validateGroupField("destinationPort", g.DestinationPort,
		g.DestinationPortAbsence); problem != nil {
		return problem
	}
	if problem := validateGroupField("logonType", g.LogonType, g.LogonTypeAbsence); problem != nil {
		return problem
	}
	if g.HttpStatus != nil || g.HttpStatusAbsence != "" {
		if problem := validateGroupField("httpStatus", g.HttpStatus, g.HttpStatusAbsence); problem != nil {
			return problem
		}
	}
	if problem := g.validateSelector(); problem != nil {
		return problem
	}
	if problem := validateElements("EdgeEvidenceGroup.accounts", g.Accounts); problem != nil {
		return problem
	}
	if problem := validateElements("EdgeEvidenceGroup.authentications", g.Authentications); problem != nil {
		return problem
	}
	if g.EvidenceCount < 1 {
		return itemError("EdgeEvidenceGroup.evidenceCount", ErrInvalid)
	}
	return nil
}

// validateGroupField は区分の欄 1 つについて、欄と、欄が出ない理由のどちらか一方だけが
// あることを確かめる。name は欄の JSON の名前である。
func validateGroupField(name string, field *RecordField, absence string) error {
	if field == nil {
		return requirePresent("EdgeEvidenceGroup."+name+"Absence", absence)
	}
	if err := field.Validate(); err != nil {
		return itemError("EdgeEvidenceGroup."+name, err)
	}
	if absence != "" {
		return itemError("EdgeEvidenceGroup carries both a "+name+" and the reason "+
			"it is absent", ErrInconsistentValue)
	}
	return nil
}

func (g EdgeEvidenceGroup) validateSelector() error {
	if g.Selector == nil {
		return requirePresent("EdgeEvidenceGroup.selectorAbsence", g.SelectorAbsence)
	}
	if err := g.Selector.Validate(); err != nil {
		return itemError("EdgeEvidenceGroup.selector", err)
	}
	if g.SelectorAbsence != "" {
		return itemError("EdgeEvidenceGroup carries both a selector and the reason it is absent",
			ErrInconsistentValue)
	}
	return nil
}

// MarshalJSON は必須の集合を要素数 0 の場合も集合として出す。
func (g EdgeEvidenceGroup) MarshalJSON() ([]byte, error) {
	// items は EdgeEvidenceGroup の method を持たないため、この Marshal は再帰しない。
	type items EdgeEvidenceGroup
	copied := items(g)
	copied.Accounts = emptyIfNil(copied.Accounts)
	encoded, err := json.Marshal(copied)
	if err != nil {
		return nil, fmt.Errorf("marshaling EdgeEvidenceGroup: %w", err)
	}
	return encoded, nil
}

// MatchStageTally は、1 つの起点に対して段階 1 つが挙げた候補を数えた結果である。
//
// **候補を 0 件にした段階も要素になる。** 段階そのものが結果から消えると、その段階を実行して
// 0 件だったのか、その段階を実行していないのかを読み分けられない。
type MatchStageTally struct {
	// StageKey は段階の種別である。
	StageKey StageKey `json:"stageKey"`
	// MemberCount は、この起点に対して段階が挙げた候補の総数である。
	MemberCount int64 `json:"memberCount"`
	// DistinctProcessCount は、段階が挙げた候補が指す異なるプロセスの個数である。
	// 出ない場合は候補がプロセスの項目を持たない。
	DistinctProcessCount *int64 `json:"distinctProcessCount,omitempty"`
	// EmptyReason は候補が 0 件になった理由である。候補が 1 件以上のとき出ない。
	EmptyReason EmptyReason `json:"emptyReason,omitempty"`
}

// Validate は項目の整合を確かめる。
func (t MatchStageTally) Validate() error {
	if problem := firstProblem(
		requireKnownEnum("MatchStageTally.stageKey", t.StageKey),
		requireNonNegative("MatchStageTally.memberCount", t.MemberCount),
	); problem != nil {
		return problem
	}
	if t.DistinctProcessCount != nil {
		if problem := requireNonNegative("MatchStageTally.distinctProcessCount",
			*t.DistinctProcessCount); problem != nil {
			return problem
		}
		if *t.DistinctProcessCount > t.MemberCount {
			return itemError("MatchStageTally.distinctProcessCount is larger than "+
				"MatchStageTally.memberCount", ErrInconsistentValue)
		}
	}
	// 候補を挙げた段階は理由を持たない。0 件の段階は理由を持つ。
	if t.MemberCount > 0 {
		if t.EmptyReason != "" {
			return itemError("MatchStageTally.emptyReason on a stage that has candidates",
				ErrInconsistentValue)
		}
		return nil
	}
	if problem := requireKnownEnum("MatchStageTally.emptyReason", t.EmptyReason); problem != nil {
		return problem
	}
	// **段階の種別ごとに置ける理由を絞る。** 時刻を比べない段階に「時刻の範囲の中に候補が無い」を
	// 置くと、実行していない比較の結果を示すことになる。判定は段階が持つ規則を用いる。
	if !t.EmptyReason.isStageReason(t.StageKey) {
		return itemError("MatchStageTally.emptyReason "+string(t.EmptyReason)+
			" on the stage "+string(t.StageKey), ErrInconsistentValue)
	}
	return nil
}

// EdgeMatch は候補のエッジ 1 本を作った関連付けの結果 1 件である。
// 起点のレコード 1 件と候補のレコード 1 件の組について、用いた条件と依拠した前提と
// 時刻の比較を持つ。両方のレコードと関連付けの条件が、収集元をまたいだ通信の関連付けの根拠になる。
//
// **まとめたエッジ 1 本が要素を 2 件以上持つ。** 同じ 2 つのノードを結ぶ関連付けが 2 件以上
// あるとき、条件の値は関連付けごとに異なる。
//
// 応答はエッジの関連付けを EdgeMatchTable の形で持つ。関連付けの結果 1 件の値は EdgeMatchTable.Match が
// 組み直す。
type EdgeMatch struct {
	// OriginRef は関連付けの起点のレコードの位置である。
	OriginRef RecordLocator `json:"originRef"`
	// CandidateRef は候補のレコードの位置である。
	CandidateRef RecordLocator `json:"candidateRef"`
	// StageKey は候補を挙げた段階の種別である。
	StageKey StageKey `json:"stageKey"`
	// Conditions は段階が関連付けに用いた条件と、用いなかった条件の全数の集合である。
	Conditions []MatchCondition `json:"conditions"`
	// Assumptions は段階が依拠する前提の集合である。要素数 0 の場合も集合である。
	Assumptions []MatchAssumption `json:"assumptions"`
	// TimeWindow は関連付けに用いた時刻の範囲である。
	TimeWindow TimeWindow `json:"timeWindow"`
	// TimeComparison は起点と候補の時刻の比較である。
	TimeComparison TimeComparison `json:"timeComparison"`
	// ClockDependencyNote は関連付けが収集元の時計に依拠する箇所である。
	//
	// **前提 (Assumptions) と別の項目である。** 前提は段階 2 が時刻の一致に置く仮定であり、
	// 本項目は段階 1 から引き継ぐ terminal_ip_assignment の条件が時計に依拠する箇所を表す。
	// 段階 2 は段階 1 の条件を引き継ぐため、この制約は候補のエッジにも該当する。
	ClockDependencyNote string `json:"clockDependencyNote"`
	// StageTallies は、この起点に対して関連付けが求めた段階を、段階の並び順ですべて数えた結果で
	// ある。要素数は 1 以上で、末尾の要素の段階がこの関連付けを出した段階である。
	//
	// **関連付けの結果 1 件だけでは候補の確度を読めない。** 同じ起点が候補を 1 件だけ挙げたのか
	// 5 件挙げたのかで、この関連付けが表す確からしさが変わる。段階ごとの件数の差が、時刻の
	// 条件がどれだけ候補を減らしたかを示す。
	StageTallies []MatchStageTally `json:"stageTallies"`
	// IndistinguishableGroups は、段階が用いた条件の値がすべて同じで互いに区別できない候補の
	// 組の集合である。要素数 0 の場合も集合である。
	//
	// **候補の確度を分析者が判断する材料である。** 組の要素は、段階が用いた条件だけでは
	// 互いに区別できない。
	IndistinguishableGroups [][]RecordLocator `json:"indistinguishableGroups"`
	// UnresolvedReasons は確定しない理由の集合である。要素数は 1 以上である。
	UnresolvedReasons []string `json:"unresolvedReasons"`
}

// Validate は項目の整合を確かめる。
func (m EdgeMatch) Validate() error {
	if err := m.OriginRef.Validate(); err != nil {
		return itemError("EdgeMatch.originRef", err)
	}
	if err := m.CandidateRef.Validate(); err != nil {
		return itemError("EdgeMatch.candidateRef", err)
	}
	if err := validateMatchStageItems("EdgeMatch", m.StageKey, m.Conditions, m.Assumptions, m.TimeWindow,
		m.ClockDependencyNote, m.StageTallies); err != nil {
		return err
	}
	if err := m.TimeComparison.Validate(); err != nil {
		return itemError("EdgeMatch.timeComparison", err)
	}
	if problem := m.validateGroups(); problem != nil {
		return problem
	}
	return validateUnresolvedReasons("EdgeMatch.unresolvedReasons", m.UnresolvedReasons)
}

// validateUnresolvedReasons は確定しない理由の集合を確かめる。要素数は 1 以上である。
func validateUnresolvedReasons(name string, reasons []string) error {
	if len(reasons) == 0 {
		return itemError(name, ErrMissingRequiredItem)
	}
	return requireNoEmptyElement(name, reasons)
}

// validateMatchStageItems は、関連付けを出した段階が関連付けに共有する項目を確かめる。
// owner は項目の名前に付ける型の名前である。
//
// **末尾の段階が関連付けを出した段階である。** 関連付けはその段階の候補 1 件について組むため、
// 末尾の段階の候補の総数は 1 以上である。
func validateMatchStageItems(
	owner string, stageKey StageKey, conditions []MatchCondition, assumptions []MatchAssumption,
	window TimeWindow, clockDependencyNote string, tallies []MatchStageTally,
) error {
	if problem := requireKnownEnum(owner+".stageKey", stageKey); problem != nil {
		return problem
	}
	// 条件の集合は候補 1 件を伴うため、候補の総数 1 で確かめる。
	if err := ValidateMatchConditions(conditions, 1); err != nil {
		return itemError(owner+".conditions", err)
	}
	if problem := validateAssumptions(owner+".assumptions", assumptions); problem != nil {
		return problem
	}
	if err := window.Validate(); err != nil {
		return itemError(owner+".timeWindow", err)
	}
	if problem := requirePresent(owner+".clockDependencyNote", clockDependencyNote); problem != nil {
		return problem
	}
	if len(tallies) == 0 {
		return itemError(owner+".stageTallies", ErrMissingRequiredItem)
	}
	for index, tally := range tallies {
		if err := tally.Validate(); err != nil {
			return itemError(owner+".stageTallies at "+formatIndex(index), err)
		}
	}
	last := tallies[len(tallies)-1]
	if last.StageKey != stageKey {
		return itemError(owner+".stageTallies does not end at "+owner+".stageKey", ErrInconsistentValue)
	}
	if last.MemberCount < 1 {
		return itemError(owner+".stageTallies at the stage of this match", ErrInvalid)
	}
	return nil
}

// validateGroups は互いに区別できない要素の組を確かめる。
func (m EdgeMatch) validateGroups() error {
	for groupIndex, group := range m.IndistinguishableGroups {
		if len(group) < minIndistinguishableGroupSize {
			return itemError("EdgeMatch.indistinguishableGroups at "+formatIndex(groupIndex),
				ErrMissingRequiredItem)
		}
		for index, locator := range group {
			if err := locator.Validate(); err != nil {
				return itemError("EdgeMatch.indistinguishableGroups at "+
					formatIndex(groupIndex)+" "+formatIndex(index), err)
			}
		}
	}
	return nil
}

// MarshalJSON は必須の集合を要素数 0 の場合も集合として出す。
func (m EdgeMatch) MarshalJSON() ([]byte, error) {
	// items は EdgeMatch の method を持たないため、この Marshal は再帰しない。
	type items EdgeMatch
	copied := items(m)
	copied.Assumptions = emptyIfNil(copied.Assumptions)
	copied.IndistinguishableGroups = emptyIfNil(copied.IndistinguishableGroups)
	encoded, err := json.Marshal(copied)
	if err != nil {
		return nil, fmt.Errorf("marshaling EdgeMatch: %w", err)
	}
	return encoded, nil
}

// EdgeDirection はノードから見たエッジの向きである。
type EdgeDirection string

// EdgeDirection の値。
const (
	// EdgeDirectionOutgoing はノードを起点とするエッジである。
	EdgeDirectionOutgoing EdgeDirection = "outgoing"
	// EdgeDirectionIncoming はノードを終点とするエッジである。
	EdgeDirectionIncoming EdgeDirection = "incoming"
)

// IsKnown は EdgeDirection が定義の中の値であるかを返す。
func (d EdgeDirection) IsKnown() bool {
	return d == EdgeDirectionOutgoing || d == EdgeDirectionIncoming
}

// NodeEdgeCount はノード 1 つに繋がるエッジの、種別と向きごとの件数である。
type NodeEdgeCount struct {
	// EdgeKind は関係の種別である。
	EdgeKind EdgeKind `json:"edgeKind"`
	// Direction はノードから見たエッジの向きである。
	Direction EdgeDirection `json:"direction"`
	// EdgeCount はまとめたエッジの本数である。
	EdgeCount int64 `json:"edgeCount"`
	// EvidenceCount はそのエッジが持つ根拠のレコードの総数である。
	EvidenceCount int64 `json:"evidenceCount"`
	// EvidenceByCase は EvidenceCount を根拠のレコードの案件ごとに分けた件数である。
	// 案件を区別しない取り込みでは出ない。
	EvidenceByCase []CaseEvidenceCount `json:"evidenceByCase,omitempty"`
}

// Validate は項目の整合を確かめる。
func (c NodeEdgeCount) Validate() error {
	return firstProblem(
		requireKnownEnum("NodeEdgeCount.edgeKind", c.EdgeKind),
		requireKnownEnum("NodeEdgeCount.direction", c.Direction),
		requireNonNegative("NodeEdgeCount.edgeCount", c.EdgeCount),
		requireNonNegative("NodeEdgeCount.evidenceCount", c.EvidenceCount),
		validateEvidenceByCase("NodeEdgeCount.evidenceByCase", c.EvidenceByCase, c.EvidenceCount),
	)
}

// NodeAttributeValue はノードの 1 つの属性に観測した値 1 件である。
type NodeAttributeValue struct {
	// Field は観測した項目である。原資料の key の文字列と値を持つ。
	Field RecordField `json:"field"`
	// ObservationCount はこの値を観測したレコードの件数である。
	ObservationCount int64 `json:"observationCount"`
	// FirstRecordRef はこの値を最初に観測したレコードの位置である。
	FirstRecordRef RecordLocator `json:"firstRecordRef"`
}

// Validate は項目の整合を確かめる。
func (v NodeAttributeValue) Validate() error {
	if err := v.Field.Validate(); err != nil {
		return itemError("NodeAttributeValue.field", err)
	}
	if problem := requireNonNegative("NodeAttributeValue.observationCount",
		v.ObservationCount); problem != nil {
		return problem
	}
	if err := v.FirstRecordRef.Validate(); err != nil {
		return itemError("NodeAttributeValue.firstRecordRef", err)
	}
	return nil
}

// NodeAttribute はノード 1 つの 1 つの意味に観測した値の集合である。
//
// **矛盾する値を 1 つに寄せない。** 同じ意味に異なる値を観測したノードは Values に
// 2 件以上を持ち、ValueCount がその個数を持つ。
type NodeAttribute struct {
	// Semantic は属性の語彙の項目である。語彙に写していない欄では出ない。
	Semantic SemanticKey `json:"semantic,omitempty"`
	// Name は原資料の key の文字列である。**語彙に写していない欄でだけ出る。**
	// 語彙の項目を持つ欄では、同じ意味を持つ別の key を 1 つの属性にまとめるため出ない。
	// まとめた各値の原資料の key は Values の要素の Field.Name が持つ。
	Name string `json:"name,omitempty"`
	// ValueCount は観測した異なる値の個数である。
	ValueCount int64 `json:"valueCount"`
	// Values は観測した値である。要素数は 1 以上である。
	Values []NodeAttributeValue `json:"values"`
}

// Validate は項目の整合と、値の個数の対応を確かめる。
//
// **語彙の項目と原資料の key のちょうど一方を求める。** 両方を持つ属性は、同じ集合を
// 2 つの鍵で指す形になる。どちらも持たない属性は、分析者が何を見ているかを示せない。
func (a NodeAttribute) Validate() error {
	if (a.Semantic == "") == (a.Name == "") {
		return itemError("NodeAttribute.semantic and NodeAttribute.name", ErrInconsistentValue)
	}
	if problem := requireKnownEnumWhenPresent("NodeAttribute.semantic",
		a.Semantic); problem != nil {
		return problem
	}
	if len(a.Values) == 0 {
		return itemError("NodeAttribute.values", ErrMissingRequiredItem)
	}
	if problem := validateElements("NodeAttribute.values", a.Values); problem != nil {
		return problem
	}
	if a.ValueCount != int64(len(a.Values)) {
		return itemError("NodeAttribute.valueCount differs from the number of values",
			ErrInconsistentValue)
	}
	return nil
}
