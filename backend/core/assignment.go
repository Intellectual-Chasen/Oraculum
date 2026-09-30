package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"unicode"
)

// TerminalAssignmentState は端末の割当の判定の状態を持つ。
type TerminalAssignmentState string

// TerminalAssignmentState の値。
const (
	// TerminalAssignmentStateDetermined は端末が 1 つに確定した状態である。
	TerminalAssignmentStateDetermined TerminalAssignmentState = "determined"
	// TerminalAssignmentStateUndetermined は端末を 1 つに確定させない状態である。
	// 割当の期間が重なるとき、時刻を比べられないとき、接続元 IP の割当が無いときに入る。
	TerminalAssignmentStateUndetermined TerminalAssignmentState = "undetermined"
	// TerminalAssignmentStateOriginOutsideAssignmentRange は起点のレコードが割当を
	// 適用してよい期間の外にある状態である。字面は EmptyReason の同じ値と揃える。
	TerminalAssignmentStateOriginOutsideAssignmentRange TerminalAssignmentState = "origin_outside_assignment_range"
)

// IsKnown は TerminalAssignmentState が定義の中の値であるかを返す。
func (s TerminalAssignmentState) IsKnown() bool {
	switch s {
	case TerminalAssignmentStateDetermined, TerminalAssignmentStateUndetermined,
		TerminalAssignmentStateOriginOutsideAssignmentRange:
		return true
	default:
		return false
	}
}

// TerminalUnresolvedReason は端末を 1 つに確定させない理由を持つ。
type TerminalUnresolvedReason string

// TerminalUnresolvedReason の値。
const (
	// TerminalUnresolvedReasonNoAssignmentForClientIp は接続元 IP の割当が 1 件も無い
	// 理由である。
	TerminalUnresolvedReasonNoAssignmentForClientIp TerminalUnresolvedReason = "no_assignment_for_client_ip"
	// TerminalUnresolvedReasonOverlappingAssignments は割当の期間が重なっている理由である。
	TerminalUnresolvedReasonOverlappingAssignments TerminalUnresolvedReason = "overlapping_assignments"
	// TerminalUnresolvedReasonTimeNotComparable は起点の時刻または割当の期間を
	// time として比べられない理由である。
	TerminalUnresolvedReasonTimeNotComparable TerminalUnresolvedReason = "time_not_comparable"
)

// IsKnown は TerminalUnresolvedReason が定義の中の値であるかを返す。
func (r TerminalUnresolvedReason) IsKnown() bool {
	switch r {
	case TerminalUnresolvedReasonNoAssignmentForClientIp,
		TerminalUnresolvedReasonOverlappingAssignments,
		TerminalUnresolvedReasonTimeNotComparable:
		return true
	default:
		return false
	}
}

// TerminalAssignmentOrigin は割当が何から出たかである。
//
// **どの由来の割当も、1 件で端末を確定させる。** 由来は、画面で分析者が割当の出どころを
// 読み分けるために持つ。
type TerminalAssignmentOrigin string

// TerminalAssignmentOrigin の値。
const (
	// TerminalAssignmentOriginObservedInSource は、収集元のレコードが接続元 IP と端末を
	// 同じレコードに記録している割当である。
	TerminalAssignmentOriginObservedInSource TerminalAssignmentOrigin = "observed_in_source"
	// TerminalAssignmentOriginImportSpecified は、利用者が取り込みの起動で収集元ごとに
	// 指定した割当である。
	TerminalAssignmentOriginImportSpecified TerminalAssignmentOrigin = "import_specified"
	// TerminalAssignmentOriginAnalystSupplied は、分析者が取り込みの後に画面から記録した
	// 割当である。
	TerminalAssignmentOriginAnalystSupplied TerminalAssignmentOrigin = "analyst_supplied"
)

// IsKnown は TerminalAssignmentOrigin が定義の中の値であるかを返す。
func (o TerminalAssignmentOrigin) IsKnown() bool {
	switch o {
	case TerminalAssignmentOriginObservedInSource, TerminalAssignmentOriginImportSpecified,
		TerminalAssignmentOriginAnalystSupplied:
		return true
	default:
		return false
	}
}

// userSupplied は、利用者が入力した割当の由来であるかを返す。
func (o TerminalAssignmentOrigin) userSupplied() bool {
	return o == TerminalAssignmentOriginImportSpecified ||
		o == TerminalAssignmentOriginAnalystSupplied
}

// TerminalAssignment は接続元 IP から端末への割当 1 件と、割当を適用してよい期間を持つ。
//
// SourceId は割当の期間を読み取った収集元を指す。**割当の根拠になったレコードは
// BasisRecordRefs が別に持つ。** 分析者が与えた割当は、期間を 1 つの収集元の収録範囲から
// 取り、根拠を別の収集元のレコードから取る。1 つの欄では 2 つを表せない。
//
// **接続元 IP、端末の外部識別子、端末の表示名は、利用者が入力した割当ではどれも省ける。**
// 1 つ以上を持つ。省いた項目は空の文字列で持ち、応答に出さない。収集元のレコードから読んだ
// 割当は 3 項目をすべて持つ。
type TerminalAssignment struct {
	// ClientIp は端末が持つ IP アドレスである。値を持たない割当は、接続元 IP から端末を
	// 求める判定 (ResolveTerminal) の対象にならない。
	ClientIp string `json:"clientIp,omitempty"`
	// TerminalId は端末の外部識別子である。語彙の terminal.id を持つ。同じ割当を 1 件に
	// まとめる鍵に入る。
	//
	// **値を持たない割当は、AppliesToSourceId の収集元を記録した端末を指す。** 端末の
	// ノードはその収集元の内容の識別で識別する (TerminalNodeKey)。
	TerminalId string `json:"terminalId,omitempty"`
	// TerminalHostname は分析者が読む端末の表示名である。語彙の terminal.hostname を
	// 持つ。**まとめる鍵に入らない。** 同じ表示名を持つ別の端末を 1 件にまとめないためである。
	TerminalHostname string `json:"terminalHostname,omitempty"`
	// TerminalHostnames は、分析者が記録した、端末が名乗るホスト名 (短い名前と FQDN) の並びで
	// ある。利用者が入力した割当だけが持つ。**まとめる鍵に入らない。**
	//
	// 表示名と合わせて、レコードが名乗ったホスト名と引数が指すホスト名を、この割当の端末と
	// 比べる材料になる (NamesHostname)。
	TerminalHostnames []string `json:"terminalHostnames,omitempty"`
	// SourceId は割当の期間を読み取った収集元の取り込み 1 件である。
	SourceId             string    `json:"sourceId"`
	SourceContentSha256  string    `json:"sourceContentSha256"`
	AssignmentValidRange TimeRange `json:"assignmentValidRange"`
	// Origin は割当が何から出たかである。
	Origin TerminalAssignmentOrigin `json:"origin"`
	// Derivation は分析者が割当を導いた筋道である。Origin が analyst_supplied のとき必須。
	// 入力された言語のまま保つ。
	Derivation string `json:"derivation,omitempty"`
	// BasisRecordRefs は分析者が根拠に挙げたレコードである。
	// Origin が analyst_supplied のとき要素数 1 以上である。
	BasisRecordRefs []AssertionRecordRef `json:"basisRecordRefs,omitempty"`
	// Author は割当を与えた分析者である。Origin が analyst_supplied のとき必須。
	Author string `json:"author,omitempty"`
	// AppliesToSourceId は、レコードの全体がこの端末のものである収集元の取り込み 1 件である。
	//
	// **自機の識別子を持たない収集元に端末を与える欄である。** Linux の監査ログは接続元 IP も
	// ホスト名も記録しないため、レコードから端末を求められない。値を持たない割当は、接続元 IP と
	// 端末の対応だけを表す。Origin が import_specified のときと、TerminalId を持たない
	// ときは、SourceId と同じ値を持つ。
	AppliesToSourceId string `json:"appliesToSourceId,omitempty"`
}

// Validate は項目の整合と、由来に対応する根拠があることを確かめる。
func (a TerminalAssignment) Validate() error {
	problem := firstProblem(
		requirePresent("TerminalAssignment.sourceId", a.SourceId),
		requireLowerHex64("TerminalAssignment.sourceContentSha256", a.SourceContentSha256),
		requireKnownEnum("TerminalAssignment.origin", a.Origin),
	)
	if problem != nil {
		return problem
	}
	if problem := a.validateTerminalItems(); problem != nil {
		return problem
	}
	if rangeProblem := a.AssignmentValidRange.Validate(); rangeProblem != nil {
		return itemError("TerminalAssignment.assignmentValidRange", rangeProblem)
	}
	// 利用者が入力した割当の期間は、分析者の時刻の解釈を持たない。解釈は収集元の所見が持ち、
	// 割当の保存先はずれを書かない。収集元のレコードから読んだ割当は、解釈で読んだ収録範囲を
	// 期間に持てる。
	if a.Origin.userSupplied() &&
		(a.AssignmentValidRange.From.Interpretation != nil || a.AssignmentValidRange.To.Interpretation != nil) {
		return itemError("TerminalAssignment.assignmentValidRange carries an analyst time interpretation",
			ErrUnexpectedItem)
	}
	return a.validateOriginItems()
}

// validateTerminalItems は接続元 IP と端末の 2 項目を確かめる。
//
// **収集元のレコードから読んだ割当は 3 項目をすべて持つ。** 利用者が入力した割当は、
// 3 項目のうち 1 つ以上を持つ。利用者が入力した接続元 IP は IP アドレスとして読める文字列に
// 限る。収集元のレコードが記録した文字列は、読めなくても原資料の値として保つ。
//
// **端末の外部識別子を持たない割当は、自身の期間を読み取った収集元に付ける。** 端末の
// ノードを収集元の内容の識別で識別するため、期間の収集元と端末を付ける収集元が同じで
// なければ、どの収集元の端末かを 1 件の割当から決められない。
func (a TerminalAssignment) validateTerminalItems() error {
	if !a.Origin.userSupplied() {
		if len(a.TerminalHostnames) != 0 {
			return itemError("TerminalAssignment.terminalHostnames", ErrUnexpectedItem)
		}
		return firstProblem(
			requirePresent("TerminalAssignment.clientIp", a.ClientIp),
			requirePresent("TerminalAssignment.terminalId", a.TerminalId),
			requirePresent("TerminalAssignment.terminalHostname", a.TerminalHostname),
		)
	}
	for index, hostname := range a.TerminalHostnames {
		item := "TerminalAssignment.terminalHostnames at " + formatIndex(index)
		if hostname == "" || strings.ContainsFunc(hostname, unicode.IsSpace) {
			return itemError(item+" is empty or carries a blank character", ErrInvalid)
		}
		if problem := requireSanitized(item, hostname); problem != nil {
			return problem
		}
	}
	if a.ClientIp == "" && a.TerminalId == "" && a.TerminalHostname == "" {
		return itemError("TerminalAssignment carries none of clientIp, terminalId and terminalHostname",
			ErrMissingRequiredItem)
	}
	for _, named := range []struct{ item, value string }{
		{"TerminalAssignment.terminalId", a.TerminalId},
		{"TerminalAssignment.terminalHostname", a.TerminalHostname},
	} {
		item, value := named.item, named.value
		if problem := requireSanitized(item, value); problem != nil {
			return problem
		}
		// 空白だけの文字列を、項目を持つ値として受け入れない。
		if value != "" && strings.TrimSpace(value) == "" {
			return itemError(item+" carries blank characters alone", ErrMissingRequiredItem)
		}
	}
	if a.ClientIp != "" {
		if _, err := netip.ParseAddr(a.ClientIp); err != nil {
			return itemError("TerminalAssignment.clientIp is not an IP address", ErrInvalid)
		}
	}
	// 接続元 IP も付ける収集元もホスト名も持たない割当は、端末の判定にもグラフにも使われない。
	// ホスト名だけを持つ割当は、同じホスト名を名乗るレコードと引数をその端末に結ぶ (NamesHostname)。
	if a.ClientIp == "" && a.AppliesToSourceId == "" &&
		a.TerminalHostname == "" && len(a.TerminalHostnames) == 0 {
		return itemError("TerminalAssignment carries none of clientIp, appliesToSourceId and a hostname",
			ErrMissingRequiredItem)
	}
	if a.TerminalId == "" && a.AppliesToSourceId != a.SourceId {
		return itemError("TerminalAssignment without terminalId applies to a source other than sourceId",
			ErrInconsistentValue)
	}
	return nil
}

// TerminalNodeKey は割当が指す端末のノードの識別鍵を返す。
// ok が偽になるのは、端末の外部識別子を持たず、端末を付ける収集元も持たない割当である。
//
// 端末の外部識別子を持つ割当は、その識別子の鍵を返す。持たない割当は、自身の期間を
// 読み取った収集元を記録した端末の鍵を返す (RecordingTerminalNodeKey)。
func (a TerminalAssignment) TerminalNodeKey() (NodeKey, bool) {
	if a.TerminalId != "" {
		return TerminalNodeKey(a.TerminalId)
	}
	if a.AppliesToSourceId == "" || a.AppliesToSourceId != a.SourceId {
		return NodeKey{}, false
	}
	return RecordingTerminalNodeKey(a.SourceContentSha256)
}

// NamesHostname は、割当が記録したホスト名 (表示名と TerminalHostnames) が hostname と同じ
// 端末の名前であるかを返す。大文字と小文字を区別しない。
//
// **割当が FQDN (`.` を含む名前) を 1 つでも持つときは、名前の全体で比べる。** 短い名前だけを
// 持つ割当は、hostname の最初の `.` より前の文字列と比べる。名前を持たない割当は偽である。
func (a TerminalAssignment) NamesHostname(hostname string) bool {
	if hostname == "" {
		return false
	}
	// 表示名を並びの先頭として読む。レコードごとに呼ばれるため、並びを組み直さない。
	nameAt := func(index int) string {
		if index == 0 {
			return a.TerminalHostname
		}
		return a.TerminalHostnames[index-1]
	}
	count := len(a.TerminalHostnames) + 1
	holdsFqdn := false
	for index := range count {
		name := nameAt(index)
		if name != "" && strings.EqualFold(name, hostname) {
			return true
		}
		holdsFqdn = holdsFqdn || strings.Contains(name, ".")
	}
	if holdsFqdn {
		return false
	}
	short, _, _ := strings.Cut(hostname, ".")
	for index := range count {
		if name := nameAt(index); name != "" && strings.EqualFold(name, short) {
			return true
		}
	}
	return false
}

// validateOriginItems は由来ごとに必要な項目を確かめる。
//
// **分析者が与えた割当は、導いた筋道と根拠のレコードを必ず持つ。** 根拠を持たない推定を
// 収集元の観測と同じ形で並べない。**取り込みの指定は、収集元 1 件に付ける。** 筋道と
// 根拠のレコードと分析者を持たない。
func (a TerminalAssignment) validateOriginItems() error {
	if a.Origin != TerminalAssignmentOriginAnalystSupplied {
		if problem := firstProblem(
			requireAbsent("TerminalAssignment.derivation", a.Derivation),
			requireAbsent("TerminalAssignment.author", a.Author),
		); problem != nil {
			return problem
		}
		if len(a.BasisRecordRefs) != 0 {
			return itemError("TerminalAssignment.basisRecordRefs", ErrUnexpectedItem)
		}
		if a.Origin == TerminalAssignmentOriginImportSpecified {
			if a.AppliesToSourceId != a.SourceId {
				return itemError("TerminalAssignment.appliesToSourceId of an import specification",
					ErrInconsistentValue)
			}
			return nil
		}
		return requireAbsent("TerminalAssignment.appliesToSourceId", a.AppliesToSourceId)
	}
	problem := firstProblem(
		requirePresent("TerminalAssignment.derivation", a.Derivation),
		requirePresent("TerminalAssignment.author", a.Author),
		requireSanitized("TerminalAssignment.author", a.Author),
	)
	if problem != nil {
		return problem
	}
	if strings.TrimSpace(a.Derivation) == "" {
		return itemError("TerminalAssignment.derivation carries blank characters alone",
			ErrMissingRequiredItem)
	}
	if len(a.BasisRecordRefs) == 0 {
		return itemError("TerminalAssignment.basisRecordRefs", ErrMissingRequiredItem)
	}
	return validateElements("TerminalAssignment.basisRecordRefs", a.BasisRecordRefs)
}

// ClockDependency は判定が比べた 2 つの時計を持つ。
type ClockDependency struct {
	LeftClock  Clock `json:"leftClock"`
	RightClock Clock `json:"rightClock"`
}

// Validate は全項目が既知の時計であることを確かめる。
func (d ClockDependency) Validate() error {
	return firstProblem(
		requireKnownEnum("ClockDependency.leftClock", d.LeftClock),
		requireKnownEnum("ClockDependency.rightClock", d.RightClock),
	)
}

// TerminalResolutionInput は端末の判定に与える 1 件のレコードと割当の集合と前提を持つ。
// Assumptions は時刻の比較が依拠する前提であり、呼び出し元が根拠区分ごとに与える。
type TerminalResolutionInput struct {
	ClientIp    string               `json:"clientIp"`
	EventTime   Timestamp            `json:"eventTime"`
	Assignments []TerminalAssignment `json:"assignments"`
	Assumptions []MatchAssumption    `json:"assumptions"`
}

// Validate は項目の整合を確かめる。
func (in TerminalResolutionInput) Validate() error {
	if problem := requirePresent("TerminalResolutionInput.clientIp", in.ClientIp); problem != nil {
		return problem
	}
	if problem := in.EventTime.Validate(); problem != nil {
		return itemError("TerminalResolutionInput.eventTime", problem)
	}
	if problem := validateElements("TerminalResolutionInput.assignments",
		in.Assignments); problem != nil {
		return problem
	}
	return validateElements("TerminalResolutionInput.assumptions", in.Assumptions)
}

// TerminalResolution は端末の判定の結果を持つ。
// Members の要素数が 2 以上のとき、判定は端末を 1 つに確定させない。
type TerminalResolution struct {
	AssignmentState TerminalAssignmentState `json:"assignmentState"`
	Members         []TerminalAssignment    `json:"members"`
	// MemberCount は端末の候補の総数である。
	MemberCount int64 `json:"memberCount"`
	// Assumptions は入力の前提と、判定が足した前提である。
	Assumptions []MatchAssumption `json:"assumptions"`
	// UnresolvedReasons は端末を 1 つに確定させない理由である。
	UnresolvedReasons []TerminalUnresolvedReason `json:"unresolvedReasons"`
	// ClockDependency は判定が 2 つの時計の時刻を比べたときに出る。
	// 2 つの時計のずれは未確定である。
	ClockDependency *ClockDependency `json:"clockDependency,omitempty"`
}

// UnmarshalJSON は全項目を復元する。memberCount を欠いた object は error を返す。
// 同項目は必須であり、欠けた総数を 0 へ復元すると、端末の候補がある判定を候補が無い
// 判定として読める形になる。
func (r *TerminalResolution) UnmarshalJSON(data []byte) error {
	type items TerminalResolution
	var decoded struct {
		items
		MemberCount *int64 `json:"memberCount"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return fmt.Errorf("decoding TerminalResolution: %w", err)
	}
	if problem := requireDecodedNumber(
		"TerminalResolution.memberCount", decoded.MemberCount); problem != nil {
		return fmt.Errorf("decoding TerminalResolution: %w", problem)
	}
	value := TerminalResolution(decoded.items)
	value.MemberCount = *decoded.MemberCount
	*r = value
	return nil
}

// Determined は端末が 1 つに確定したかを返す。
func (r TerminalResolution) Determined() bool {
	return r.AssignmentState == TerminalAssignmentStateDetermined
}

// Validate は項目の整合と、状態と要素数の対応を確かめる。
func (r TerminalResolution) Validate() error {
	if problem := requireKnownEnum("TerminalResolution.assignmentState", r.AssignmentState); problem != nil {
		return problem
	}
	if problem := validateElements("TerminalResolution.members", r.Members); problem != nil {
		return problem
	}
	if problem := validateElements("TerminalResolution.assumptions",
		r.Assumptions); problem != nil {
		return problem
	}
	for index, reason := range r.UnresolvedReasons {
		if problem := requireKnownEnum(
			"TerminalResolution.unresolvedReasons at "+formatIndex(index), reason); problem != nil {
			return problem
		}
	}
	if r.ClockDependency != nil {
		if problem := r.ClockDependency.Validate(); problem != nil {
			return itemError("TerminalResolution.clockDependency", problem)
		}
	}
	if r.MemberCount != int64(len(r.Members)) {
		return itemError("TerminalResolution.memberCount differs from the number of members",
			ErrInconsistentValue)
	}
	return r.validateStateAgainstMembers()
}

// validateStateAgainstMembers は状態と端末の候補の要素数の対応を確かめる。
func (r TerminalResolution) validateStateAgainstMembers() error {
	switch r.AssignmentState {
	case TerminalAssignmentStateDetermined:
		if !pointToOneTerminal(r.Members) {
			return itemError("TerminalResolution.members on a determined state",
				ErrInconsistentValue)
		}
		if len(r.UnresolvedReasons) > 0 {
			return itemError("TerminalResolution.unresolvedReasons", ErrUnexpectedItem)
		}
	case TerminalAssignmentStateOriginOutsideAssignmentRange:
		if len(r.Members) != 0 {
			return itemError("TerminalResolution.members", ErrUnexpectedItem)
		}
		if len(r.UnresolvedReasons) > 0 {
			return itemError("TerminalResolution.unresolvedReasons", ErrUnexpectedItem)
		}
	case TerminalAssignmentStateUndetermined:
		if len(r.UnresolvedReasons) == 0 {
			return itemError("TerminalResolution.unresolvedReasons", ErrMissingRequiredItem)
		}
	}
	return nil
}

// pointToOneTerminal は、割当が 1 件か、2 件以上のすべてが同じ端末のノードを指すかを返す。
// 端末のノードを指さない割当 (TerminalNodeKey の ok が偽) を 2 件以上含む組は偽である。
//
// 収集元ごとに同じ端末と IP を指定した取り込みは、同じ内容の割当を収集元の数だけ持つ。
func pointToOneTerminal(members []TerminalAssignment) bool {
	if len(members) == 1 {
		return true
	}
	if len(members) == 0 {
		return false
	}
	first, keyed := members[0].TerminalNodeKey()
	if !keyed {
		return false
	}
	for _, member := range members[1:] {
		key, keyed := member.TerminalNodeKey()
		if !keyed || !slices.Equal(key.DigestParts(), first.DigestParts()) {
			return false
		}
	}
	return true
}

// ResolveTerminal は 1 件のレコードの接続元 IP と時刻から端末の候補を求める。
// 引数から結果を決める純粋な判定である。
//
// 期間は両端を含む閉じた範囲として判定する。期間の端点のレコードは、期間の端点を
// 観測した収集元に現れたレコードであるためである。
// 期間の重なる割当が別の端末を指すときは端末を 1 つに確定させない。**同じ端末を指すときは
// 確定させる** (pointToOneTerminal)。どちらのときも重なった割当をすべて Members に残す。
func ResolveTerminal(in TerminalResolutionInput) (TerminalResolution, error) {
	if problem := in.Validate(); problem != nil {
		return TerminalResolution{}, itemError("resolving the terminal", problem)
	}
	assumptions := make([]MatchAssumption, len(in.Assumptions))
	copy(assumptions, in.Assumptions)
	resolution := TerminalResolution{
		AssignmentState:   TerminalAssignmentStateUndetermined,
		Assumptions:       assumptions,
		UnresolvedReasons: []TerminalUnresolvedReason{},
	}

	forClientIp := make([]TerminalAssignment, 0, len(in.Assignments))
	for _, assignment := range in.Assignments {
		if assignment.ClientIp == in.ClientIp {
			forClientIp = append(forClientIp, assignment)
		}
	}
	if len(forClientIp) == 0 {
		resolution.UnresolvedReasons = append(resolution.UnresolvedReasons,
			TerminalUnresolvedReasonNoAssignmentForClientIp)
		return resolution, nil
	}

	members := make([]TerminalAssignment, 0, len(forClientIp))
	for _, assignment := range forClientIp {
		inRange, canCompare := assignment.AssignmentValidRange.Contains(in.EventTime)
		if !canCompare {
			resolution.UnresolvedReasons = append(resolution.UnresolvedReasons,
				TerminalUnresolvedReasonTimeNotComparable)
			return resolution, nil
		}
		if inRange {
			members = append(members, assignment)
		}
	}

	resolution.ClockDependency = clockDependencyOf(in.EventTime, forClientIp)
	resolution.Members = members
	resolution.MemberCount = int64(len(members))
	switch {
	case len(members) == 0:
		resolution.AssignmentState = TerminalAssignmentStateOriginOutsideAssignmentRange
		resolution.Members = nil
	case pointToOneTerminal(members):
		// どの由来の割当も 1 件で端末を確定させる。
		resolution.AssignmentState = TerminalAssignmentStateDetermined
	default:
		resolution.UnresolvedReasons = append(resolution.UnresolvedReasons,
			TerminalUnresolvedReasonOverlappingAssignments)
	}
	return resolution, nil
}

// clockDependencyOf は判定が 2 つの時計の時刻を比べたかを返す。
// 比べた時計が 1 つのときは nil を返す。
func clockDependencyOf(eventTime Timestamp, assignments []TerminalAssignment) *ClockDependency {
	for _, assignment := range assignments {
		rangeClock := assignment.AssignmentValidRange.From.Clock
		if rangeClock != eventTime.Clock {
			return &ClockDependency{LeftClock: eventTime.Clock, RightClock: rangeClock}
		}
	}
	return nil
}
