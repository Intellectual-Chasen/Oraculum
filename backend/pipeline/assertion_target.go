package pipeline

import (
	"strconv"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// assertionRecordKey は、収集元の内容の識別と収集元の中の位置でレコードを探す鍵である。
// sourceId を材料に入れないため、取り込みをやり直した結果の中でも同じレコードに一致する。
type assertionRecordKey struct {
	contentSha256  string
	positionKind   core.PositionKind
	sequenceNumber string
	lineNumber     string
	// byteOffset は byte の範囲で位置を示す参照の byte の位置である (byteOffsetText)。
	byteOffset string
}

// AssertionResolver は分析者の所見の対象を、観測層のグラフの中で探す。
//
// **所見はグラフを書き換えない。** 本型が行うのは、所見が指す対象が現在のグラフの
// どこから出たものかを読むことだけである。
type AssertionResolver struct {
	graph Graph
	// recordKeys はグラフが根拠に持つレコードを、sourceId に依らない鍵で探す集合である。
	recordKeys map[assertionRecordKey]struct{}
	// sourceContents はグラフが根拠に持つレコードの収集元の内容の識別の集合である。
	sourceContents map[string]struct{}
}

// NewAssertionResolver はグラフの根拠のレコードを 1 回走査して、所見の対象を探す器を作る。
func NewAssertionResolver(graph Graph) AssertionResolver {
	keys := make(map[assertionRecordKey]struct{}, len(graph.records))
	contents := map[string]struct{}{}
	for _, record := range graph.records {
		keys[assertionRecordKeyOf(core.NewAssertionRecordRef(record.locator))] = struct{}{}
		contents[record.locator.SourceContentSha256] = struct{}{}
	}
	return AssertionResolver{graph: graph, recordKeys: keys, sourceContents: contents}
}

// Origin は所見の対象が何から出たかを、観測層のグラフと分析者が足した関係の集合から返す。
//
// **観測から直に出した関係、関連付けが挙げた候補、分析者が付けた所見、現在の取り込み結果に
// 無い対象を分ける。** グラフが対象を持つときはグラフの関係の状態が答えになり、グラフが
// 対象を持たないときは added が答えを決める。added も対象を持たないときの答えは absent
// である。
func (r AssertionResolver) Origin(
	assertion core.Assertion, added AddedRelations,
) core.AssertionTargetOrigin {
	if origin, found := r.originInGraph(assertion.Target); found {
		return origin
	}
	if assertion.Target.Kind == core.AssertionTargetKindEdge && assertion.Target.Edge != nil &&
		added.carries(*assertion.Target.Edge) {
		return core.AssertionTargetOriginAnalystAssertion
	}
	return core.AssertionTargetOriginAbsent
}

// ProposalOrigin は AI 提案の対象が何から出たかを、観測層のグラフから返す。
//
// **値の意味は所見の対象の出所と同じである。** グラフが対象を持たない提案は absent になる。関係を足す
// 提案の関係は採用するまでグラフに無いため absent になり、両端のノードがあるかは EndpointsInGraph が
// 答える。
func (r AssertionResolver) ProposalOrigin(proposal core.AssistProposal) core.AssertionTargetOrigin {
	if origin, found := r.originInGraph(proposal.Target); found {
		return origin
	}
	return core.AssertionTargetOriginAbsent
}

// EndpointsInGraph は、関係の参照の両端のノードが現在のグラフにあるかを返す。
func (r AssertionResolver) EndpointsInGraph(edge core.AssertionEdgeRef) bool {
	return r.graph.HasNode(edge.SourceNodeId) && r.graph.HasNode(edge.TargetNodeId)
}

// InGraph は対象が現在のグラフにあるかを返す。
func (r AssertionResolver) InGraph(target core.AssertionTarget) bool {
	_, found := r.originInGraph(target)
	return found
}

// AddedRelations は分析者が所見で足した関係の集合である。
//
// **記録した時点のグラフが持たない関係へ分析者が付けた所見 (core.Assertion.AddsRelation) を、
// 分析者が足した関係として扱う。** 記録時にグラフにあった関係への所見は、その関係が再解析で
// 消えても集合に入れない。消えた関係の出所は absent になる。
//
// **主張されている所見だけを数える。** 取り下げた所見の関係は、誰も主張していないため
// 集合に入らない。
type AddedRelations struct {
	edgeIds map[string]struct{}
}

// NewAddedRelations は所見の一覧から、分析者が足した関係の集合を組む。
func NewAddedRelations(assertions []core.Assertion) AddedRelations {
	ids := make(map[string]struct{})
	for _, assertion := range assertions {
		if assertion.State != core.AssertionStateActive || !assertion.AddsRelation ||
			assertion.Target.Edge == nil {
			continue
		}
		ids[assertionEdgeIdOf(*assertion.Target.Edge)] = struct{}{}
	}
	return AddedRelations{edgeIds: ids}
}

// carries は関係の参照が集合にあるかを返す。
func (r AddedRelations) carries(ref core.AssertionEdgeRef) bool {
	_, found := r.edgeIds[assertionEdgeIdOf(ref)]
	return found
}

// assertionEdgeIdOf は関係の参照を、グラフのエッジと同じ識別子へ直す。
func assertionEdgeIdOf(ref core.AssertionEdgeRef) string {
	return edgeIdOf(ref.Kind, ref.SourceNodeId, ref.TargetNodeId)
}

// originInGraph は対象がグラフにあるときの出所を返す。
// found が偽になるのは、現在のグラフが対象を持たないときである。
func (r AssertionResolver) originInGraph(
	target core.AssertionTarget,
) (core.AssertionTargetOrigin, bool) {
	switch target.Kind {
	case core.AssertionTargetKindNode:
		if r.graph.HasNode(target.NodeId) {
			return core.AssertionTargetOriginObservation, true
		}
	case core.AssertionTargetKindEdge:
		return r.edgeOrigin(target.Edge)
	case core.AssertionTargetKindRecord:
		if target.Record == nil {
			return "", false
		}
		if _, found := r.recordKeys[assertionRecordKeyOf(*target.Record)]; found {
			return core.AssertionTargetOriginObservation, true
		}
	case core.AssertionTargetKindSource:
		if _, found := r.sourceContents[target.SourceContentSha256]; found {
			return core.AssertionTargetOriginObservation, true
		}
	}
	return "", false
}

// edgeOrigin は関係の参照が指すエッジの出所を返す。
// 関係の状態が observed のエッジは観測から直に出たもので、candidate と uncertain_chain の
// エッジは関連付けが挙げた候補である。
func (r AssertionResolver) edgeOrigin(
	ref *core.AssertionEdgeRef,
) (core.AssertionTargetOrigin, bool) {
	if ref == nil {
		return "", false
	}
	at, found := r.graph.edgeIndexOf(assertionEdgeIdOf(*ref))
	if !found {
		return "", false
	}
	if r.graph.edges[at].state == core.RelationStateObserved {
		return core.AssertionTargetOriginObservation, true
	}
	return core.AssertionTargetOriginMatchingCandidate, true
}

// assertionRecordKeyOf はレコードの参照を探す鍵へ直す。
func assertionRecordKeyOf(ref core.AssertionRecordRef) assertionRecordKey {
	return assertionRecordKey{
		contentSha256:  ref.SourceContentSha256,
		positionKind:   ref.PositionKind,
		sequenceNumber: assertionPositionText(ref.SequenceNumber),
		lineNumber:     assertionPositionText(ref.LineNumber),
		byteOffset:     byteOffsetText(ref),
	}
}

// byteOffsetText は、byte の範囲で位置を示す参照の byte の位置を鍵の文字列へ直す。
// **他の位置の指し方では空の文字列にする。** 通番と行番号で位置を示す参照は、その主の位置で
// 1 件のレコードに決まる。byte の位置を鍵に足すと、byte の位置を持たない参照が同じレコードに
// 一致しなくなる。
func byteOffsetText(ref core.AssertionRecordRef) string {
	if ref.PositionKind != core.PositionKindByteRange {
		return ""
	}
	return assertionPositionText(ref.ByteOffset)
}

// assertionPositionText は省略可の位置を鍵の文字列へ直す。
// 値が出ていない状態を 0 と同じ文字列にしないため、出ていない状態は空の文字列になる。
func assertionPositionText(value *int64) string {
	if value == nil {
		return ""
	}
	return strconv.FormatInt(*value, 10)
}
