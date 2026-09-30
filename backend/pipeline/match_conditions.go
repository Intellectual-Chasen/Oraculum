package pipeline

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// candidateConditionSpecs は関連付けの段階 (core.MatchRequest) が持つ条件の宣言を、起点の側の収集元と候補の側の
// 収集元の宣言から組む。
//
// **どの条件を段階が持つかを core と本 package が決めない。** 条件の一覧と、両側が比べる
// 語彙の項目は入力形式の組で変わるため、入力形式を読む adapter の binding が宣言する
// (ParserIdentity.ConnectionMatchConditions)。
//
// 条件の一覧と並びは起点の側の宣言が決める。相手の語彙の項目は、候補の側の収集元が同じ
// 条件へ挙げた項目を宣言の順に集めた集合である。候補の側の収集元が 1 つも無い段階では、
// 起点の側が挙げた項目を相手の項目に置く。
//
// **絞りに用いる条件は分析者の選択が決める。** 収集元の宣言が「候補を絞れる」と挙げた
// 条件のうち、選んだものだけが use に used を取る。選ばなかった条件は一覧から消えず、
// use が not_used の要素として残る。段階ごとの use は core が段階の種別と相手の欄から決める。
//
// **error を返すのは、収集元の宣言の組から段階の条件を組めないときである。** 絞りに用いる
// 条件へ候補の側の 2 つの収集元が別の語彙の項目を挙げた組が該当する。宣言 1 件ずつの検査は
// 収集元を開く前に済んでおり (ParserIdentity.Validate)、和集合の食い違いだけが残る。
// **起点のレコードについての分類に混ぜない。** 呼び出し元は宣言に由来する分類で返す。
func candidateConditionSpecs(
	origin matchSide, counterpart sideDeclarations, selection MatchConditionSelection,
) ([]core.MatchConditionSpec, error) {
	specs := make([]core.MatchConditionSpec, 0, len(origin.matchConditions))
	for _, declaration := range origin.matchConditions {
		if len(declaration.Semantics) == 0 {
			return nil, fmt.Errorf("composing the %q condition: the origin source names no semantic",
				string(declaration.ConditionKey))
		}
		semantics := counterpart.conditionSemantics[declaration.ConditionKey]
		if len(semantics) == 0 {
			semantics = declaration.Semantics
		}
		// 収集元が候補を絞れると挙げていない条件は、選んでも絞りに使えない。その条件で
		// 比べられる値を、その収集元のレコードが持たないためである。**候補の側の収集元が
		// 絞れると挙げていない条件も使えない** (sideDeclarations.narrows)。
		_, chosen := selection.narrows(declaration.ConditionKey)
		compared := declaration.NarrowsCandidates && chosen &&
			counterpart.narrows(declaration.ConditionKey)
		// ホスト名だけを持つ起点は、接続先 IP の条件を選んでも絞りに使えない (namesHostnameOnly)。
		if declaration.ConditionKey == core.ConditionKeyDestinationIp && namesHostnameOnly(origin) {
			compared = false
		}
		if compared && len(semantics) > 1 {
			return nil, fmt.Errorf("composing the %q condition: the candidate sources name "+
				"more than one semantic for a condition that narrows candidates",
				string(declaration.ConditionKey))
		}
		specs = append(specs, core.MatchConditionSpec{
			ConditionKey:         declaration.ConditionKey,
			OriginSemantic:       declaration.Semantics[0],
			CounterpartSemantics: slices.Clone(semantics),
			Compared:             compared,
		})
	}
	return specs, nil
}

// anyComparedCondition は、候補を絞るのに用いる条件が 1 つ以上あるかを返す。
func anyComparedCondition(specs []core.MatchConditionSpec) bool {
	for _, spec := range specs {
		if spec.Compared {
			return true
		}
	}
	return false
}

// sideDeclarations は片側のレコードを出した収集元の宣言を 1 つにまとめたものである。
//
// 段階の条件も、候補として数えるレコードの母集合も、候補の側の収集元の組で決まる。
// **関連付け 1 件が探した候補のレコードから決めない。** 候補が 1 件も無い要求でも、
// 相手の収集元が何を持つかは定まる。
type sideDeclarations struct {
	// conditionSemantics は条件の種別ごとに、この側の収集元が比べる語彙の項目を集めた表である。
	conditionSemantics map[core.ConditionKey][]core.SemanticKey
	// connectionRequestKinds は、この側の収集元が外向きの通信の要求として挙げた観測の種別で
	// ある。
	connectionRequestKinds []core.ObservationKindSelector
	// sources は宣言を取り込んだ収集元の識別子である。同じ収集元を 2 回取り込まない。
	sources map[string]struct{}
	// narrowing は、この側の収集元が候補を絞れると挙げた条件の種別である。
	narrowing map[core.ConditionKey]struct{}
}

// newSideDeclarations は宣言を 1 つも持たない状態を組む。
func newSideDeclarations() sideDeclarations {
	return sideDeclarations{
		conditionSemantics:     make(map[core.ConditionKey][]core.SemanticKey),
		connectionRequestKinds: make([]core.ObservationKindSelector, 0),
		sources:                make(map[string]struct{}),
		narrowing:              make(map[core.ConditionKey]struct{}),
	}
}

// narrows は、この側の収集元がその条件で候補を絞れるかを返す。
//
// **宣言を 1 つも取り込んでいない側は、どの条件でも絞れるとする。** 候補の無い組は、起点の
// 側の宣言と選択だけで条件を組む。
func (d sideDeclarations) narrows(key core.ConditionKey) bool {
	if len(d.sources) == 0 {
		return true
	}
	_, narrowing := d.narrowing[key]
	return narrowing
}

// declarationsKeyOf は、収集元 1 件の関連付けの条件の宣言を 1 つの文字列にまとめる。同じ宣言の
// 収集元を、候補の同じ組に集める (candidateSideGroups)。
func declarationsKeyOf(declarations []ConnectionMatchCondition) string {
	var key strings.Builder
	for _, declaration := range declarations {
		key.WriteString(strconv.Quote(string(declaration.ConditionKey)))
		for _, semantic := range declaration.Semantics {
			key.WriteString(strconv.Quote(string(semantic)))
		}
		key.WriteString(strconv.FormatBool(declaration.NarrowsCandidates))
	}
	return key.String()
}

// add は収集元 1 件の宣言を取り込む。
func (d *sideDeclarations) add(publication SourcePublication) {
	sourceId := publication.status.SourceId
	if _, taken := d.sources[sourceId]; taken {
		return
	}
	d.sources[sourceId] = struct{}{}
	for _, declaration := range publication.ConnectionMatchConditions() {
		semantics := d.conditionSemantics[declaration.ConditionKey]
		for _, semantic := range declaration.Semantics {
			if slices.Contains(semantics, semantic) {
				continue
			}
			semantics = append(semantics, semantic)
		}
		d.conditionSemantics[declaration.ConditionKey] = semantics
		if declaration.NarrowsCandidates {
			d.narrowing[declaration.ConditionKey] = struct{}{}
		}
	}
	for _, selector := range publication.ConnectionRequestKinds() {
		if containsSelector(d.connectionRequestKinds, selector) {
			continue
		}
		d.connectionRequestKinds = append(d.connectionRequestKinds, selector)
	}
}

// observationKinds は候補として数えるレコードを選ぶ観測の種別を返す。
//
// **この側の収集元が挙げた種別の和集合を返す。** 観測の種別の欄を持たない収集元が混ざる
// 組でも、欄を持つ収集元が挙げた種別を除かない。
//
// **種別の欄を持たない収集元のレコードは、この集合で除かれない。** 選択子が一致するのは観測の
// 種別を持つ候補だけであり、種別を持たない候補は選択子を持つ段階にも入る
// (backend/core の selectedByObservationKind)。和集合を返さずに要素数 0 へまとめると、欄を
// 持つ収集元の候補が種別で絞られなくなる。
//
// 要素数 0 を返すのは、この側の収集元が種別を 1 つも挙げないときである。その段階は種別で
// 絞らない。
func (d sideDeclarations) observationKinds() []core.ObservationKindSelector {
	return slices.Clone(d.connectionRequestKinds)
}

// derivedObservationSemantic は、レコードの項目ではなく別の収集元の割当から導く語彙の
// 項目である。観測へ入れるのは呼び出し元であり、レコードの項目からは求められない。
const derivedObservationSemantic = core.SemanticKeyTerminalId

// requiredOriginSemantics は起点の観測に必ず入れる語彙の項目を返す。
//
// **観測へ入れる項目と、core が絞りに用いる項目を同じ宣言から導く。** 2 か所で別に
// 持つと、宣言とずれた起点で関連付けが error になり、候補がエラーを出さずに消える。
//
// 事象の時刻は絞りに用いる条件でなくても必ず入れる。段階の second_of_time が起点の側の
// 項目の名前を取る (backend/core の MatchOrigin.validateComparableFields)。
func requiredOriginSemantics(specs []core.MatchConditionSpec) []core.SemanticKey {
	semantics := make([]core.SemanticKey, 0, len(specs))
	seen := make(map[core.SemanticKey]struct{}, len(specs))
	add := func(semantic core.SemanticKey) {
		if semantic == derivedObservationSemantic {
			return
		}
		if _, duplicate := seen[semantic]; duplicate {
			return
		}
		seen[semantic] = struct{}{}
		semantics = append(semantics, semantic)
	}
	for _, spec := range specs {
		if spec.Compared {
			add(spec.OriginSemantic)
		}
	}
	add(core.SemanticKeyEventTime)
	return semantics
}

// optionalOriginSemantics は、起点の観測に在れば入れる語彙の項目を返す。
//
// 段階の条件が起点の側の値を出すために要る。持たない起点でも関連付けは進むため、欠けても
// 観測を組めなかったことにしない。
func optionalOriginSemantics(specs []core.MatchConditionSpec) []core.SemanticKey {
	return optionalSemantics(specs, requiredOriginSemantics(specs),
		func(spec core.MatchConditionSpec) []core.SemanticKey {
			return []core.SemanticKey{spec.OriginSemantic}
		})
}

// requiredCounterpartSemantics は候補の観測に必ず入れる語彙の項目を返す。
func requiredCounterpartSemantics(specs []core.MatchConditionSpec) []core.SemanticKey {
	semantics := make([]core.SemanticKey, 0, len(specs))
	seen := make(map[core.SemanticKey]struct{}, len(specs))
	add := func(semantic core.SemanticKey) {
		if semantic == derivedObservationSemantic {
			return
		}
		if _, duplicate := seen[semantic]; duplicate {
			return
		}
		seen[semantic] = struct{}{}
		semantics = append(semantics, semantic)
	}
	for _, spec := range specs {
		if !spec.Compared {
			continue
		}
		// 絞りに用いる条件は相手の語彙の項目を 1 件だけ挙げる
		// (backend/core の MatchRequest.validateConditions が確かめる)。
		add(spec.CounterpartSemantics[0])
	}
	add(core.SemanticKeyEventTime)
	return semantics
}

// optionalCounterpartSemantics は、候補の観測に在れば入れる語彙の項目を返す。
func optionalCounterpartSemantics(specs []core.MatchConditionSpec) []core.SemanticKey {
	return optionalSemantics(specs, requiredCounterpartSemantics(specs),
		func(spec core.MatchConditionSpec) []core.SemanticKey {
			return spec.CounterpartSemantics
		})
}

// optionalSemantics は宣言が挙げた語彙の項目から、必須の一覧に無いものを返す。
func optionalSemantics(
	specs []core.MatchConditionSpec, required []core.SemanticKey,
	pick func(core.MatchConditionSpec) []core.SemanticKey,
) []core.SemanticKey {
	seen := make(map[core.SemanticKey]struct{}, len(required)+len(specs))
	for _, semantic := range required {
		seen[semantic] = struct{}{}
	}
	seen[derivedObservationSemantic] = struct{}{}
	semantics := make([]core.SemanticKey, 0, len(specs))
	for _, spec := range specs {
		for _, semantic := range pick(spec) {
			if _, duplicate := seen[semantic]; duplicate {
				continue
			}
			seen[semantic] = struct{}{}
			semantics = append(semantics, semantic)
		}
	}
	return semantics
}

// unionItemSemantics は候補の側の収集元が運びうる語彙の項目の和集合を返す。
//
// **和集合を採る。** 候補は収集元をまたいで集まるため、1 つの段階の候補が複数の収集元に
// またがりうる。欄を持つ収集元が 1 つでもあれば「相手の入力形式に欄が無い」とは
// 言えない。
func unionItemSemantics(sides []matchSide) []core.SemanticKey {
	semantics := make([]core.SemanticKey, 0)
	seen := make(map[core.SemanticKey]struct{})
	for _, side := range sides {
		for _, semantic := range side.itemSemantics {
			if _, duplicate := seen[semantic]; duplicate {
				continue
			}
			seen[semantic] = struct{}{}
			semantics = append(semantics, semantic)
		}
	}
	return semantics
}

// containsSelector は同じ欄と value の組を挙げる宣言が既にあるかを返す。
//
// 並びの違いを別の宣言として数えない。core.ObservationKind.Matches は欄を名前で探すため、
// 要素の並びは一致の判定に作用しない。
func containsSelector(
	selectors []core.ObservationKindSelector, candidate core.ObservationKindSelector,
) bool {
	for _, selector := range selectors {
		if sameSelector(selector, candidate) {
			return true
		}
	}
	return false
}

// sameSelector は 2 つの宣言が同じ欄と value の組を挙げるかを返す。
func sameSelector(left, right core.ObservationKindSelector) bool {
	leftValues := selectorValues(left)
	rightValues := selectorValues(right)
	// **要素数ではなく、異なる欄の名前の個数で比べる。** 同じ名前を 2 回挙げる宣言は
	// レコードを 1 件も解析する前に ParserIdentity.Validate が拒むが、要素数だけで比べる
	// 形にすると、名前を繰り返した宣言が別の宣言と同じ組に見える。
	if len(leftValues) != len(rightValues) {
		return false
	}
	for name, value := range leftValues {
		other, found := rightValues[name]
		if !found || other != value {
			return false
		}
	}
	return true
}

// selectorValues は宣言が挙げた欄の名前から value を探す表を返す。
func selectorValues(selector core.ObservationKindSelector) map[string]string {
	values := make(map[string]string, len(selector.Items))
	for _, item := range selector.Items {
		values[item.Name] = item.Value
	}
	return values
}

// coarsestTimePrecision は候補の側の収集元の時刻の精度のうち、最も粗いものを返す。
//
// **最も粗い精度を採る。** 段階の条件は段階の全候補に係る 1 件であり、秒までしか持たない
// 収集元が 1 つでも混ざる段階では、秒未満を全候補について比べられるとは言えない。
// 候補が 1 件も無い段階は、比べる相手が無いため空を返す。
func coarsestTimePrecision(sides []matchSide) core.Precision {
	// **空の文字列を未初期化の目印にしない。** 収集元の精度そのものが空を取りうるため、
	// 目印と値が衝突する。先頭の収集元が空の精度を持つ組で、後続の既知の精度に
	// 上書きされる。
	coarsest := core.Precision("")
	found := false
	for _, side := range sides {
		if !found || side.timePrecision.IsCoarserThan(coarsest) {
			coarsest = side.timePrecision
			found = true
		}
	}
	return coarsest
}
