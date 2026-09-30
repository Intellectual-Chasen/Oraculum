package core

// RelationDerivationOutcome は、1 件のレコードを起点にして関係を導いた結果の分類である。
//
// **関係を 1 本も導けなかった起点を通知せずに捨てない。** 「導いた結果、相手が 0 件であった」と
// 「導く処理をそもそも実行できなかった」を分けて数える。この 2 つを混ぜると、分析者は
// 原資料について言える事実と、本ツールが処理できなかったことを見分けられない。
type RelationDerivationOutcome string

// RelationDerivationOutcome の値。並びは KnownRelationDerivationOutcomes が持つ。
const (
	// RelationDerivationNotUsedAsOrigin は、関係を導く起点にならなかったレコードである。
	//
	// **起点にならなかったことと、起点になって結果を出していないことを分ける。** 項目を
	// 省くと 2 つが同じ形になる。
	RelationDerivationNotUsedAsOrigin RelationDerivationOutcome = "not_used_as_origin"
	// RelationDerivationMatched は候補の関係を 1 本以上導いた起点である。
	RelationDerivationMatched RelationDerivationOutcome = "matched"
	// RelationDerivationDestinationIpAbsent は接続先 IP も要求先のホスト名も比べられる形で
	// 持たない起点である。要求先の authority がホスト名である起点は、ホスト名のノードを
	// 起点にして関連付けを実行するため、この値を取らない。
	RelationDerivationDestinationIpAbsent RelationDerivationOutcome = "destination_ip_absent"
	// RelationDerivationDestinationPortAbsent は接続先 port を比べられる形で持たない
	// 起点である。
	RelationDerivationDestinationPortAbsent RelationDerivationOutcome = "destination_port_absent"
	// RelationDerivationNoCandidateRecord は、相手の側の文字列が一致するレコードが 0 件で
	// ある起点である。
	RelationDerivationNoCandidateRecord RelationDerivationOutcome = "no_candidate_record"
	// RelationDerivationCandidateItemUnreadable は、文字列が一致するレコードはあるが、その
	// すべてが取り出す語彙の項目を比べられる形で持たない起点である。
	RelationDerivationCandidateItemUnreadable RelationDerivationOutcome = "candidate_item_unreadable"
	// RelationDerivationTerminalUndetermined は、アドレスから端末への割当が 1 つに
	// 定まらない起点である。
	RelationDerivationTerminalUndetermined RelationDerivationOutcome = "terminal_undetermined"
	// RelationDerivationTerminalIdAbsent は、アドレスから端末への割当は 1 つに定まるが、
	// その割当が端末の外部識別子を持たない起点である。関連付けは端末を外部識別子で比べる。
	RelationDerivationTerminalIdAbsent RelationDerivationOutcome = "terminal_id_absent"
	// RelationDerivationProxyAddressUnknown は、候補が記録した接続先が Proxy のアドレスで
	// あり、起点の収集元 (Proxy のログ) の端末の IP アドレスが分からない起点である。
	// 候補を Proxy への接続に限れない。端末の IP は、収集元の全体に付けた端末の割当が与える。
	RelationDerivationProxyAddressUnknown RelationDerivationOutcome = "proxy_address_unknown"
	// RelationDerivationOriginItemUnreadable は、取り出す語彙の項目または時刻を比べられる形で
	// 持たない起点である。
	RelationDerivationOriginItemUnreadable RelationDerivationOutcome = "origin_item_unreadable"
	// RelationDerivationFailed は候補集合の構築が error になった起点である。
	RelationDerivationFailed RelationDerivationOutcome = "candidate_set_failed"
	// RelationDerivationNoCandidateMatchingConditions は、段階 1 の条件で既に候補が 0 件に
	// なった起点である。文字列は段階が置く EmptyReason と揃える。
	RelationDerivationNoCandidateMatchingConditions RelationDerivationOutcome = "no_candidate_matching_conditions"
	// RelationDerivationNoCandidateInWindow は、段階 1 では候補があり、時刻の一致を足して
	// 段階 2 が 0 件になった起点である。文字列は段階が置く EmptyReason と揃える。
	RelationDerivationNoCandidateInWindow RelationDerivationOutcome = "no_candidate_in_window"
	// RelationDerivationNodeUnresolved は、段階 2 の候補があるのに相手のノードまたは起点の
	// レコードをグラフから探せなかった起点と、求めた段階を候補集合が持たない起点である。
	RelationDerivationNodeUnresolved RelationDerivationOutcome = "node_unresolved"
	// RelationDerivationSourceDeclarationConflict は、収集元の宣言の組から段階の条件を
	// 組めない起点である。
	RelationDerivationSourceDeclarationConflict RelationDerivationOutcome = "source_declaration_conflict"
	// RelationDerivationNoComparedCondition は、分析者が選んだ条件のうち、この起点の
	// 収集元が候補の絞りに使えるものが 1 つも無い起点である。
	//
	// **絞る条件を持たない関連付けを実行しない。** 条件を 1 つも比べない段階は、相手の側の
	// レコードをそのまま候補に並べ、根拠の無い組を候補として示すことになる。
	RelationDerivationNoComparedCondition RelationDerivationOutcome = "no_compared_condition"
)

// KnownRelationDerivationOutcomes は契約が定める分類を、本関数が並べた順で返す。
func KnownRelationDerivationOutcomes() []RelationDerivationOutcome {
	return []RelationDerivationOutcome{
		RelationDerivationNotUsedAsOrigin,
		RelationDerivationMatched,
		RelationDerivationDestinationIpAbsent,
		RelationDerivationDestinationPortAbsent,
		RelationDerivationNoCandidateRecord,
		RelationDerivationCandidateItemUnreadable,
		RelationDerivationTerminalUndetermined,
		RelationDerivationTerminalIdAbsent,
		RelationDerivationProxyAddressUnknown,
		RelationDerivationOriginItemUnreadable,
		RelationDerivationFailed,
		RelationDerivationNoCandidateMatchingConditions,
		RelationDerivationNoCandidateInWindow,
		RelationDerivationNodeUnresolved,
		RelationDerivationSourceDeclarationConflict,
		RelationDerivationNoComparedCondition,
	}
}

// IsKnown は契約が定める値かを返す。
func (o RelationDerivationOutcome) IsKnown() bool {
	for _, known := range KnownRelationDerivationOutcomes() {
		if o == known {
			return true
		}
	}
	return false
}

// RelationDerivationBasis は、分類が何について言えることかである。
//
// **分析者が次に採る手はこの区分で違う。** 原資料について言えることなら原資料を読み、
// 入力の欄が無いなら収集する対象を増やし、宣言の欠陥ならこちらの宣言を直し、
// 本ツールが処理できなかったことなら実装を直す。
type RelationDerivationBasis string

// RelationDerivationBasis の値。
const (
	// RelationDerivationBasisSourceFact は、原資料について言える事実である。
	RelationDerivationBasisSourceFact RelationDerivationBasis = "source_fact"
	// RelationDerivationBasisItemAbsent は、レコードが取り出す語彙の項目を比べられる形で
	// 持たない状態である。原資料の欄の有無で決まる。
	RelationDerivationBasisItemAbsent RelationDerivationBasis = "input_item_absent"
	// RelationDerivationBasisSourceDeclarationDefect は、収集元の宣言の組の欠陥である。
	RelationDerivationBasisSourceDeclarationDefect RelationDerivationBasis = "source_declaration_defect"
	// RelationDerivationBasisInternalGap は、本ツールが処理できなかったことである。
	RelationDerivationBasisInternalGap RelationDerivationBasis = "internal_gap"
	// RelationDerivationBasisAnalystSelection は、分析者が選んだ関連付けの条件か、利用者が
	// 入力した端末の割当で決まる状態である。条件か割当を変えると結果が変わる。
	RelationDerivationBasisAnalystSelection RelationDerivationBasis = "analyst_selection"
	// RelationDerivationBasisNotApplicable は、区分を当てはめない状態である。
	// 起点にならなかったレコードと、関係を導けた起点が取る。
	RelationDerivationBasisNotApplicable RelationDerivationBasis = "not_applicable"
)

// KnownRelationDerivationBases は契約が定める区分を、本関数が並べた順で返す。
func KnownRelationDerivationBases() []RelationDerivationBasis {
	return []RelationDerivationBasis{
		RelationDerivationBasisSourceFact,
		RelationDerivationBasisItemAbsent,
		RelationDerivationBasisSourceDeclarationDefect,
		RelationDerivationBasisInternalGap,
		RelationDerivationBasisAnalystSelection,
		RelationDerivationBasisNotApplicable,
	}
}

// IsKnown は契約が定める値かを返す。
func (b RelationDerivationBasis) IsKnown() bool {
	for _, known := range KnownRelationDerivationBases() {
		if b == known {
			return true
		}
	}
	return false
}

// relationDerivationBases は分類ごとの区分である。
var relationDerivationBases = map[RelationDerivationOutcome]RelationDerivationBasis{
	RelationDerivationNotUsedAsOrigin:               RelationDerivationBasisNotApplicable,
	RelationDerivationMatched:                       RelationDerivationBasisNotApplicable,
	RelationDerivationDestinationIpAbsent:           RelationDerivationBasisItemAbsent,
	RelationDerivationDestinationPortAbsent:         RelationDerivationBasisItemAbsent,
	RelationDerivationOriginItemUnreadable:          RelationDerivationBasisItemAbsent,
	RelationDerivationCandidateItemUnreadable:       RelationDerivationBasisItemAbsent,
	RelationDerivationNoCandidateRecord:             RelationDerivationBasisSourceFact,
	RelationDerivationNoCandidateMatchingConditions: RelationDerivationBasisSourceFact,
	RelationDerivationNoCandidateInWindow:           RelationDerivationBasisSourceFact,
	RelationDerivationTerminalUndetermined:          RelationDerivationBasisSourceFact,
	RelationDerivationSourceDeclarationConflict:     RelationDerivationBasisSourceDeclarationDefect,
	RelationDerivationNoComparedCondition:           RelationDerivationBasisAnalystSelection,
	// 外部識別子を省けるのは利用者が入力した割当だけであり、入力に識別子を足すと結果が変わる。
	RelationDerivationTerminalIdAbsent: RelationDerivationBasisAnalystSelection,
	// Proxy の収集元に端末の IP を付けた割当を足すと結果が変わる。
	RelationDerivationProxyAddressUnknown: RelationDerivationBasisAnalystSelection,
	RelationDerivationFailed:              RelationDerivationBasisInternalGap,
	RelationDerivationNodeUnresolved:      RelationDerivationBasisInternalGap,
}

// BasisOf は分類の区分を返す。契約の外の分類では not_applicable を返さず、
// 空の値を返す。呼び出し元は分類を先に検査する。
func (o RelationDerivationOutcome) BasisOf() RelationDerivationBasis {
	return relationDerivationBases[o]
}

// RelationDerivation は、レコード 1 件を起点にして関係を導いた結果である。
//
// **レコードの種別のノードだけが持つ。** 他の種別のノードは起点にならない。
type RelationDerivation struct {
	// Outcome は導いた結果の分類である。
	Outcome RelationDerivationOutcome `json:"outcome"`
	// Basis は分類が何について言えることかである。
	Basis RelationDerivationBasis `json:"basis"`
}

// NewRelationDerivation は分類から、区分を添えた結果を作る。
func NewRelationDerivation(outcome RelationDerivationOutcome) (RelationDerivation, error) {
	if err := requireKnownEnum("RelationDerivation.outcome", outcome); err != nil {
		return RelationDerivation{}, err
	}
	return RelationDerivation{Outcome: outcome, Basis: outcome.BasisOf()}, nil
}

// Validate は項目の整合を確かめる。
func (d RelationDerivation) Validate() error {
	if err := requireKnownEnum("RelationDerivation.outcome", d.Outcome); err != nil {
		return err
	}
	if err := requireKnownEnum("RelationDerivation.basis", d.Basis); err != nil {
		return err
	}
	if d.Basis != d.Outcome.BasisOf() {
		return itemError("RelationDerivation.basis", ErrInconsistentValue)
	}
	return nil
}
