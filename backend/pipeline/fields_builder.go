package pipeline

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// ErrRecordWithoutSemantics は、取り込みの段階 1 が意味付けを行わなかったレコードを指す。
//
// 収集元に実在し、原文と位置と収集元の識別を持つレコードである。Build はこのレコードに
// 対して本 error を包んだ error を返す。呼び出し元は errors.Is で他の失敗と分けられる。
var ErrRecordWithoutSemantics = errors.New("the record carries no semantics")

// 応答の fields に入る name のうち、段階 2 が補うもの。
const (
	clientTerminalFieldName     = "clientTerminal"
	clientTerminalNameFieldName = "clientTerminalName"
	clientIpFieldName           = "clientIp"
	clientPortFieldName         = "clientPort"
	processFieldName            = "process"
)

// responseField は段階 2 が補う 1 項目の name と、その項目が持つ語彙の項目である。
type responseField struct {
	// name は応答の fields に出る欄の名前である。
	name string
	// semantic は補う項目に載せる語彙の項目であり、補うかどうかの判定にも使う。
	//
	// **値の有無で意味の有無を変えない。** 同じ name が値を持つレコードで語彙の項目を
	// 持ち、値を持たないレコードで持たない状態を作らない。
	semantic core.SemanticKey
}

// supplementedFields は、接続を記録したレコードに段階 2 が補う接続元と実行主体の項目である。
//
// **補うかどうかは語彙の項目で決める。** レコードが同じ意味の項目を持っていれば補わない。
// 入力形式ごとの name の一覧を持つと、入力形式を足すたびに取り込みの実行を触ることに
// なる。
//
// 並びが応答の fields の末尾の並びになる。clientIp を先頭に置くのは、端末を導く段階が
// 接続元 IP の項目を読むためである。
var supplementedFields = []responseField{
	{name: clientIpFieldName, semantic: core.SemanticKeyConnectionSourceAddress},
	{name: clientTerminalFieldName, semantic: core.SemanticKeyTerminalId},
	{name: clientTerminalNameFieldName, semantic: core.SemanticKeyTerminalHostname},
	{name: clientPortFieldName, semantic: core.SemanticKeyConnectionSourcePort},
	{name: processFieldName, semantic: core.SemanticKeyProcessId},
}

// FieldsBuilder は取り込み結果から、応答の fields を 1 件ずつ組む段階 2 である。
//
// 取り込み結果の走査は NewFieldsBuilder が 1 回行う。要求ごとに走査する形は採らない
// (CandidateIndex と同じ)。
type FieldsBuilder struct {
	// byCase は案件ごとの割当である。案件を区別しない取り込みでは空の文字列の鍵 1 つを持つ。
	//
	// **レコードの端末は、そのレコードの案件の割当だけから導く。** 別の案件の収集元が記録した
	// IP と端末の対応を当てると、別の期間の端末がレコードに付く。
	byCase map[string]caseAssignments
	// caseOfSource は収集元の sourceId から、収集元に付けた案件を探す表である。
	caseOfSource map[string]string
}

// caseAssignments は案件 1 つの割当と、割当を読み取った収集元である。
type caseAssignments struct {
	// assignments は IP から端末への割当と、鍵ごとに観測した端末の表示名である。
	assignments terminalAssignments
	// sourceIds は割当を読み取った収集元である。導出できなかった理由が持つ
	// 「参照した収集元の sourceId」になる。
	sourceIds []string
}

// NewFieldsBuilder は取り込み結果から、応答の fields を組む器を作る。
// 取り込み結果を案件ごとに 1 回走査して IP から端末への割当を組む。
func NewFieldsBuilder(result ImportResult) *FieldsBuilder {
	builder := &FieldsBuilder{
		byCase: make(map[string]caseAssignments), caseOfSource: make(map[string]string),
	}
	for sourceId := range result.identities {
		builder.caseOfSource[sourceId] = result.caseOfSource(sourceId)
	}
	for _, scope := range result.caseScopes() {
		scoped := caseAssignments{assignments: terminalAssignmentsOf(scope)}
		for _, assignment := range scoped.assignments.entries {
			if !contains(scoped.sourceIds, assignment.SourceId) {
				scoped.sourceIds = append(scoped.sourceIds, assignment.SourceId)
			}
		}
		caseId := ""
		if len(scope.publications) > 0 {
			caseId = result.caseOfSource(scope.publications[0].status.SourceId)
		}
		builder.byCase[caseId] = scoped
	}
	return builder
}

// scopeOf はレコードの収集元の案件の割当を返す。
func (b *FieldsBuilder) scopeOf(entry RecordEntry) caseAssignments {
	return b.byCase[b.caseOfSource[entry.Locator.SourceId]]
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

// Build は 1 件のレコードから、応答の fields を返す。
//
// **name の集合は、そのレコードが持つ項目が決める。** 段階 1 の結果 (adapter が返す
// その入力形式の key と、同じレコードの中の文字列から binding が導いた項目) をその並びの
// まま持ち、段階 2 が補う項目を末尾に足す。
//
// 意味付けを持たないレコードは error になる。段階 1 の結果が無い状態では、
// 要素数 1 以上の fields を組む材料が無い。
func (b *FieldsBuilder) Build(entry RecordEntry) ([]core.RecordField, error) {
	if entry.Semantics == nil {
		return nil, fmt.Errorf("building response fields: %w", ErrRecordWithoutSemantics)
	}
	fields := make([]core.RecordField, 0, len(entry.Semantics.Fields)+len(supplementedFields))
	for _, field := range entry.Semantics.Fields {
		fields = append(fields, withRecordInterpretation(cloneRecordField(field), entry.ObservedAt))
	}
	// 接続を記録していないレコードは接続元も実行主体も持たないため、補う項目が無い。
	if entry.Semantics.Endpoint == nil {
		return fields, nil
	}
	// 判定はレコード自身が持つ項目に対して行う。補った項目を数え直すと、端末の識別子を
	// 補った時点で表示名の判定が「既に持っている」に変わり、表示名が応答から消える。
	for _, supplement := range supplementedFields {
		if carriesSupplement(entry.Semantics.Fields, supplement.semantic) {
			continue
		}
		field, err := b.supplementedField(entry, fields, supplement)
		if err != nil {
			return nil, fmt.Errorf("building the response field %q: %w", supplement.name, err)
		}
		fields = append(fields, field)
	}
	return fields, nil
}

// carriesSupplement は、段階 2 が補う項目をレコードが既に持っているかを返す。
//
// **端末の 2 項目は識別子と表示名のどちらかが出れば補わない。** レコードが端末の
// 識別子を書いている状態で、別の収集元の割当から表示名だけを導くと、レコードが書いた
// 端末と割当が指す端末が別でも応答が食い違いを持たない。
func carriesSupplement(fields []core.RecordField, semantic core.SemanticKey) bool {
	if isTerminalSemantic(semantic) {
		return len(fieldsWithSemantic(fields, core.SemanticKeyTerminalId)) > 0 ||
			len(fieldsWithSemantic(fields, core.SemanticKeyTerminalHostname)) > 0
	}
	return len(fieldsWithSemantic(fields, semantic)) > 0
}

// isTerminalSemantic は、語彙の項目が別の収集元との突き合わせから導く端末の項目かを返す。
func isTerminalSemantic(semantic core.SemanticKey) bool {
	return semantic == core.SemanticKeyTerminalId ||
		semantic == core.SemanticKeyTerminalHostname
}

// supplementedField は段階 2 が補う 1 項目を組む。
//
// 補う要素は 2 つに分かれる。端末の 2 項目は別の収集元との突き合わせから導く要素で、
// 残りは入力形式に欄そのものが無い要素である。
func (b *FieldsBuilder) supplementedField(
	entry RecordEntry, fields []core.RecordField, wanted responseField,
) (core.RecordField, error) {
	if isTerminalSemantic(wanted.semantic) {
		return b.derivedTerminalField(entry, fields, wanted)
	}
	field, err := core.NewTextField(wanted.name, wanted.semantic, core.NewAbsentItemValue())
	if err != nil {
		return core.RecordField{}, fmt.Errorf("building the item_absent field %q: %w",
			wanted.name, err)
	}
	return field, nil
}

// derivedTerminalField は接続元 IP から端末を導いた 1 項目を返す。
//
// clientTerminal は端末の外部識別子 (terminal.id)、clientTerminalName は分析者の読む
// 表示名 (terminal.hostname) を持つ。**2 項目は同じ割当 1 件から出る。** 識別子を
// 導けなかったレコードでは表示名も同じ理由の derivation_undetermined になり、識別子
// 無しで名前だけを出さない。
//
// **逆は成り立たない。** 同じ端末に別の表示名が付いた原資料では、識別子が derived の
// まま表示名だけが derivation_undetermined になる (terminalValueOf)。
func (b *FieldsBuilder) derivedTerminalField(
	entry RecordEntry, fields []core.RecordField, wanted responseField,
) (core.RecordField, error) {
	value, err := b.clientTerminalValue(entry, fields, wanted.semantic)
	if err != nil {
		return core.RecordField{}, fmt.Errorf("deriving the %s value: %w", wanted.name, err)
	}
	field, err := core.NewTextField(wanted.name, wanted.semantic, value)
	if err != nil {
		return core.RecordField{}, fmt.Errorf("building the %s field: %w", wanted.name, err)
	}
	return field, nil
}

// clientTerminalValue は IP から端末への割当を適用した値を返す。
//
// semantic は割当のどの項目を正規化値に置くかを決める。terminal.id は
// core.TerminalAssignment.TerminalId、terminal.hostname は同 TerminalHostname である。
//
// 判定は core.ResolveTerminal が行い、本関数は結果を core.ValueState と
// 導出できなかった理由へ写す。
//
// **突き合わせる値を読めないレコードは、割当を探す段階 1 の前で分かれる。**
func (b *FieldsBuilder) clientTerminalValue(
	entry RecordEntry, fields []core.RecordField, semantic core.SemanticKey,
) (core.RawAndNormalized, error) {
	matchKeyName, matchKeyValue := clientAddress(fields)
	clientIp, readable := matchKeyValue.ComparableValue()
	if !readable {
		return unreadableMatchKey(matchKeyName, matchKeyValue.ValueState)
	}
	key := matchKey{name: matchKeyName, comparable: clientIp}
	scope := b.scopeOf(entry)
	forClientIp := scope.assignmentsFor(clientIp)
	if len(forClientIp) == 0 {
		return undeterminedTerminal(terminalReasonNoAssignment, derivationItems{
			matchKey: key, assignmentLookup: true,
			referencedSourceIds: scope.sourceIds,
		})
	}
	if entry.ObservedAt == nil {
		return undeterminedTerminal(terminalReasonTimeNotComparable, derivationItems{
			matchKey: key, assignmentLookup: true, assignments: forClientIp,
			notComparable: missingEventTimeText,
		})
	}
	resolution, err := core.ResolveTerminal(core.TerminalResolutionInput{
		ClientIp: clientIp, EventTime: *entry.ObservedAt, Assignments: forClientIp,
	})
	if err != nil {
		return core.RawAndNormalized{}, fmt.Errorf("resolving the terminal of %q: %w", clientIp, err)
	}
	return scope.terminalValueOf(resolution, semantic, key, *entry.ObservedAt, forClientIp)
}

// matchKey は割当を探す鍵にした項目の name と、突き合わせに使った値である。
//
// name は、そのレコードが実際に持っている項目の name である。derivation は
// 「入力に使った同じレコードの項目の name と、その値」を必ず含む。
// Squid combined の接続元 IP の欄は clientIp、markii 形式の接続のレコードは srcIP を書く。
type matchKey struct {
	name       string
	comparable string
}

// text は derivation に載せる「項目の name と、その値」の組を返す。
func (k matchKey) text() string {
	return derivationValue(k.name) + derivationLabelSeparator + derivationValue(k.comparable)
}

// clientAddress は割当を探す鍵になる接続元 IP の項目の name と値を返す。
//
// **入力形式ごとの key の文字列で探さない** (語彙の接続元 IP アドレスで探す)。返す name は
// 導出できなかった理由が持つ「読めなかった項目の name」になる。
//
// 呼ぶのは段階 2 が接続元 IP の項目を補った後だけであり、その意味を持つ項目が 1 件はある。
// 既定の組は、その項目が値を持つ形になっていないレコードで clientIpFieldName を返す。
func clientAddress(fields []core.RecordField) (string, core.RawAndNormalized) {
	for _, field := range fields {
		if field.Semantic == core.SemanticKeyConnectionSourceAddress && field.Text != nil {
			return field.Name, *field.Text
		}
	}
	return clientIpFieldName, core.NewAbsentItemValue()
}

// assignmentsFor は接続元 IP の文字列に対応する割当を返す。
func (b caseAssignments) assignmentsFor(clientIp string) []core.TerminalAssignment {
	matched := make([]core.TerminalAssignment, 0, len(b.assignments.entries))
	for _, assignment := range b.assignments.entries {
		if assignment.ClientIp == clientIp {
			matched = append(matched, assignment)
		}
	}
	return matched
}

// terminalValueOf は判定の結果を core.ValueState へ写す。
// semantic は確定した割当のどの項目を正規化値に置くかを決める。
//
// **端末が 1 つに確定しても、表示名が 1 つに確定するとは限らない。** 同じ端末に別の
// 表示名が付いた原資料では、terminal.hostname だけが導出できなかった状態になる。
// 2 項目が別の状態を取れることが、識別子と表示名を分けて持つ意味である。
func (b caseAssignments) terminalValueOf(
	resolution core.TerminalResolution, semantic core.SemanticKey, key matchKey,
	eventTime core.Timestamp, forClientIp []core.TerminalAssignment,
) (core.RawAndNormalized, error) {
	clientIp := key.comparable
	items := derivationItems{
		matchKey: key, assignmentLookup: true,
		assignments: forClientIp, eventTime: &eventTime,
	}
	switch {
	case resolution.Determined():
		// 同じ端末を指す割当が 2 件以上あるときは、すべてを derivation に残す。
		items.assignments = resolution.Members
		if _, known := terminalItemOf(resolution.Members[0], semantic); !known {
			return core.RawAndNormalized{}, fmt.Errorf(
				"building the derived terminal of %q: the semantic %q has no item in the assignment",
				clientIp, semantic)
		}
		values := suppliedTerminalItems(resolution.Members, semantic)
		// 利用者が入力した割当は、端末の識別子と表示名を省ける。省いた項目を空の文字列で
		// 確定させない。
		if len(values) == 0 {
			return undeterminedTerminal(terminalReasonItemNotSupplied, items)
		}
		normalized := values[0]
		// 原資料が 1 つの端末に別の表示名を付けた組では、表示名を 1 つに選ばない。
		// 先に走査した表示名だけを derived で返すと、残りの表示名が応答から消える。
		if semantic == core.SemanticKeyTerminalHostname {
			observed := values
			for _, member := range resolution.Members {
				for _, hostname := range b.assignments.hostnamesOf(member) {
					if !slices.Contains(observed, hostname) {
						observed = append(observed, hostname)
					}
				}
			}
			if len(observed) > 1 {
				count := int64(len(observed))
				items.hostnameCount = &count
				// **どの表示名が観測されたかを応答が持つ。** 数だけを持つと、分析者は
				// 原資料へ戻って数え直すことになる。
				//
				// 既知の制限: 表示名の並びに上限と続きを与えず、観測したすべてを載せる,
				// 1 つの鍵 (tmid, ip) は端末 1 台を指し、表示名は端末の名前の変更の回数だけ増える,
				// 1 鍵あたりの異なる表示名が 1 件を超える収集元を扱うとき
				items.hostnames = observed
				return undeterminedTerminal(terminalReasonMultipleHostnames, items)
			}
		}
		value, err := core.NewDerivedValue(normalized, derivedTerminalDerivation(items))
		if err != nil {
			return core.RawAndNormalized{}, fmt.Errorf("building the derived terminal of %q: %w",
				clientIp, err)
		}
		return value, nil
	case resolution.AssignmentState == core.TerminalAssignmentStateOriginOutsideAssignmentRange:
		return undeterminedTerminal(terminalReasonOutsideRange, items)
	case containsReason(resolution.UnresolvedReasons, core.TerminalUnresolvedReasonTimeNotComparable):
		items.notComparable = notComparableText(eventTime, forClientIp)
		return undeterminedTerminal(terminalReasonTimeNotComparable, items)
	case containsReason(resolution.UnresolvedReasons, core.TerminalUnresolvedReasonOverlappingAssignments):
		items.assignments = resolution.Members
		items.memberCount = &resolution.MemberCount
		return undeterminedTerminal(terminalReasonMultipleMatches, items)
	default:
		return core.RawAndNormalized{}, fmt.Errorf(
			"resolving the terminal of %q: the state %q carries no reason this layer maps",
			clientIp, resolution.AssignmentState)
	}
}

// terminalItemOf は語彙の項目が指す割当の値を返す。
// ok が偽になるのは、割当が持たない意味を渡したときである。
func terminalItemOf(assignment core.TerminalAssignment, semantic core.SemanticKey) (string, bool) {
	switch semantic {
	case core.SemanticKeyTerminalId:
		return assignment.TerminalId, true
	case core.SemanticKeyTerminalHostname:
		return assignment.TerminalHostname, true
	default:
		return "", false
	}
}

// suppliedTerminalItems は、割当が持つ語彙の項目の値のうち空でないものを、重ねずに割当の順で返す。
func suppliedTerminalItems(members []core.TerminalAssignment, semantic core.SemanticKey) []string {
	var values []string
	for _, member := range members {
		value, _ := terminalItemOf(member, semantic)
		if value != "" && !slices.Contains(values, value) {
			values = append(values, value)
		}
	}
	return values
}

func containsReason(reasons []core.TerminalUnresolvedReason, want core.TerminalUnresolvedReason) bool {
	for _, reason := range reasons {
		if reason == want {
			return true
		}
	}
	return false
}

// unreadableMatchKey は、突き合わせる値を読めなかったレコードの項目を組む。
//
// 併せて書くものは理由、突き合わせた項目の name、その値の状態、適用した規則である。
// **参照した収集元の sourceId を書かない。**
func unreadableMatchKey(name string, state core.ValueState) (core.RawAndNormalized, error) {
	derivation := strings.Join([]string{
		labelReason + terminalReasonKeyUnreadable,
		labelMatchKey + derivationValue(name),
		labelMatchKeyState + string(state),
		labelRule + terminalAssignmentRuleName,
	}, derivationItemSeparator)
	value, err := core.NewDerivationUndeterminedValue(derivation)
	if err != nil {
		return core.RawAndNormalized{}, fmt.Errorf("building the unreadable %s match key: %w",
			name, err)
	}
	return value, nil
}

// undeterminedTerminal は導出できなかった理由を持つ項目を組む。
func undeterminedTerminal(reason string, items derivationItems) (core.RawAndNormalized, error) {
	value, err := core.NewDerivationUndeterminedValue(undeterminedTerminalDerivation(reason, items))
	if err != nil {
		return core.RawAndNormalized{}, fmt.Errorf("building the undetermined terminal: %w", err)
	}
	return value, nil
}

// derivation_undetermined の理由の文字列。
//
// **terminalReasonMultipleMatches と terminalReasonMultipleHostnames を同じ理由に
// しない。** 前者は段階 3 で残った割当が 2 件以上ある状態、後者は端末が 1 件に確定した上で
// 収集元が付けた表示名が 2 件以上ある状態である。同じ文字列を返すと、端末が 1 件に確定して
// いる応答を曖昧だと読ませる。
//
// 分析者が画面で読む値であるため日本語で書く。
const (
	terminalReasonKeyUnreadable     = "突き合わせる値を読めない"
	terminalReasonNoAssignment      = "該当が 0 件"
	terminalReasonTimeNotComparable = "時刻を比べられない"
	terminalReasonOutsideRange      = "割当の期間の外"
	terminalReasonMultipleMatches   = "該当が複数件"
	terminalReasonMultipleHostnames = "表示名が複数件"
	terminalReasonItemNotSupplied   = "該当の割当はこの項目を持たない"
)

// terminalAssignmentRuleName は突き合わせに使った規則の名前である。derivation は
// この名前を必ず含む。
const terminalAssignmentRuleName = "terminal_ip_assignment"

// derivation が値を並べるときの区切りと、項目の名前。
const (
	derivationItemSeparator  = "; "
	derivationLabelSeparator = "="
	derivationRangeJoiner    = "/"
	derivationListJoiner     = ","
	labelReason              = "reason="
	labelRule                = "rule="
	labelSourceId            = "sourceId="
	labelValidRange          = "assignmentValidRange="
	labelOrigin              = "origin="
	labelAuthor              = "author="
	labelEventTime           = "eventTime="
	// labelMemberCount は段階 3 で残った割当の数である。
	// **表示名の数を本 label に書かない。**
	labelMemberCount = "memberCount="
	// labelHostnameCount は 1 つの端末に対して収集元が付けた表示名の数である。
	labelHostnameCount = "hostnameCount="
	// labelHostnames は同じ端末に対して収集元が付けた表示名そのものである。
	labelHostnames         = "hostnames="
	labelNotComparable     = "notComparable="
	labelReferencedSources = "referencedSourceIds="
	labelMatchKey          = "matchKeyItem="
	labelMatchKeyState     = "matchKeyValueState="
)

// missingEventTimeText はレコードが時刻を持たないときに書く文字列である。
const missingEventTimeText = "レコードが時刻を持つ項目を持っていない"

// derivationEscapedBytes は値の中で backslash を前に置く byte である。
// 組・label・並びの 3 つの区切りと、backslash 自身からなる。
const derivationEscapedBytes = `\;=,`

// derivationValue は derivation の 1 つの値を、区切りで壊れない文字列にする。
//
// derivation は組を区切りで並べた 1 本の文字列であり、原資料の文字列をそのまま置くと、
// 区切りを含む値が実在しない組を作れる。分析者は derivation を根拠の説明として読むため、
// 偽の組は、行われていない突き合わせを行われたと読ませる。
//
// **label の区切りも escape する。** 組の区切りだけを escape すると、区切りを含む値が
// 素朴に分けた読み手に対して `label=値` の形を保ったまま現れる。
//
// 区切りを含まない値は原資料の文字列のまま返す。escape の対象はすべて ASCII であり、
// UTF-8 の後続 byte は 0x80 以上であるため、byte で走査しても字を割らない。
func derivationValue(text string) string {
	if !strings.ContainsAny(text, derivationEscapedBytes) {
		return text
	}
	var escaped strings.Builder
	escaped.Grow(len(text) + len(text)/4)
	for index := 0; index < len(text); index++ {
		if strings.IndexByte(derivationEscapedBytes, text[index]) >= 0 {
			escaped.WriteByte('\\')
		}
		escaped.WriteByte(text[index])
	}
	return escaped.String()
}

// derivationItems は derivation に並べる値である。
//
// assignmentLookup は、突き合わせる値に対応する割当を探す段階 1 を実行したかである。
// 実行した段階だけが referencedSourceIds を書ける。
type derivationItems struct {
	// matchKey は割当を探す鍵にした項目の name と、その値である。
	matchKey    matchKey
	assignments []core.TerminalAssignment
	eventTime   *core.Timestamp
	// memberCount は段階 3 で残った割当の数である。
	memberCount *int64
	// hostnameCount は割当の鍵 1 つ (terminalAssignmentKey) に対して収集元が付けた
	// 表示名の数である。
	hostnameCount *int64
	// hostnames は同じ鍵に対して収集元が付けた表示名そのものを、観測した順で持つ。
	hostnames []string
	// notComparable は notComparableText が組み立てた文字列である。**他の field と違い、
	// 葉を escape 済みの複合を持つ。** 生の値を入れると二重に escape する。
	notComparable       string
	assignmentLookup    bool
	referencedSourceIds []string
}

// derivedTerminalDerivation は導出をたどる要素を並べた文字列を返す。
func derivedTerminalDerivation(items derivationItems) string {
	return strings.Join(append(
		[]string{items.matchKey.text(), labelRule + terminalAssignmentRuleName},
		assignmentTexts(items.assignments)...), derivationItemSeparator)
}

// undeterminedTerminalDerivation は導出できなかった理由と、併せて書くものを並べる。
func undeterminedTerminalDerivation(reason string, items derivationItems) string {
	parts := []string{
		labelReason + reason,
		items.matchKey.text(),
		labelRule + terminalAssignmentRuleName,
	}
	if len(items.assignments) > 0 {
		parts = append(parts, assignmentTexts(items.assignments)...)
	}
	// **割当を探した段階だけが参照した収集元を書く。** 探していない状態で 0 件と書くと、
	// 割当が 1 件も無かった事実を応答が持つ。
	if items.assignmentLookup && len(items.assignments) == 0 {
		parts = append(parts, labelReferencedSources+sourceIdListText(items.referencedSourceIds))
	}
	if items.eventTime != nil {
		parts = append(parts, labelEventTime+derivationValue(eventTimeText(*items.eventTime)))
	}
	if items.memberCount != nil {
		parts = append(parts, labelMemberCount+strconv.FormatInt(*items.memberCount, 10))
	}
	if items.hostnameCount != nil {
		parts = append(parts, labelHostnameCount+strconv.FormatInt(*items.hostnameCount, 10))
	}
	if len(items.hostnames) > 0 {
		parts = append(parts, labelHostnames+derivationValueListText(items.hostnames))
	}
	if items.notComparable != "" {
		parts = append(parts, labelNotComparable+items.notComparable)
	}
	return strings.Join(parts, derivationItemSeparator)
}

// assignmentTexts は割当 1 件ごとに、読み取った収集元と適用期間の両端を並べる。
//
// **期間の両端を秒へ書き換えない。** 割当の期間の判定が比べる単位のまま載せる。
//
// 両端を分ける derivationRangeJoiner を escape の対象にしないのは、timestampText が
// 正規化値を先に返し、Timestamp の検証が正規化値を time の書式に限るためである
// (core/timestamp.go の validateNormalizedText)。両端の文字列は区切りを含まない。
//
// **利用者が入力した割当は由来を書く。** 期間を読み取った収集元だけを書くと、その収集元が
// 接続元 IP と端末を観測したと読める。画面から記録した割当は
// 記録した分析者も書く。収集元のレコードが記録した割当は由来を書かない。
func assignmentTexts(assignments []core.TerminalAssignment) []string {
	texts := make([]string, 0, len(assignments)*3)
	for _, assignment := range assignments {
		texts = append(texts, labelSourceId+derivationValue(assignment.SourceId),
			labelValidRange+derivationValue(timestampText(assignment.AssignmentValidRange.From))+
				derivationRangeJoiner+
				derivationValue(timestampText(assignment.AssignmentValidRange.To)))
		if assignment.Origin == core.TerminalAssignmentOriginObservedInSource {
			continue
		}
		texts = append(texts, labelOrigin+derivationValue(string(assignment.Origin)))
		if assignment.Author != "" {
			texts = append(texts, labelAuthor+derivationValue(assignment.Author))
		}
	}
	return texts
}

// sourceIdListText は参照した収集元を並べる。0 件のときは件数を書く。
func sourceIdListText(sourceIds []string) string {
	if len(sourceIds) == 0 {
		return "0 件"
	}
	return derivationValueListText(sourceIds)
}

// derivationValueListText は値の並びを、1 つずつ区切りで壊れない文字列にしてから並べる。
func derivationValueListText(values []string) string {
	texts := make([]string, 0, len(values))
	for _, value := range values {
		texts = append(texts, derivationValue(value))
	}
	return strings.Join(texts, derivationListJoiner)
}

// timestampText は割当の適用期間の端を derivation に載せる文字列へ直す。
// 正規化値を持つ時刻は正規化値、原資料の文字列だけを持つ時刻は原資料の文字列を返す。
func timestampText(value core.Timestamp) string {
	if normalized, ok := value.NormalizedValue(); ok {
		return normalized
	}
	if rawText, ok := value.RawTextValue(); ok {
		return rawText
	}
	return string(value.ValueState)
}

// eventTimeText はレコードの時刻を derivation に載せる文字列へ直す。
//
// **原資料の文字列を載せる。** 導出できなかった理由には、レコードの時刻の rawText を
// 併せて書く。
func eventTimeText(value core.Timestamp) string {
	if rawText, ok := value.RawTextValue(); ok {
		return rawText
	}
	if normalized, ok := value.NormalizedValue(); ok {
		return normalized
	}
	return string(value.ValueState)
}

// notComparableText は時刻を比べられない側と、その理由を返す。
func notComparableText(eventTime core.Timestamp, assignments []core.TerminalAssignment) string {
	sides := make([]string, 0, len(assignments)*2+1)
	if _, ok := eventTime.Instant(); !ok {
		sides = append(sides, labelEventTime+derivationValue(eventTimeText(eventTime))+
			" (normalizedForm="+string(eventTime.NormalizedForm)+
			" offsetState="+string(eventTime.OffsetState)+")")
	}
	for _, assignment := range assignments {
		sides = append(sides,
			rangeEndNotComparable("from", assignment.AssignmentValidRange.From)...)
		sides = append(sides,
			rangeEndNotComparable("to", assignment.AssignmentValidRange.To)...)
	}
	return strings.Join(sides, derivationListJoiner)
}

// rangeEndNotComparable は期間の端を time として比べられないときに、その端の文字列を返す。
func rangeEndNotComparable(end string, value core.Timestamp) []string {
	if _, ok := value.Instant(); ok {
		return nil
	}
	return []string{labelValidRange + end + "=" + derivationValue(timestampText(value)) +
		" (normalizedForm=" + string(value.NormalizedForm) +
		" offsetState=" + string(value.OffsetState) + ")"}
}
