package pipeline

import (
	"cmp"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 既知の制限: 番号の抜けと突き合わせを要求のたびに全レコードから数え、結果を保持しない,
// 数万件の EVTX の収集元で、抜けの応答が 0.1 秒、別の収集元との突き合わせの応答が 1 秒に収まる,
// 要求の待ち時間が 1 秒を超える収集元が現れたとき、分析者の入力の状態ごとに結果を保持する

// RecordNumberingNames は、番号の抜けと突き合わせに使う欄の名前である (ParserIdentity.RecordNumbering)。
// 空の名前は、その形式が欄を宣言していないことを表す。
type RecordNumberingNames struct {
	// Channel と Computer は番号を振った単位の欄、EventRecordID は番号の欄である。
	Channel, Computer, EventRecordID string
	// RecordHeaderID は file の見出しと比べる、レコードの見出しの番号の欄である。
	RecordHeaderID string
	// ProviderName と EventID は、番号を持たない形式との突き合わせの鍵の欄である。
	ProviderName, EventID string
	// HeaderNextRecordID と HeaderDirty は収集元の識別の fileHeader の項目の名前である。
	HeaderNextRecordID, HeaderDirty string
}

// carriesNumbers は、形式が番号の欄を宣言したかを返す。
func (n RecordNumberingNames) carriesNumbers() bool { return n.EventRecordID != "" }

// comparable は、形式が時刻の鍵の欄を宣言したかを返す。
func (n RecordNumberingNames) comparable() bool { return n.ProviderName != "" && n.EventID != "" }

// numberingItems は 1 件のレコードの番号の欄の文字列である。欄が無いときは nil である。
type numberingItems struct {
	Channel, Computer, EventRecordID, RecordHeaderID, ProviderName, EventID *string
}

// numberingItemsOf はレコードの Fields を欄の名前で読む。意味付けに至らなかったレコードは欄を持たない。
func numberingItemsOf(record RecordEntry, names RecordNumberingNames) numberingItems {
	var items numberingItems
	if record.Semantics == nil {
		return items
	}
	for _, field := range record.Semantics.Fields {
		if field.Text == nil || field.Name == "" {
			continue
		}
		switch field.Name {
		case names.Channel:
			items.Channel = field.Text.RawText
		case names.Computer:
			items.Computer = field.Text.RawText
		case names.EventRecordID:
			items.EventRecordID = field.Text.RawText
		case names.RecordHeaderID:
			items.RecordHeaderID = field.Text.RawText
		case names.ProviderName:
			items.ProviderName = field.Text.RawText
		case names.EventID:
			items.EventID = field.Text.RawText
		}
	}
	return items
}

// headerItemOf は fileHeader の name の項目の文字列を返す。
func headerItemOf(header []core.RecordField, name string) *string {
	for _, field := range header {
		if name != "" && field.Name == name && field.Text != nil {
			return field.Text.RawText
		}
	}
	return nil
}

// maxReadableRecordNumber は読める番号の最大である。応答の番号は画面が値を丸めずに読める整数
// (2^53 - 1 以下) に収める。
const maxReadableRecordNumber = 1<<53 - 1

// recordNumberOf は番号の文字列を 10 進の符号の無い整数として読む。符号と空白と区切りの `_` を
// 持つ文字列は読まない (strconv.ParseUint は基数 10 でこれらを退ける)。
//
// 既知の制限: 2^53 以上の番号を読めない番号として数える,
// EventRecordID は 64 bit の欄だが、JSON の数値が正確に表せる整数は 2^53 未満である,
// 2^53 以上の番号を持つ収集元が現れたとき、応答の番号を文字列で渡す
func recordNumberOf(text *string) (uint64, bool) {
	if text == nil {
		return 0, false
	}
	number, err := strconv.ParseUint(*text, 10, 64)
	return number, err == nil && number <= maxReadableRecordNumber
}

// streamKey はチャネルと Computer の文字列の組である。番号の単位はチャネルだけを入れ、突き合わせの
// 鍵は両方を入れる。欄の無い値は has を偽にし、空の文字列と分ける。
type streamKey struct {
	channel, computer       string
	hasChannel, hasComputer bool
}

func streamKeyOf(items numberingItems) streamKey {
	key := streamKey{hasChannel: items.Channel != nil, hasComputer: items.Computer != nil}
	if key.hasChannel {
		key.channel = *items.Channel
	}
	if key.hasComputer {
		key.computer = *items.Computer
	}
	return key
}

func compareStreamKeys(left, right streamKey) int {
	return cmp.Or(
		compareBool(left.hasChannel, right.hasChannel), cmp.Compare(left.channel, right.channel),
		compareBool(left.hasComputer, right.hasComputer), cmp.Compare(left.computer, right.computer))
}

func compareBool(left, right bool) int {
	switch {
	case left == right:
		return 0
	case left:
		return 1
	default:
		return -1
	}
}

// numberedRecord は番号を読めたレコード 1 件である。at は収集元のレコードの並びの位置、computer は
// レコードが名乗った Computer である。
type numberedRecord struct {
	number   uint64
	at       int
	computer *string
}

// RecordNumbers は収集元 1 件のレコードの番号の抜けを、番号を振った単位ごとに返す。ok が偽に
// なるのは、sourceId の公開された収集元が無いときである。
//
// **抜けの理由を推測しない。** 抜けの直前と直後のレコードの間の byte 範囲に取り込みの失敗の
// 記録があるときだけ、その失敗を抜けに添える。
func (r ImportResult) RecordNumbers(sourceId string) (core.RecordNumbers, bool) {
	publication, found := r.Publication(sourceId)
	if !found {
		return core.RecordNumbers{}, false
	}
	names := publication.parser.RecordNumbering
	result := core.RecordNumbers{
		SourceId: sourceId, Streams: []core.RecordNumberStream{}, ListLimit: core.RecordNumberListLimit,
		FileHeader: fileHeaderComparisonOf(r.identities[sourceId].FileHeader, publication.records, names),
	}
	if !names.carriesNumbers() {
		result.Examination = core.RecordNumberExaminationNotExamined
		result.NotExaminedReason = core.RecordNumberNotExaminedNotDeclared
		return result, true
	}
	streams := map[streamKey][]numberedRecord{}
	unreadable := int64(0)
	for at, record := range publication.records {
		items := numberingItemsOf(record, names)
		number, readable := recordNumberOf(items.EventRecordID)
		if !readable {
			unreadable++
			continue
		}
		// 単位は同じチャネルである。Computer の名前が変わっても番号は続く。
		key := streamKeyOf(numberingItems{Channel: items.Channel})
		streams[key] = append(streams[key], numberedRecord{number: number, at: at, computer: items.Computer})
	}
	result.UnreadableRecordCount = &unreadable
	if len(streams) == 0 {
		result.Examination = core.RecordNumberExaminationNotExamined
		result.NotExaminedReason = core.RecordNumberNotExaminedNumbersUnreadable
		return result, true
	}
	result.Examination = core.RecordNumberExaminationNoGaps
	keys := make([]streamKey, 0, len(streams))
	for key := range streams {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, compareStreamKeys)
	for _, key := range keys {
		stream := streamOf(key, streams[key], publication)
		if len(stream.Gaps) > 0 {
			result.Examination = core.RecordNumberExaminationGapsFound
		}
		result.Streams = append(result.Streams, stream)
	}
	return result, true
}

// streamOf は 1 つの単位の番号を昇順に並べ、隣り合う 2 つの番号の間の抜けを数える。
func streamOf(key streamKey, numbers []numberedRecord, publication SourcePublication) core.RecordNumberStream {
	slices.SortStableFunc(numbers, func(left, right numberedRecord) int {
		return cmp.Compare(left.number, right.number)
	})
	stream := core.RecordNumberStream{
		RecordCount:  int64(len(numbers)),
		LowestNumber: numbers[0].number, HighestNumber: numbers[len(numbers)-1].number,
		Gaps: []core.RecordNumberGap{}, Computers: computersOf(numbers),
	}
	if key.hasChannel {
		stream.Channel = &key.channel
	}
	for index := 1; index < len(numbers); index++ {
		preceding, following := numbers[index-1], numbers[index]
		if following.number == preceding.number {
			stream.DuplicatedRecordCount++
			continue
		}
		if following.number == preceding.number+1 {
			continue
		}
		stream.MissingNumberCount += following.number - preceding.number - 1
		stream.GapCount++
		if len(stream.Gaps) < core.RecordNumberListLimit {
			stream.Gaps = append(stream.Gaps, gapBetween(preceding, following, publication))
		}
	}
	return stream
}

// gapBetween は番号の隣り合う 2 件の間の抜けを組む。
//
// **失敗を結ぶのは、2 件が収集元のレコードの並びで隣り合うときだけである。** 2 件の間に別の
// チャネルのレコードがあれば、その間の失敗が抜けた番号のレコードであるとは言えない。隣り合う
// 向きは問わない。番号を新しい順に書き出した収集元では、番号の大きいレコードが前にある。
func gapBetween(preceding, following numberedRecord, publication SourcePublication) core.RecordNumberGap {
	before := cloneLocator(publication.records[preceding.at].Locator)
	after := cloneLocator(publication.records[following.at].Locator)
	earlier, later := preceding.at, following.at
	if earlier > later {
		earlier, later = later, earlier
	}
	failures := []core.RecordLocator{}
	if later == earlier+1 {
		failures = failuresBetween(publication.status.Failures,
			publication.records[earlier].Locator, publication.records[later].Locator)
	}
	return core.RecordNumberGap{
		FirstMissingNumber: preceding.number + 1, LastMissingNumber: following.number - 1,
		PrecedingRecordRef: before, FollowingRecordRef: after,
		FailureRecordCount: int64(len(failures)),
		FailureRecordRefs:  failures[:min(len(failures), core.RecordNumberListLimit)],
	}
}

// computersOf は単位のレコードが名乗った Computer ごとの件数を、欄の無いもの、文字列の順に返す。
func computersOf(numbers []numberedRecord) []core.RecordNumberComputer {
	counts := map[streamKey]int64{}
	for _, record := range numbers {
		counts[streamKeyOf(numberingItems{Computer: record.computer})]++
	}
	keys := make([]streamKey, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, compareStreamKeys)
	computers := make([]core.RecordNumberComputer, 0, len(keys))
	for _, key := range keys {
		computer := core.RecordNumberComputer{RecordCount: counts[key]}
		if key.hasComputer {
			computer.Computer = &key.computer
		}
		computers = append(computers, computer)
	}
	return computers
}

// failuresBetween は before の終わりから after の始まりまでの byte 範囲に位置を持つ失敗の
// レコードを返す。位置を確定できなかった失敗は範囲に入らない。
func failuresBetween(failures []core.ImportFailure, before, after core.RecordLocator) []core.RecordLocator {
	refs := []core.RecordLocator{}
	if before.ByteOffset == nil || before.ByteLength == nil || after.ByteOffset == nil {
		return refs
	}
	from, to := *before.ByteOffset+*before.ByteLength, *after.ByteOffset
	for _, failure := range failures {
		if failure.RecordRef == nil || failure.RecordRef.ByteOffset == nil {
			continue
		}
		if at := *failure.RecordRef.ByteOffset; at >= from && at < to {
			refs = append(refs, cloneLocator(*failure.RecordRef))
		}
	}
	return refs
}

// fileHeaderComparisonOf は file の見出しの次のレコード番号を、レコードの見出しの番号の最大の
// 1 つ後ろと比べる。見出しの項目を宣言していない形式では nil を返す。読める番号は
// maxReadableRecordNumber 以下であり、1 つ後ろの番号は uint64 に収まる。
func fileHeaderComparisonOf(
	header []core.RecordField, records []RecordEntry, names RecordNumberingNames,
) *core.RecordNumberFileHeader {
	if names.HeaderNextRecordID == "" {
		return nil
	}
	comparison := &core.RecordNumberFileHeader{
		Comparison: core.RecordNumberHeaderUnreadable, Dirty: clonePointer(headerItemOf(header, names.HeaderDirty)),
	}
	for _, record := range records {
		number, readable := recordNumberOf(numberingItemsOf(record, names).RecordHeaderID)
		if !readable {
			comparison.UnreadableRecordHeaderCount++
			continue
		}
		if comparison.HighestRecordNumber == nil || number > *comparison.HighestRecordNumber {
			comparison.HighestRecordNumber = &number
		}
	}
	next, readable := recordNumberOf(headerItemOf(header, names.HeaderNextRecordID))
	if !readable {
		return comparison
	}
	comparison.NextRecordNumber = &next
	switch highest := comparison.HighestRecordNumber; {
	case highest == nil:
		comparison.Comparison = core.RecordNumberHeaderNoRecordNumbers
	case next == *highest+1:
		comparison.Comparison = core.RecordNumberHeaderAgrees
	default:
		comparison.Comparison = core.RecordNumberHeaderDiffers
	}
	return comparison
}

// comparisonKey は突き合わせの鍵 1 つである。番号の鍵は unit と recordNumber を、時刻の鍵は
// provider と eventID と second (UNIX 時刻の秒) を持つ。
type comparisonKey struct {
	unit              streamKey
	provider, eventID string
	recordNumber      uint64
	second            int64
}

// keyedRecords は 1 つの収集元のレコードを鍵ごとに集めた結果である。
type keyedRecords struct {
	byKey map[comparisonKey][]int
	// unkeyed は鍵の欄を読めなかったレコードの件数、withoutInstant はそのうち時点の定まる
	// 時刻を持たなかったレコードの件数である。
	unkeyed, withoutInstant int64
}

func keyRecords(publication SourcePublication, key core.RecordComparisonKey) keyedRecords {
	keyed := keyedRecords{byKey: map[comparisonKey][]int{}}
	for at, record := range publication.records {
		items := numberingItemsOf(record, publication.parser.RecordNumbering)
		var built comparisonKey
		var readable bool
		if key == core.RecordComparisonKeyRecordNumber {
			built.recordNumber, readable = recordNumberOf(items.EventRecordID)
			built.unit = streamKeyOf(items)
			readable = readable && built.unit.hasChannel
		} else {
			built, readable = secondProviderEventKey(record, items)
			if record.ObservedAt == nil || !hasInstant(*record.ObservedAt) {
				keyed.withoutInstant++
			}
		}
		if !readable {
			keyed.unkeyed++
			continue
		}
		keyed.byKey[built] = append(keyed.byKey[built], at)
	}
	return keyed
}

func hasInstant(timestamp core.Timestamp) bool {
	_, ok := timestamp.Instant()
	return ok
}

// secondProviderEventKey は、時点を秒に切り捨てた値とプロバイダとイベント ID の鍵を組む。
// 時点は原資料のずれか、分析者が記録した時刻の解釈で決まる。
//
// **秒に切り捨てる。** イベントビューアーの CSV は時刻を秒までしか書かず、秒未満を持つ形式の
// 時刻を切り捨てた値が CSV の文字列と同じ秒を指す。
func secondProviderEventKey(record RecordEntry, items numberingItems) (comparisonKey, bool) {
	if record.ObservedAt == nil || items.ProviderName == nil || items.EventID == nil {
		return comparisonKey{}, false
	}
	instant, ok := record.ObservedAt.Instant()
	if !ok {
		return comparisonKey{}, false
	}
	return comparisonKey{
		provider: *items.ProviderName, eventID: *items.EventID, second: instant.Unix(),
	}, true
}

// CompareRecords は 2 つの収集元のレコードを鍵で突き合わせ、片方にしか無い鍵のレコードと、
// 鍵で 1 対 1 に決められない件数を返す。ok が偽になるのは、どちらかの公開された収集元が
// 無いときである。
//
// 両方の形式が EventRecordID を持つときはチャネルと端末と番号を鍵にする。どちらかが持たない
// ときは、時点の秒とプロバイダとイベント ID を鍵にする。**UTC からのずれを推測で補わない。**
// 時点の定まる時刻を持つレコードが片方で 0 件のときは突き合わせない。
func (r ImportResult) CompareRecords(sourceId, comparedSourceId string) (core.RecordComparison, bool) {
	source, sourceFound := r.Publication(sourceId)
	compared, comparedFound := r.Publication(comparedSourceId)
	if !sourceFound || !comparedFound {
		return core.RecordComparison{}, false
	}
	result := core.RecordComparison{
		ComparedSourceId: comparedSourceId, State: core.RecordComparisonNotCompared,
		OnlyInSourceRecordRefs: []core.RecordLocator{}, OnlyInComparedRecordRefs: []core.RecordLocator{},
		OnlyInSourceInsideComparedRangeRecordRefs: []core.RecordLocator{},
		OnlyInComparedInsideSourceRangeRecordRefs: []core.RecordLocator{},
		UnequalKeys: []core.RecordComparisonUnequalKey{}, ListLimit: core.RecordNumberListLimit,
	}
	sourceNames, comparedNames := source.parser.RecordNumbering, compared.parser.RecordNumbering
	if !sourceNames.comparable() || !comparedNames.comparable() {
		result.NotComparedReason = core.RecordNotComparedNotDeclared
		return result, true
	}
	result.Key = core.RecordComparisonKeySecondProviderEvent
	if sourceNames.carriesNumbers() && comparedNames.carriesNumbers() {
		result.Key = core.RecordComparisonKeyRecordNumber
	}
	left, right := keyRecords(source, result.Key), keyRecords(compared, result.Key)
	result.SourceUnkeyedRecordCount, result.ComparedUnkeyedRecordCount = left.unkeyed, right.unkeyed
	if result.Key == core.RecordComparisonKeySecondProviderEvent &&
		((len(left.byKey) == 0 && left.withoutInstant > 0) || (len(right.byKey) == 0 && right.withoutInstant > 0)) {
		result.NotComparedReason = core.RecordNotComparedTimeOffsetUndetermined
		return result, true
	}
	result.State = core.RecordComparisonCompared
	leftRanges, rightRanges := left.ranges(), right.ranges()
	var onlyInSource, onlyInCompared, sourceInside, comparedInside []int
	var unequal [][2][]int
	for key, sourceAt := range left.byKey {
		comparedAt := right.byKey[key]
		if len(comparedAt) == 0 {
			onlyInSource = append(onlyInSource, sourceAt...)
			switch rangeSideOf(key, rightRanges, len(right.byKey)) {
			case outsideRange:
				result.OnlyInSourceOutsideComparedRangeRecordCount += int64(len(sourceAt))
			case insideRange:
				sourceInside = append(sourceInside, sourceAt...)
			}
			continue
		}
		countSharedKey(&result, len(sourceAt), len(comparedAt))
		if len(sourceAt) != len(comparedAt) {
			unequal = append(unequal, [2][]int{sourceAt, comparedAt})
		}
	}
	// 鍵の map を回す順は決まらないため、この収集元の中の最初のレコードの順に並べる。
	slices.SortFunc(unequal, func(left, right [2][]int) int {
		return cmp.Compare(slices.Min(left[0]), slices.Min(right[0]))
	})
	result.UnequalKeys = make([]core.RecordComparisonUnequalKey, 0, min(len(unequal), core.RecordNumberListLimit))
	for _, at := range unequal[:min(len(unequal), core.RecordNumberListLimit)] {
		result.UnequalKeys = append(result.UnequalKeys, resolveUnequalKey(source.records, compared.records, at[0], at[1]))
	}
	for key, comparedAt := range right.byKey {
		if _, found := left.byKey[key]; !found {
			onlyInCompared = append(onlyInCompared, comparedAt...)
			switch rangeSideOf(key, leftRanges, len(left.byKey)) {
			case outsideRange:
				result.OnlyInComparedOutsideSourceRangeRecordCount += int64(len(comparedAt))
			case insideRange:
				comparedInside = append(comparedInside, comparedAt...)
			}
		}
	}
	result.OnlyInSourceRecordCount, result.OnlyInComparedRecordCount = int64(len(onlyInSource)), int64(len(onlyInCompared))
	result.OnlyInSourceRecordRefs = locatorsAt(source.records, onlyInSource)
	result.OnlyInComparedRecordRefs = locatorsAt(compared.records, onlyInCompared)
	result.OnlyInSourceInsideComparedRangeRecordCount = int64(len(sourceInside))
	result.OnlyInComparedInsideSourceRangeRecordCount = int64(len(comparedInside))
	result.OnlyInSourceInsideComparedRangeRecordRefs = locatorsAt(source.records, sourceInside)
	result.OnlyInComparedInsideSourceRangeRecordRefs = locatorsAt(compared.records, comparedInside)
	return result, true
}

// keyRange は、1 つの範囲の単位の鍵が占める位置の最小と最大である。
type keyRange struct{ lowest, highest int64 }

// rangeUnit は鍵の範囲の単位と、単位の中の位置である。番号の鍵ではチャネルと番号、時刻の鍵では
// プロバイダと時点の秒である。**番号の鍵の単位に Computer を入れない。** 番号はチャネルの中で
// 続き、Computer の名前が変わっても振り直さない。
func (k comparisonKey) rangeUnit() (string, int64) {
	if k.unit.hasChannel {
		// 番号は 2^53-1 以下であり (recordNumberOf)、int64 に収まる。
		if k.recordNumber > math.MaxInt64 {
			return "channel\x00" + k.unit.channel, math.MaxInt64
		}
		return "channel\x00" + k.unit.channel, int64(k.recordNumber)
	}
	return "provider\x00" + k.provider, k.second
}

// ranges は、範囲の単位ごとに鍵の位置の最小と最大を返す。
func (k keyedRecords) ranges() map[string]keyRange {
	ranges := make(map[string]keyRange)
	for key := range k.byKey {
		unit, position := key.rangeUnit()
		current, found := ranges[unit]
		if !found {
			ranges[unit] = keyRange{position, position}
			continue
		}
		ranges[unit] = keyRange{min(current.lowest, position), max(current.highest, position)}
	}
	return ranges
}

// rangeSide は、片方にしか無い鍵が相手の範囲のどちらにあるかである。
type rangeSide int

const (
	undeterminedRange rangeSide = iota
	outsideRange
	insideRange
)

// rangeSideOf は鍵が相手の範囲の外か内かを返す。相手が鍵を 1 つも持たないときは決めない。
func rangeSideOf(key comparisonKey, ranges map[string]keyRange, keyCount int) rangeSide {
	if keyCount == 0 {
		return undeterminedRange
	}
	unit, position := key.rangeUnit()
	span, found := ranges[unit]
	if !found || position < span.lowest || position > span.highest {
		return outsideRange
	}
	return insideRange
}

// countSharedKey は、両方の収集元のレコードが持つ鍵 1 つを、1 対 1 の鍵か、決められない鍵として数える。
func countSharedKey(result *core.RecordComparison, sourceCount, comparedCount int) {
	if sourceCount == 1 && comparedCount == 1 {
		result.OneToOneKeyCount++
		return
	}
	result.UndeterminedKeyCount++
	result.UndeterminedSourceRecordCount += int64(sourceCount)
	result.UndeterminedComparedRecordCount += int64(comparedCount)
	surplus := int64(sourceCount - comparedCount)
	if surplus != 0 {
		result.UnequalKeyCount++
	}
	result.SourceSurplusRecordCount += max(surplus, 0)
	result.ComparedSurplusRecordCount += max(-surplus, 0)
}

// resolveUnequalKey は件数の違う鍵 1 つのレコードを、鍵に入れていない語彙の項目の値で対にし、
// 対にならないレコードを返す (core.RecordComparisonUnequalKey)。
func resolveUnequalKey(
	sourceRecords, comparedRecords []RecordEntry, sourceAt, comparedAt []int,
) core.RecordComparisonUnequalKey {
	slices.Sort(sourceAt)
	slices.Sort(comparedAt)
	sourceValues, comparedValues := semanticValuesAt(sourceRecords, sourceAt), semanticValuesAt(comparedRecords, comparedAt)
	semantics := agreeingSemantics(sourceValues, comparedValues)
	resolved := core.RecordComparisonUnequalKey{
		Outcome: core.UnequalKeyNoComparedValues, ComparedSemantics: semantics,
		SourceRecordRefs:       locatorsAt(sourceRecords, slices.Clone(sourceAt)),
		ComparedRecordRefs:     locatorsAt(comparedRecords, slices.Clone(comparedAt)),
		OnlyInSourceRecordRefs: []core.RecordLocator{}, OnlyInComparedRecordRefs: []core.RecordLocator{},
	}
	if len(semantics) == 0 {
		resolved.ComparedSemantics = []core.SemanticKey{}
		return resolved
	}
	sourceSignatures, comparedSignatures := signaturesOf(sourceValues, semantics), signaturesOf(comparedValues, semantics)
	unpairedSource, unpairedCompared := unpaired(sourceSignatures, comparedSignatures)
	resolved.OnlyInSourceRecordRefs = locatorsAt(sourceRecords, pick(sourceAt, unpairedSource))
	resolved.OnlyInComparedRecordRefs = locatorsAt(comparedRecords, pick(comparedAt, unpairedCompared))
	resolved.Outcome = core.UnequalKeyUndecided
	if len(unpairedSource) == 0 && len(unpairedCompared) == len(comparedAt)-len(sourceAt) &&
		distinctWithin(comparedSignatures, unpairedCompared) ||
		len(unpairedCompared) == 0 && len(unpairedSource) == len(sourceAt)-len(comparedAt) &&
			distinctWithin(sourceSignatures, unpairedSource) {
		resolved.Outcome = core.UnequalKeyIdentified
	}
	return resolved
}

// semanticValuesAt は、位置 at のレコードごとに、語彙の項目から比べる文字列への表を返す。同じ
// 項目を 2 つ以上持つレコードは、文字列を欄の並びの順に NUL でつないだ値を持つ。
func semanticValuesAt(records []RecordEntry, at []int) []map[core.SemanticKey]string {
	values := make([]map[core.SemanticKey]string, 0, len(at))
	for _, index := range at {
		fields := map[core.SemanticKey]string{}
		if semantics := records[index].Semantics; semantics != nil {
			for _, field := range semantics.Fields {
				if field.Semantic == "" || field.Text == nil {
					continue
				}
				text, readable := field.Text.ComparableValue()
				if !readable {
					continue
				}
				if previous, found := fields[field.Semantic]; found {
					text = previous + "\x00" + text
				}
				fields[field.Semantic] = text
			}
		}
		values = append(values, fields)
	}
	return values
}

// agreeingSemantics は、両側の全レコードが持ち、少ない側の値の多重集合が多い側の値の多重集合に
// 含まれる語彙の項目を、文字列の順に返す。
func agreeingSemantics(source, compared []map[core.SemanticKey]string) []core.SemanticKey {
	fewer, more := source, compared
	if len(fewer) > len(more) {
		fewer, more = more, fewer
	}
	var semantics []core.SemanticKey
	for semantic := range fewer[0] {
		counts := map[string]int{}
		held := true
		for _, record := range more {
			value, found := record[semantic]
			held = held && found
			counts[value]++
		}
		for _, record := range fewer {
			value, found := record[semantic]
			held = held && found && counts[value] > 0
			counts[value]--
		}
		if held {
			semantics = append(semantics, semantic)
		}
	}
	slices.Sort(semantics)
	return semantics
}

// signaturesOf はレコードごとに、semantics の値を NUL の 2 つでつないだ文字列を返す。
func signaturesOf(values []map[core.SemanticKey]string, semantics []core.SemanticKey) []string {
	signatures := make([]string, len(values))
	for index, record := range values {
		parts := make([]string, len(semantics))
		for at, semantic := range semantics {
			parts[at] = record[semantic]
		}
		signatures[index] = strings.Join(parts, "\x00\x00")
	}
	return signatures
}

// unpaired は同じ文字列どうしを先頭から対にし、対にならない要素の添字を両側について返す。
func unpaired(source, compared []string) ([]int, []int) {
	waiting := map[string][]int{}
	for index, signature := range compared {
		waiting[signature] = append(waiting[signature], index)
	}
	var unpairedSource []int
	for index, signature := range source {
		if len(waiting[signature]) == 0 {
			unpairedSource = append(unpairedSource, index)
			continue
		}
		waiting[signature] = waiting[signature][1:]
	}
	var unpairedCompared []int
	for _, indexes := range waiting {
		unpairedCompared = append(unpairedCompared, indexes...)
	}
	slices.Sort(unpairedCompared)
	return unpairedSource, unpairedCompared
}

// distinctWithin は、chosen の添字の文字列が signatures の中で 1 回だけ出るかを返す。
func distinctWithin(signatures []string, chosen []int) bool {
	counts := map[string]int{}
	for _, signature := range signatures {
		counts[signature]++
	}
	for _, index := range chosen {
		if counts[signatures[index]] != 1 {
			return false
		}
	}
	return true
}

// pick は at の中から indexes の添字の要素を返す。
func pick(at, indexes []int) []int {
	picked := make([]int, len(indexes))
	for position, index := range indexes {
		picked[position] = at[index]
	}
	return picked
}

// locatorsAt はレコードの並びの位置 at のレコードの位置を、収集元の中の順に先頭の上限の件数まで返す。
func locatorsAt(records []RecordEntry, at []int) []core.RecordLocator {
	slices.Sort(at)
	at = at[:min(len(at), core.RecordNumberListLimit)]
	refs := make([]core.RecordLocator, 0, len(at))
	for _, index := range at {
		refs = append(refs, cloneLocator(records[index].Locator))
	}
	return refs
}
