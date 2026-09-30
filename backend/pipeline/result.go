package pipeline

import (
	"fmt"
	"reflect"
	"slices"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// ImportResult は収集元の順序を保つ確定済みの取り込み結果である。
type ImportResult struct {
	publications   []SourcePublication
	identities     map[string]core.SourceIdentity
	rawTexts       rawTextIndex
	analysisRunRef string
	// analystAssignments は分析者が与えた端末の割当である。取り込みの外から来る値で
	// あり、原資料と自動導出を書き換えない。
	analystAssignments []core.TerminalAssignment
	// importAssignments は利用者が取り込みの起動で収集元ごとに指定した端末の割当である。
	importAssignments []core.TerminalAssignment
	// importOffsets は、利用者が取り込みの起動で端末と一緒に指定した UTC からのずれを sourceId で
	// 探す。分析者が画面で記録した時刻の解釈を持たない収集元の時刻を、このずれで読む
	// (withTimeInterpretations)。
	importOffsets map[string]core.UtcOffset
	// interpretedRanges は、分析者の時刻の解釈で読んだ収録範囲を sourceId で探す。
	// 収集元の識別 (identities) の収録範囲は原資料の文字列から定まる値のまま保つ。
	interpretedRanges map[string]core.TimeRange
	// sourceInterpretations は、分析者が収集元に与えた時刻の解釈を sourceId で探す。
	// 地方時の期間を持つ利用者の割当を、期間を読み取った収集元の解釈で読む
	// (withInterpretedRange)。
	sourceInterpretations map[string]core.TimestampInterpretation
	// skippedFiles は、収集の directory にあり取り込まなかった file である (ExpandPlans)。
	skippedFiles []core.SkippedFile
}

// WithSkippedFiles は、収集の directory にあり取り込まなかった file を持たせた取り込み結果を返す。
func (r ImportResult) WithSkippedFiles(files []core.SkippedFile) ImportResult {
	r.skippedFiles = slices.Clone(files)
	return r
}

// SkippedFiles は、収集の directory にあり取り込まなかった file を、展開した順で返す。
func (r ImportResult) SkippedFiles() []core.SkippedFile {
	return slices.Clone(r.skippedFiles)
}

// ImportTimeOffset は、利用者が取り込みの起動で収集元に指定した UTC からのずれを返す。ok が偽に
// なるのは、ずれを指定していない収集元である。分析者が画面で解釈を記録した収集元でも、起動で
// 指定した値を返す。
func (r ImportResult) ImportTimeOffset(sourceId string) (core.UtcOffset, bool) {
	offset, specified := r.importOffsets[sourceId]
	return offset, specified
}

// ImportSpecifiedTerminalAssignments は、取り込みの起動で指定した端末の割当を、収集元の
// 取り込みの入力順で返す。
func (r ImportResult) ImportSpecifiedTerminalAssignments() []core.TerminalAssignment {
	return cloneAssignments(r.importAssignments)
}

// userAssignments は利用者が入力した割当を、取り込みの指定、分析者の記録の順で返す。
func (r ImportResult) userAssignments() []core.TerminalAssignment {
	return append(slices.Clone(r.importAssignments), r.analystAssignments...)
}

// WithAnalystTerminalAssignments は、分析者が与えた端末の割当を足した取り込み結果を返す。
//
// **原資料と自動導出を書き換えない。** 割当は端末の判定に足す材料であり、レコードにも
// 収集元の識別にも触れない。返した結果の割当の集合は元の結果と別の領域にある。
func (r ImportResult) WithAnalystTerminalAssignments(
	assignments []core.TerminalAssignment,
) ImportResult {
	r.analystAssignments = cloneAssignments(assignments)
	return r
}

// AnalystTerminalAssignments は分析者が与えた端末の割当を返す。
func (r ImportResult) AnalystTerminalAssignments() []core.TerminalAssignment {
	return cloneAssignments(r.analystAssignments)
}

// AnalysisRunRef は取り込み全体を覆う解析実行への参照を返す。
// 収集元が 0 件の取り込み結果でも 1 値に決まる。
func (r ImportResult) AnalysisRunRef() string {
	return r.analysisRunRef
}

// ImportStatusRef は収集元 1 件の取り込みの状態を指す参照を返す。
// ok が偽になるのは、その sourceId の収集元がこの取り込み結果に無いときである。
//
// 参照は解析実行と収集元の組を指す。収集元の一覧が変わることは解析実行を新しくするため、
// 同じ値が別の取り込みの状態を指すことが無い。
func (r ImportResult) ImportStatusRef(sourceId string) (string, bool) {
	for _, publication := range r.publications {
		if publication.status.SourceId == sourceId {
			return "status:" + identityDigest([]string{r.analysisRunRef, sourceId}), true
		}
	}
	return "", false
}

// SourceEntry は 1 件の収集元の識別と取り込み状態の組である。
type SourceEntry struct {
	Identity core.SourceIdentity
	Status   core.ImportStatus
}

// SourceEntries は取り込み結果の入力順で、収集元の識別と取り込み状態を対で返す。
// 対応する識別が無い収集元は内部不変条件の破れであり、error を返す。
func (r ImportResult) SourceEntries() ([]SourceEntry, error) {
	entries := make([]SourceEntry, 0, len(r.publications))
	for _, publication := range r.publications {
		status := publication.Status()
		identity, ok := r.Identity(status.SourceId)
		if !ok {
			return nil, fmt.Errorf("listing sources: source %q has no identity", status.SourceId)
		}
		entries = append(entries, SourceEntry{Identity: identity, Status: status})
	}
	return entries, nil
}

// RawText は公開可能な収集元の参照が指すレコードの原文を返す。
func (r ImportResult) RawText(ref string) (string, bool) {
	entry, ok := r.rawTexts.texts[ref]
	if !ok {
		return "", false
	}
	if _, ok := r.Publication(entry.sourceId); !ok {
		return "", false
	}
	return entry.text, true
}

// ConvertedFieldTexts は、原文を収集元の byte 列から組み立てた収集元
// (ParserIdentity.RawTextConverted) の公開されたレコードのうち、原文の参照が refs にあるものに
// ついて、欄の名前と、欄の原資料の文字列と正規化値を、原文の参照ごとに返す。
//
// ponytail: 呼ぶたびに変換した収集元の全レコードを 1 回走査する。遅くなったら参照から欄への
// 索引を取り込みの時点で組む。
func (r ImportResult) ConvertedFieldTexts(refs map[string]bool) map[string][]string {
	texts := map[string][]string{}
	for _, publication := range r.publications {
		if !publication.parser.RawTextConverted ||
			publication.status.PublicationState == core.PublicationStateWithheld {
			continue
		}
		for _, record := range publication.records {
			ref := record.Locator.RecordRawTextRef
			if record.Semantics == nil || !refs[ref] {
				continue
			}
			for _, field := range record.Semantics.Fields {
				texts[ref] = append(texts[ref], field.Name)
				if raw, present := fieldRawText(field); present {
					texts[ref] = append(texts[ref], raw)
				}
				if normalized, present := fieldNormalized(field); present {
					texts[ref] = append(texts[ref], normalized)
				}
			}
		}
	}
	return texts
}

// Identity は収集元 1 件の識別を複製して返す。
func (r ImportResult) Identity(sourceId string) (core.SourceIdentity, bool) {
	identity, ok := r.identities[sourceId]
	if !ok {
		return core.SourceIdentity{}, false
	}
	identity.RecordCount = clonePointer(identity.RecordCount)
	identity.FormatVersion = clonePointer(identity.FormatVersion)
	identity.FormatSpec = clonePointer(identity.FormatSpec)
	identity.CaseId = clonePointer(identity.CaseId)
	identity.ObservedRangeFirst = cloneTimestampPointer(identity.ObservedRangeFirst)
	identity.ObservedRangeLast = cloneTimestampPointer(identity.ObservedRangeLast)
	identity.MessageUnrenderedCount = clonePointer(identity.MessageUnrenderedCount)
	identity.TerminalCandidates = slices.Clone(identity.TerminalCandidates)
	identity.FileHeader = cloneRecordFields(identity.FileHeader)
	identity.Members = slices.Clone(identity.Members)
	return identity, true
}

type rawTextKey struct{ sourceId, hash, position string }
type indexedRawText struct{ sourceId, text string }
type rawTextIndex struct {
	references map[rawTextKey]string
	texts      map[string]indexedRawText
}

func (index rawTextIndex) add(locator core.RecordLocator, text string) string {
	key := rawTextKey{locator.SourceId, locator.SourceContentSha256, locatorPositionKey(locator)}
	if ref, ok := index.references[key]; ok {
		return ref
	}
	ref := "raw:" + identityDigest([]string{key.sourceId, key.hash, key.position})
	index.references[key] = ref
	index.texts[ref] = indexedRawText{sourceId: locator.SourceId, text: text}
	return ref
}

// SourcePublication は公開状態と診断と件数を伴う収集元のレコードである。
type SourcePublication struct {
	status  core.ImportStatus
	records []RecordEntry
	parser  ParserIdentity
	// localRange は、UTC からのずれの決まらない地方時の文字列の最も早い値と最も遅い値である。
	// 母集団は収録範囲と同じく解析に失敗したレコードを含む。地方時を持たない収集元では nil である。
	// 分析者が時刻の解釈を記録すると、両端にそのずれを与えた値が収録範囲になる。
	localRange *core.TimeRange
	// messageUnrendered は、説明を組めなかったレコードの records の中の位置である。
	messageUnrendered []int
	// terminalNamings は、レコードが記録した端末自身の名前である。位置は records の中の位置で
	// ある。公開を止めた収集元では nil である。
	terminalNamings []recordTerminalNaming
}

// MessageUnrenderedRecordRefs は、公開された収集元 1 件のうち、説明を組めなかったレコードの
// 位置を、収集元の中の順に先頭の limit 件まで返す。ok が偽になるのは、sourceId の公開された
// 収集元が無いときである。
func (r ImportResult) MessageUnrenderedRecordRefs(sourceId string, limit int) ([]core.RecordLocator, bool) {
	publication, found := r.Publication(sourceId)
	if !found {
		return nil, false
	}
	at := publication.messageUnrendered[:min(len(publication.messageUnrendered), limit)]
	refs := make([]core.RecordLocator, 0, len(at))
	for _, index := range at {
		refs = append(refs, cloneLocator(publication.records[index].Locator))
	}
	return refs, true
}

// ItemSemantics は、この収集元のレコードが運びうる語彙の項目を返す。
//
// 関連付けが「相手の入力形式にその欄が無い」と言えるかの判定に用いる。**レコードの項目の
// 集合から導かない** (ParserIdentity.ItemSemantics)。
func (p SourcePublication) ItemSemantics() []core.SemanticKey {
	return slices.Clone(p.parser.ItemSemantics)
}

// TimePrecision は、この収集元のレコードの時刻の精度を返す。
func (p SourcePublication) TimePrecision() core.Precision {
	return p.parser.TimePrecision
}

// ConnectionRequestKinds は、この収集元で外向きの通信の要求を記録した観測の種別を返す。
//
// 関連付けが候補として数えるレコードの母集合を決めるのに用いる
// (ParserIdentity.ConnectionRequestKinds)。
func (p SourcePublication) ConnectionRequestKinds() []core.ObservationKindSelector {
	return slices.Clone(p.parser.ConnectionRequestKinds)
}

// connectionPhaseOf は、この収集元の観測の種別 kind が接続を開いた記録か閉じた記録かを返す
// (ParserIdentity.ConnectionOpenKinds、ConnectionCloseKinds)。
func (p SourcePublication) connectionPhaseOf(kind core.ObservationKind) connectionPhase {
	matches := func(selector core.ObservationKindSelector) bool { return kind.Matches(selector) }
	switch {
	case slices.ContainsFunc(p.parser.ConnectionOpenKinds, matches):
		return connectionPhaseOpen
	case slices.ContainsFunc(p.parser.ConnectionCloseKinds, matches):
		return connectionPhaseClose
	default:
		return connectionPhaseUnknown
	}
}

// requestStatusOf は、レコードが記録した要求処理の結果の欄を指して返す
// (ParserIdentity.RequestStatusItems)。欄は語彙の項目を持たないため、欄の名前で探す。
// 欄を持たないレコードでは nil を返す。取り込み結果の項目を複製せずに指す。
func (p SourcePublication) requestStatusOf(record RecordEntry) *core.RecordField {
	if len(p.parser.RequestStatusItems) == 0 || record.Semantics == nil {
		return nil
	}
	fields := record.Semantics.Fields
	if at := slices.IndexFunc(fields, func(field core.RecordField) bool {
		return field.Semantic == "" && slices.Contains(p.parser.RequestStatusItems, field.Name)
	}); at >= 0 {
		return &fields[at]
	}
	return nil
}

// replacesContent は、この収集元の観測の種別 kind と項目 fields のレコードが、対象の内容を
// 置き換える記録かを返す (ParserIdentity.ContentReplacementKinds)。欄の条件は、名前が一致する
// 最初の項目の原資料の文字列と比べる。その名前の項目を持たないレコードと、項目が原資料の文字列を
// 持たないレコードは条件に一致しない。
func (p SourcePublication) replacesContent(kind core.ObservationKind, fields []core.RecordField) bool {
	return slices.ContainsFunc(p.parser.ContentReplacementKinds, func(selector core.ContentReplacementSelector) bool {
		if !kind.Matches(selector.Kind) {
			return false
		}
		for _, item := range selector.Fields {
			at := slices.IndexFunc(fields, func(field core.RecordField) bool { return field.Name == item.Name })
			if at < 0 || fields[at].Text == nil {
				return false
			}
			if raw, present := fields[at].Text.RawTextValue(); !present || raw != item.Value {
				return false
			}
		}
		return true
	})
}

// flowOperation は、この収集元の観測の種別 kind のレコードの操作の分類を返す
// (ParserIdentity.FlowOperationKinds)。宣言が該当しない種別では空の文字列である。
func (p SourcePublication) flowOperation(kind core.ObservationKind) core.FlowOperation {
	at := slices.IndexFunc(p.parser.FlowOperationKinds, func(selector core.FlowOperationSelector) bool {
		return kind.Matches(selector.Kind)
	})
	if at < 0 {
		return ""
	}
	return p.parser.FlowOperationKinds[at].Operation
}

// ConnectionMatchConditions は、この収集元が関連付けの条件へ差し出す宣言を返す。
//
// 段階の条件の一覧は、起点の側の収集元の宣言と候補の側の収集元の宣言から組む
// (ParserIdentity.ConnectionMatchConditions)。
func (p SourcePublication) ConnectionMatchConditions() []ConnectionMatchCondition {
	return slices.Clone(p.parser.ConnectionMatchConditions)
}

// TranscriptIdentityItems は、同じ事象を 2 回転記したレコードを見分ける欄の名前を返す。
//
// グラフが関係の根拠を数えるときに、転記のうち 1 件だけを数えるのに用いる
// (ParserIdentity.TranscriptIdentityItems)。
func (p SourcePublication) TranscriptIdentityItems() []string {
	return slices.Clone(p.parser.TranscriptIdentityItems)
}

// Publication は公開できる収集元の結果を返す。
func (r ImportResult) Publication(sourceId string) (SourcePublication, bool) {
	for _, publication := range r.publications {
		if publication.status.SourceId == sourceId &&
			publication.status.PublicationState != core.PublicationStateWithheld {
			return publication, true
		}
	}
	return SourcePublication{}, false
}

// Statuses は収集元の入力順で診断と件数と公開状態を返す。
func (r ImportResult) Statuses() []core.ImportStatus {
	statuses := make([]core.ImportStatus, len(r.publications))
	for i, publication := range r.publications {
		statuses[i] = publication.Status()
	}
	return statuses
}

// Status は収集元の診断と件数と公開状態を返す。
func (p SourcePublication) Status() core.ImportStatus {
	return cloneStatus(p.status)
}

// Records は公開されたレコードを複製して返す。
func (p SourcePublication) Records() []RecordEntry {
	return cloneRecords(p.records)
}

func newImportResult(sources []scannedSource, statuses []core.ImportStatus, runRef string,
	rawTextRef func(core.RecordLocator, string) string,
) (ImportResult, error) {
	if len(sources) != len(statuses) || runRef == "" {
		return ImportResult{}, fmt.Errorf("settling import result: matching source and status lengths and a run reference are required")
	}
	assigned := make([]string, len(statuses))
	for i, status := range statuses {
		if err := checkStatusSource(sources[i], status); err != nil {
			return ImportResult{}, fmt.Errorf("settling source %d: %w", i, err)
		}
		assigned[i] = status.SourceId
	}
	broken, err := checkIdentifierCollision(sources, assigned)
	if err != nil {
		return ImportResult{}, fmt.Errorf("settling import result: %w", err)
	}
	broken = append(broken, checkAnalysisRun(statuses, runRef)...)
	broken = append(broken, checkEvidenceReferences(sources, statuses)...)
	result := ImportResult{
		publications:   make([]SourcePublication, len(sources)),
		analysisRunRef: runRef,
	}
	// 文字列の値の共有は、公開する全収集元のレコードにまたがる。
	texts := make(textInterner)
	for i, inputStatus := range statuses {
		source := sources[i]
		var applicable []invariantBreak
		for _, problem := range broken {
			if problem.runWide || problem.sourceIndex == i {
				applicable = append(applicable, problem)
			}
		}
		publication, err := completeSourcePublication(source, inputStatus, runRef, applicable, rawTextRef, texts)
		if err != nil {
			return ImportResult{}, fmt.Errorf("settling publication %d: %w", i, err)
		}
		result.publications[i] = publication
	}
	return result, nil
}

func completeSourcePublication(source scannedSource, inputStatus core.ImportStatus, runRef string,
	broken []invariantBreak, rawTextRef func(core.RecordLocator, string) string, texts textInterner,
) (SourcePublication, error) {
	status := cloneStatus(inputStatus)
	status.PublicationState, status.WithheldReason = decidePublicationState(source, broken)
	status.AnalysisRunRef = runRef
	if err := status.Validate(); err != nil {
		return SourcePublication{}, fmt.Errorf("validating settled status: %w", err)
	}
	publication := SourcePublication{
		status: status, parser: source.Parser, localRange: localRangeOf(source),
		messageUnrendered: slices.Clone(source.MessageUnrenderedRecords),
	}
	if status.PublicationState == core.PublicationStateWithheld {
		return publication, nil
	}
	publication.records = texts.records(source.Records)
	publication.terminalNamings = slices.Clone(source.TerminalNamings)
	for i := range publication.records {
		locator, err := completeRecordLocator(publication.records[i].Locator, status.SourceId, status.Scope.SourceContentSha256, publication.records[i].RawText, rawTextRef)
		if err != nil {
			return SourcePublication{}, fmt.Errorf("completing published record %d: %w", i, err)
		}
		if locator.SourceFileName != source.Plan.FileName || !matchesParserPosition(locator, source.Parser) {
			return SourcePublication{}, fmt.Errorf("published locator file name or position kind differs from source plan and parser")
		}
		publication.records[i].Locator = locator
		if err := completeRecordProcessRef(publication.records[i].Semantics, status.SourceId); err != nil {
			return SourcePublication{}, fmt.Errorf("completing published record %d: %w", i, err)
		}
	}
	return publication, nil
}

// completeRecordProcessRef は意味付けの結果が持つプロセスの参照に、識別子を発行した後に
// 決まる sourceId を足し、全項目が揃ったことを確かめる。
func completeRecordProcessRef(semantics *RecordSemantics, sourceId string) error {
	if semantics == nil || semantics.ProcessRef == nil {
		return nil
	}
	semantics.ProcessRef.SourceId = sourceId
	if err := semantics.ProcessRef.Validate(); err != nil {
		return fmt.Errorf("validating the settled process reference: %w", err)
	}
	return nil
}

// matchesParserPosition は、公開するレコードの位置の指し方がパーサーの名乗りと一致するかを
// 返す。
//
// **公開するレコードだけを見る。** 失敗の recordRef は本関数を通らない。sn を読めない
// markii 形式のレコードは stage が field_map の失敗になり、その recordRef が行番号で位置を
// 指す。
//
// 通番を名乗るパーサーが行番号のレコードを公開する組み合わせを受け付けない。受け付けると、
// sequenceNumber を持たないレコードが通番の収集元として公開され、通番で探す要求から
// 外れる。
func matchesParserPosition(locator core.RecordLocator, parser ParserIdentity) bool {
	return locator.PositionKind == parser.PositionKind
}

func checkStatusSource(source scannedSource, status core.ImportStatus) error {
	scope := source.Scope
	scope.SourceId = status.SourceId
	scope.SourceContentSha256 = source.Measurement.ContentSha256
	if !reflect.DeepEqual(scope, status.Scope) ||
		!reflect.DeepEqual(source.Counts, status.Counts) ||
		!slices.Equal(source.DiagnosisCounts, status.DiagnosisCounts) ||
		source.FailureCount != status.FailureCount || len(source.Failures) != len(status.Failures) {
		return fmt.Errorf("source measurements, scope or counts differ from status")
	}
	for i, failed := range source.Failures {
		failure := cloneFailure(failed.Failure)
		completed := status.Failures[i]
		failure.SourceId = completed.SourceId
		failure.SourceContentSha256 = completed.SourceContentSha256
		failure.ParserVersion = completed.ParserVersion
		failure.SanitizedMessage = completed.SanitizedMessage
		if failure.RecordRef != nil {
			failure.RawTextRef = completed.RawTextRef
			if failure.RecordRef.SourceId == "" {
				failure.RecordRef.SourceId = status.SourceId
			}
			if failure.RecordRef.SourceContentSha256 == "" {
				failure.RecordRef.SourceContentSha256 = source.Measurement.ContentSha256
			}
			if completed.RecordRef != nil {
				failure.RecordRef.RecordRawTextRef = completed.RecordRef.RecordRawTextRef
			}
		}
		if !reflect.DeepEqual(failure, completed) {
			return fmt.Errorf("source failure differs from completed status")
		}
	}
	return nil
}

func clonePointer[T any](value *T) *T {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func cloneLocator(locator core.RecordLocator) core.RecordLocator {
	locator.SequenceNumber = clonePointer(locator.SequenceNumber)
	locator.LineNumber = clonePointer(locator.LineNumber)
	locator.ByteOffset = clonePointer(locator.ByteOffset)
	locator.ByteLength = clonePointer(locator.ByteLength)
	locator.LineCount = clonePointer(locator.LineCount)
	return locator
}

func cloneFailure(failure core.ImportFailure) core.ImportFailure {
	failure.LineNumber = clonePointer(failure.LineNumber)
	failure.ByteOffset = clonePointer(failure.ByteOffset)
	if failure.RecordRef != nil {
		locator := cloneLocator(*failure.RecordRef)
		failure.RecordRef = &locator
	}
	return failure
}

func cloneStatus(status core.ImportStatus) core.ImportStatus {
	status.Scope.FromPosition = clonePointer(status.Scope.FromPosition)
	status.Scope.ToPosition = clonePointer(status.Scope.ToPosition)
	status.DiagnosisCounts = slices.Clone(status.DiagnosisCounts)
	status.Failures = slices.Clone(status.Failures)
	for i := range status.Failures {
		status.Failures[i] = cloneFailure(status.Failures[i])
	}
	return status
}

func cloneRecords(records []RecordEntry) []RecordEntry {
	cloned := slices.Clone(records)
	for i := range cloned {
		cloned[i].Locator = cloneLocator(cloned[i].Locator)
		cloned[i].ObservedAt = cloneTimestampPointer(cloned[i].ObservedAt)
		cloned[i].Semantics = cloneSemantics(cloned[i].Semantics)
		cloned[i].Terminal = cloneRecordFields(cloned[i].Terminal)
	}
	return cloned
}
