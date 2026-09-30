package core

import (
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"time"
)

// 検索の条件の項目の名前。部分グラフの要求の query の項目と、検索の条件の JSON の項目が
// 同じ名前を使う。error message はこの名前で項目を指す。
const (
	searchNodeKindItem         = "nodeKind"
	searchNodeIdItem           = "nodeId"
	searchDepthItem            = "depth"
	searchEdgeKindItem         = "edgeKind"
	searchValueContainsItem    = "valueContains"
	searchValueExcludesItem    = "valueExcludes"
	searchValueFieldItem       = "valueField"
	searchFieldContainsItem    = "fieldContains"
	searchFieldEqualsItem      = "fieldEquals"
	searchAddressInCidrItem    = "addressInCidr"
	searchAddressNotInCidrItem = "addressNotInCidr"
	searchCountByItem          = "countBy"
	searchEventActionFromItem  = "eventActionFrom"
	searchEventActionToItem    = "eventActionTo"
	searchTimeFromItem         = "timeFrom"
	searchTimeFromPrecision    = "timeFromPrecision"
	searchTimeToItem           = "timeTo"
	searchTimeToPrecision      = "timeToPrecision"
	searchFilterUnitItem       = "filterUnit"
	searchCaseItem             = "case"
	searchSourceItem           = "source"
)

// 起点から辿る段数の範囲。0 はノードだけ、1 は隣り合うノードまで、2 以上はその段数だけ
// 辿って到達したノードまでを返す。
//
// 既知の制限: 起点から辿る段数の上限を 8 に固定し、要求ごとに上限を上げる項目を置かない,
// 関係の種別を与えない要求は少ない段数でグラフの全域へ届くため、返す量は段数より展開の度合いが
// 決める。上限に届く連鎖を持つ入力は repo の中に無い,
// 上限に届く連鎖を収集元で確認したときに見直す。
const (
	MinSearchDepth = 0
	MaxSearchDepth = 8
)

// MaxSearchOriginNodes は、1 つの検索の条件が近傍を広げる起点として指定できるノードの数の上限である。
//
// 既知の制限: 起点の数を 32 に固定する, 起点は分析者がノードを 1 つずつ選んで足すため、
// 数は操作の回数に等しい。起点が多いほど近傍の和が広がり、グラフに描く上限 (画面の既定 200)
// に先に達する。起点を 32 個足す操作は実測していない, 分析者が 32 個を超える起点を足す
// 操作を実測したときに見直す
const MaxSearchOriginNodes = 32

// 検索の文字列の数と長さの上限。
//
// 既知の制限: 含む文字列と含まない文字列を合わせて 16 個、1 つの文字列を 1,024 byte までに固定する,
// 文字列の判定は文字列ごとに全ノードの全属性を走査するため、要求の手間は文字列の数と長さに比例する。
// 上限ちょうどの要求の所要は測っていない, 分析者が 16 個を超える文字列を重ねる操作を実測したときに
// 見直す
const (
	MaxSearchTokens      = 16
	MaxSearchTokenLength = 1024
)

// MaxSafeInteger は、JSON の整数を JavaScript の Number で正確に受け取れる上限である。
const MaxSafeInteger int64 = 1<<53 - 1

// SearchQuery は、検索欄が表せる検索の条件である。部分グラフの要求と、LLM が組み立てた
// 検索の条件が同じ型を使う。
//
// **実行と分けて検証する。** Validate を通った条件は、取り込み結果に依る判定 (案件、端末、
// 起点のノードの有無) を除いて、部分グラフの要求として受け付けられる。**文字列の項目は、空の
// 文字列を「条件を与えない」として扱う。**
type SearchQuery struct {
	// NodeKinds はノードの種別の条件である。種別のどれかに一致するノードが残る。
	NodeKinds []NodeKind `json:"nodeKinds,omitempty" jsonschema:"node kinds to keep, as the kinds overview lists, such as process, ip, terminal, or record"`
	// Granularity はレコードのノードを出すかである。空の値はレコードの粒度として扱う。
	Granularity GraphGranularity `json:"granularity,omitempty" jsonschema:"record or object; record shows record nodes too, object shows only the nodes the records name and carries no record nodeKind; graph_search reads an empty value as record, and show_search_query leaves it to the screen"`
	// NodeIds は近傍を広げる起点のノードの識別子である。
	NodeIds []string `json:"nodeIds,omitempty" jsonschema:"origin node references issued in this conversation, such as n3; the search keeps the nodes within depth hops of the origins"`
	// Depth は起点から広げる段数である。
	Depth int `json:"depth" jsonschema:"required hops from 0 to 8, expanded from nodeIds or, without nodeIds, from every node the conditions match; 0 returns the nodes and no edges, and 1 or more also returns the neighbours and edges within that many hops"`
	// EdgeKinds は関係の種別の条件である。
	EdgeKinds []EdgeKind `json:"edgeKinds,omitempty" jsonschema:"edge kinds to follow, as the kinds of the edges graph_search returns, such as ran_on or process_parent_child"`
	// ValueContains は、ノードの属性の値にどれも含まれることを求める文字列である。
	// ValueExcludes は、どれも含まれないことを求める文字列である。
	ValueContains []string `json:"valueContains,omitempty" jsonschema:"strings each of which some attribute value of the node must contain; the attributes may differ per string"`
	ValueExcludes []string `json:"valueExcludes,omitempty" jsonschema:"strings none of which any attribute value of the node may contain"`
	// ValueField は文字列を当てる欄の指定である。語彙の項目か原資料の key の文字列を持つ。
	ValueField string `json:"valueField,omitempty" jsonschema:"a field name that limits valueContains and valueExcludes to that field; needs valueContains or valueExcludes"`
	// FieldContains は欄と文字列の組である。`欄=文字列` の文字列で、どの組もその欄に文字列を含む
	// ことを求める。最初の `=` で欄と文字列を分ける。ValueField の指定は組に当てない。
	FieldContains []string `json:"fieldContains,omitempty" jsonschema:"field and string pairs written as field=string; the field value must contain the string"`
	// FieldEquals は、値の全体が文字列と等しい欄だけを一致とする欄と文字列の組である。書き方は
	// FieldContains と同じである。
	FieldEquals []string `json:"fieldEquals,omitempty" jsonschema:"field and string pairs written as field=string; the whole field value must equal the string"`
	// SearchExpression は、欄・演算子・論理・括弧で書いた検索式の文字列である。構文は pipeline の
	// 検索式の解析が確かめる。
	SearchExpression string `json:"searchExpression,omitempty" jsonschema:"comparisons written as field operator value joined by and, or, not, and parentheses; operators are == != > >= < <= contains; quote a value holding spaces with double quotes"`
	// ConditionsOnOriginsOnly は、文字列と事象の種別の条件を起点の判定だけに当てるかである。
	ConditionsOnOriginsOnly bool `json:"conditionsOnOriginsOnly,omitempty" jsonschema:"true applies the string and event conditions only to choosing the origins"`
	// EndpointRecordsInPeriod は、端点のレコードが期間の外にあるエッジを辿らないかである。
	EndpointRecordsInPeriod bool `json:"endpointRecordsInPeriod,omitempty" jsonschema:"true skips edges whose endpoint records fall outside the period of timeFrom and timeTo"`
	// CountBy は値ごとに数える欄の指定である。語彙の項目か原資料の key の文字列を持つ。
	CountBy string `json:"countBy,omitempty" jsonschema:"a field name whose values the search counts"`
	// AddressInCidr と AddressNotInCidr は、IP アドレスのノードを絞るアドレスの範囲の文字列である。
	// 先頭のアドレスと prefix の長さを `/` で繋いだ形である。
	AddressInCidr    string `json:"addressInCidr,omitempty" jsonschema:"an address range in CIDR form, such as 192.0.2.0/24, that ip nodes must fall in"`
	AddressNotInCidr string `json:"addressNotInCidr,omitempty" jsonschema:"an address range in CIDR form that ip nodes must fall outside"`
	// RecordConditions は根拠のレコードを絞る条件である。
	RecordConditions
}

// RecordConditions は、根拠のレコードを絞る条件である。部分グラフの検索と時系列が同じ条件を使う。
type RecordConditions struct {
	// EventCategory と EventAction は、根拠のレコードの事象の分類の文字列である。完全一致で比べる。
	EventCategory string `json:"eventCategory,omitempty" jsonschema:"the event category of the evidence records, compared whole, as overview lists"`
	EventAction   string `json:"eventAction,omitempty" jsonschema:"the event action of the evidence records, compared whole, as overview lists, such as 4624"`
	// EventActionFrom と EventActionTo は、事象の動作を 10 進の数として比べる範囲の両端である。
	// 両端を含む。
	EventActionFrom *uint64 `json:"eventActionFrom,omitempty" jsonschema:"the lowest event action read as a decimal number, inclusive"`
	EventActionTo   *uint64 `json:"eventActionTo,omitempty" jsonschema:"the highest event action read as a decimal number, inclusive"`
	// TimeFrom と TimeTo は根拠のレコードの時刻で絞る期間の両端の文字列である。日付と UTC からの
	// ずれを含む RFC 3339 の形である。TimeFromPrecision と TimeToPrecision はその精度である。
	TimeFrom          string    `json:"timeFrom,omitempty" jsonschema:"the start of the period of the evidence records, an RFC 3339 date-time with an offset such as 2026-01-02T03:04:05Z; needs timeFromPrecision and filterUnit"`
	TimeFromPrecision Precision `json:"timeFromPrecision,omitempty" jsonschema:"the precision timeFrom is written in: second (no second fraction), millisecond (3 fraction digits), or microsecond (6 fraction digits)"`
	TimeTo            string    `json:"timeTo,omitempty" jsonschema:"the end of the period of the evidence records, written as timeFrom; needs timeToPrecision and filterUnit"`
	TimeToPrecision   Precision `json:"timeToPrecision,omitempty" jsonschema:"the precision timeTo is written in, with the values of timeFromPrecision"`
	// FilterUnit は期間の判定に用いる比較の単位である。期間の端を与えた条件は必ず与える。
	FilterUnit FilterUnit `json:"filterUnit,omitempty" jsonschema:"the unit the period compares record times in: second, millisecond, or microsecond; required with timeFrom or timeTo and absent otherwise"`
	// Case は根拠のレコードの収集元に付けた案件である。
	Case string `json:"case,omitempty" jsonschema:"the case of the evidence sources, as the screen context holds"`
	// Terminal は根拠のレコードを置いた端末のノードの識別子である。
	Terminal string `json:"terminal,omitempty" jsonschema:"the terminal node id the evidence records belong to, copied from the screen context; node references such as n3 are not accepted"`
	// Sources は根拠のレコードを絞る収集元の sourceId である。どれかの収集元のレコードが残る。
	Sources []string `json:"sources,omitempty" jsonschema:"sourceId values overview lists; records of any of the sources remain"`
}

// FilterUnit は期間の判定に用いる時刻の比較の単位である。
type FilterUnit string

// FilterUnit の値。
const (
	FilterUnitSecond      FilterUnit = "second"
	FilterUnitMillisecond FilterUnit = "millisecond"
	FilterUnitMicrosecond FilterUnit = "microsecond"
)

// IsKnown は値が列挙の定義の中にあることを返す。
func (u FilterUnit) IsKnown() bool {
	return u == FilterUnitSecond || u == FilterUnitMillisecond || u == FilterUnitMicrosecond
}

// Duration は比較の単位を、時刻の下の桁を切り捨てる幅へ直す。空の値は秒として扱う。
func (u FilterUnit) Duration() time.Duration {
	if u == FilterUnitMillisecond {
		return time.Millisecond
	}
	if u == FilterUnitMicrosecond {
		return time.Microsecond
	}
	return time.Second
}

// RequestedBound は条件が与えた期間の端である。Requested は応答へそのまま返す組、Instant は
// 絞り込みの比較に用いる時刻である。
type RequestedBound struct {
	Requested RequestedTime
	Instant   time.Time
}

// Validate は検索の条件の項目の整合を確かめる。
//
// **利用者が入れた文字列を error message に載せない。** 載せるのは項目の名前と上限だけである。
func (q SearchQuery) Validate() error {
	if err := q.validateValueField(); err != nil {
		return err
	}
	if err := q.validateEventAction(); err != nil {
		return err
	}
	if err := q.validateCase(); err != nil {
		return err
	}
	if err := q.validateSources(); err != nil {
		return err
	}
	if err := q.validateElements(); err != nil {
		return err
	}
	if err := q.validateFieldPairs(); err != nil {
		return err
	}
	if err := q.validateKinds(); err != nil {
		return err
	}
	if q.Depth < MinSearchDepth || q.Depth > MaxSearchDepth {
		return fmt.Errorf("%s must be between %d and %d", searchDepthItem, MinSearchDepth, MaxSearchDepth)
	}
	if _, _, err := q.AddressPrefixes(); err != nil {
		return err
	}
	if _, _, err := q.Period(); err != nil {
		return err
	}
	return nil
}

// ValidateShowable は、画面の検索欄が表せる条件であることを確かめる。Validate を通った条件に使う。
//
// 図に出す対象は「検索の条件から決める」か、ノードの種別の組で表し、種別の組がレコードを含むとき
// だけレコードの粒度で出す。
func (q SearchQuery) ValidateShowable() error {
	if q.Granularity == GraphGranularityRecord && !slices.Contains(q.NodeKinds, NodeKindRecord) {
		return errors.New("the search field of the screen shows the record granularity only with the record " +
			searchNodeKindItem)
	}
	return nil
}

// validateValueField は、文字列を当てる欄の指定が文字列を伴うことを確かめる。文字列を伴わない欄の
// 指定は、通知なしに作用しない条件になる。
func (q SearchQuery) validateValueField() error {
	if q.ValueField == "" {
		return nil
	}
	if len(q.ValueField) > MaxSearchTokenLength || len(q.ValueContains)+len(q.ValueExcludes) == 0 {
		return errors.New(searchValueFieldItem + " must name a field and come with a search term")
	}
	return nil
}

// validateElements は、集合の項目が空の要素を持たないことと、数と長さの上限を確かめる。
func (q SearchQuery) validateElements() error {
	for _, item := range []struct {
		name   string
		values []string
	}{
		{searchNodeKindItem, stringsOf(q.NodeKinds)}, {searchEdgeKindItem, stringsOf(q.EdgeKinds)},
		{searchValueContainsItem, q.ValueContains}, {searchValueExcludesItem, q.ValueExcludes},
		{searchNodeIdItem, q.NodeIds},
	} {
		if slices.Contains(item.values, "") {
			return errors.New(item.name + " must not be empty")
		}
	}
	if len(q.NodeIds) > MaxSearchOriginNodes {
		return fmt.Errorf("%s must occur at most %d times", searchNodeIdItem, MaxSearchOriginNodes)
	}
	tokens := slices.Concat(q.ValueContains, q.ValueExcludes)
	if len(tokens)+len(q.FieldContains)+len(q.FieldEquals) > MaxSearchTokens {
		return fmt.Errorf("%s, %s, %s and %s must carry at most %d tokens together",
			searchValueContainsItem, searchValueExcludesItem, searchFieldContainsItem, searchFieldEqualsItem,
			MaxSearchTokens)
	}
	for _, token := range tokens {
		if len(token) > MaxSearchTokenLength {
			return fmt.Errorf("a search token must be at most %d bytes", MaxSearchTokenLength)
		}
	}
	return nil
}

// validateFieldPairs は、欄と文字列の組がどれも `=` で繋いだ空でない欄と文字列を持ち、長さの
// 上限に収まることを確かめる。欄の名前は `=` を含まないため、最初の `=` で分ける。
func (q SearchQuery) validateFieldPairs() error {
	for _, item := range []struct {
		name  string
		pairs []string
	}{{searchFieldContainsItem, q.FieldContains}, {searchFieldEqualsItem, q.FieldEquals}} {
		for _, pair := range item.pairs {
			field, contains, found := strings.Cut(pair, "=")
			if !found || field == "" || contains == "" ||
				len(field) > MaxSearchTokenLength || len(contains) > MaxSearchTokenLength {
				return errors.New(item.name + " must carry a field and a search term joined by =")
			}
		}
	}
	return nil
}

// validateKinds は列挙の項目を確かめる。
func (q SearchQuery) validateKinds() error {
	for _, kind := range q.NodeKinds {
		if !kind.IsKnown() {
			return errors.New(searchNodeKindItem + " is outside the contract")
		}
	}
	if q.Granularity != "" && !q.Granularity.IsKnown() {
		return errors.New("granularity is outside the contract")
	}
	if q.Granularity == GraphGranularityObject && slices.Contains(q.NodeKinds, NodeKindRecord) {
		return errors.New("the object granularity carries no record node")
	}
	for _, kind := range q.EdgeKinds {
		if !kind.IsKnown() {
			return errors.New(searchEdgeKindItem + " is outside the contract")
		}
	}
	return nil
}

// AddressPrefixes はアドレスの範囲の 2 つの項目を読む。与えていない項目は nil である。
//
// 範囲の中の値を持つ文字列 (`172.16.0.1/20`) は、その長さが表す範囲へ直して返す。
func (q SearchQuery) AddressPrefixes() (in, notIn *netip.Prefix, err error) {
	for _, item := range []struct {
		name, text string
		target     **netip.Prefix
	}{
		{searchAddressInCidrItem, q.AddressInCidr, &in},
		{searchAddressNotInCidrItem, q.AddressNotInCidr, &notIn},
	} {
		if item.text == "" {
			continue
		}
		prefix, parseErr := netip.ParsePrefix(item.text)
		if parseErr != nil {
			return nil, nil, errors.New(item.name + " must be an address range")
		}
		masked := prefix.Masked()
		*item.target = &masked
	}
	return in, notIn, nil
}

// Validate は根拠のレコードを絞る条件の項目の整合を確かめる。
func (c RecordConditions) Validate() error {
	if err := c.validateEventAction(); err != nil {
		return err
	}
	if err := c.validateCase(); err != nil {
		return err
	}
	if err := c.validateSources(); err != nil {
		return err
	}
	_, _, err := c.Period()
	return err
}

// validateSources は収集元の項目が空の要素を持たないことを確かめる。取り込み結果にその収集元が
// あるかは、取り込み結果を持つ呼び出し元が確かめる。
func (c RecordConditions) validateSources() error {
	if slices.Contains(c.Sources, "") {
		return errors.New(searchSourceItem + " must not be empty")
	}
	return nil
}

// validateEventAction は事象の動作の範囲の両端を確かめる。応答は条件の値を返すため、
// JavaScript の Number で正確に読める上限を超える値を退ける。
func (c RecordConditions) validateEventAction() error {
	for _, bound := range []struct {
		name  string
		value *uint64
	}{{searchEventActionFromItem, c.EventActionFrom}, {searchEventActionToItem, c.EventActionTo}} {
		if bound.value != nil && *bound.value > uint64(MaxSafeInteger) {
			return errors.New(bound.name + " must be a decimal number up to 2^53-1")
		}
	}
	if c.EventActionFrom != nil && c.EventActionTo != nil && *c.EventActionFrom > *c.EventActionTo {
		return errors.New(searchEventActionFromItem + " must not exceed " + searchEventActionToItem)
	}
	return nil
}

// validateCase は案件の文字列を確かめる。取り込み結果にその案件があるかは、取り込み結果を持つ
// 呼び出し元が確かめる。
func (c RecordConditions) validateCase() error {
	if c.Case == "" {
		return nil
	}
	if err := ValidateCaseId(searchCaseItem, c.Case); err != nil {
		return fmt.Errorf("reading the requested case: %w", err)
	}
	return nil
}

// Period は期間の両端を読む。与えていない端は nil である。
//
// **期間の端を与えた条件は比較の単位を必ず与える。** 単位を省いた条件に既定の単位を当てると、
// millisecond の精度で書いた端が秒で比べられたことを応答から読めない。
func (c RecordConditions) Period() (from, to *RequestedBound, err error) {
	if c.TimeFrom == "" && c.TimeTo == "" {
		if c.FilterUnit != "" {
			return nil, nil, errors.New(searchFilterUnitItem + " needs " + searchTimeFromItem + " or " +
				searchTimeToItem)
		}
		return nil, nil, c.checkUnpairedPrecisions()
	}
	if c.FilterUnit == "" {
		return nil, nil, errors.New(searchFilterUnitItem + " is required with " + searchTimeFromItem +
			" or " + searchTimeToItem)
	}
	if !c.FilterUnit.IsKnown() {
		return nil, nil, errors.New(searchFilterUnitItem + " must be second, millisecond, or microsecond")
	}
	if err := c.checkUnpairedPrecisions(); err != nil {
		return nil, nil, fmt.Errorf("checking the precision items of the requested period: %w", err)
	}
	for _, bound := range []struct {
		name, text, precisionName string
		precision                 Precision
		target                    **RequestedBound
	}{
		{searchTimeFromItem, c.TimeFrom, searchTimeFromPrecision, c.TimeFromPrecision, &from},
		{searchTimeToItem, c.TimeTo, searchTimeToPrecision, c.TimeToPrecision, &to},
	} {
		if bound.text == "" {
			continue
		}
		parsed, parseErr := parseRequestedBound(bound.name, bound.precisionName, bound.text, bound.precision)
		if parseErr != nil {
			return nil, nil, fmt.Errorf("reading a bound of the requested period: %w", parseErr)
		}
		*bound.target = &parsed
	}
	return from, to, nil
}

// checkUnpairedPrecisions は、対になる文字列を持たない精度の項目を退ける。
func (c RecordConditions) checkUnpairedPrecisions() error {
	for _, pair := range []struct {
		textName, text, precisionName string
		precision                     Precision
	}{
		{searchTimeFromItem, c.TimeFrom, searchTimeFromPrecision, c.TimeFromPrecision},
		{searchTimeToItem, c.TimeTo, searchTimeToPrecision, c.TimeToPrecision},
	} {
		if pair.precision != "" && pair.text == "" {
			return errors.New(pair.precisionName + " needs " + pair.textName)
		}
	}
	return nil
}

// 期間の端の文字列が従う書式。日付と UTC からのずれを含む形だけを受け取る。
const (
	millisecondBoundLayout = "2006-01-02T15:04:05.000Z07:00"
	microsecondBoundLayout = "2006-01-02T15:04:05.000000Z07:00"
)

// derivationRequestedTime は要求の文字列から正規化値を導いた方法である。分析者が画面で
// 読む値であるため日本語で書く。
const derivationRequestedTime = "要求の文字列が持つ日付と UTC からのずれをそのまま用い、" +
	"RFC 3339 の書式で書き直した"

// parseRequestedBound は期間の端の文字列と精度から、応答へ返す組と比較に用いる時刻を作る。
//
// 受け取る文字列は日付と UTC からのずれを含む形に限る。秒と秒未満のどちらの精度でもこの形を
// 書けるため、対応する精度は second、millisecond、microsecond の 3 値である。
func parseRequestedBound(item, precisionItem, text string, precision Precision) (RequestedBound, error) {
	layout := time.RFC3339
	wantedDigits := secondFractionDigits
	switch precision {
	case PrecisionSecond:
	case PrecisionMillisecond:
		layout = millisecondBoundLayout
		wantedDigits = millisecondFractionDigits
	case PrecisionMicrosecond:
		layout = microsecondBoundLayout
		wantedDigits = microsecondFractionDigits
	default:
		return RequestedBound{}, errors.New(item + " needs " + precisionItem +
			", which must be second, millisecond, or microsecond")
	}
	instant, err := time.Parse(time.RFC3339, text)
	if err != nil || fractionDigitsOf(text) != wantedDigits {
		return RequestedBound{}, errors.New(item + " must be a date-time with an offset whose second fraction " +
			"has the digits of " + precisionItem + ": none for second, 3 for millisecond, 6 for microsecond")
	}
	requested := RequestedTime{
		RequestText:    text,
		Precision:      precision,
		OffsetState:    OffsetStateInValue,
		Normalized:     instant.Format(layout),
		NormalizedForm: NormalizedFormRFC3339Absolute,
		Derivation:     derivationRequestedTime,
	}
	if err := requested.Validate(); err != nil {
		return RequestedBound{}, errors.New(item + " is not a time the response can return")
	}
	return RequestedBound{Requested: requested, Instant: instant}, nil
}

// fractionDigitsOf は時刻の文字列が持つ秒の小数部の桁数を返す。
//
// 小数点が現れるのは秒の後だけである。日付と時刻の区切りと UTC からのずれの文字列は
// 小数点を持たない。
func fractionDigitsOf(text string) int {
	position := strings.IndexByte(text, '.')
	if position < 0 {
		return 0
	}
	digits := 0
	for _, character := range text[position+1:] {
		if character < '0' || character > '9' {
			break
		}
		digits++
	}
	return digits
}

// stringsOf は文字列の列挙の並びを文字列の並びへ直す。
func stringsOf[T ~string](values []T) []string {
	out := make([]string, len(values))
	for index, value := range values {
		out[index] = string(value)
	}
	return out
}
