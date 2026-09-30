package pipeline

import (
	"errors"
	"fmt"
	"io"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// scanSource はレコードと診断を収集し、位置形式が混在する収集元の scope を whole_source にする。
func scanSource(parser SourceParser, input io.Reader, m sourceMeasurement, plan SourcePlan) (scannedSource, error) {
	if parser == nil || input == nil {
		return scannedSource{}, fmt.Errorf("scanning a source requires a parser and input")
	}
	identity := parser.Identity()
	if identity.FormatKey != plan.FormatKey {
		return scannedSource{}, fmt.Errorf("scanning a source with a parser for another format")
	}
	// **宣言の不備を、レコードを 1 件も解析しないうちに止める。** 解析した後で止めると、
	// adapter の宣言の欠陥がレコードについての診断に混ざる (ParserIdentity.Validate)。
	// 収集元の byte は呼び出し元が既に読み、内容の識別を取っている (Runner.scan)。
	if problem := identity.Validate(); problem != nil {
		return scannedSource{}, problem
	}
	source := scannedSource{
		Plan: plan, Measurement: m, Parser: identity,
		Scope: core.RecordRange{SourceContentSha256: m.ContentSha256},
	}
	reader := &scanByteReader{input: input}
	parser.Reset(reader)
	state := sourceScanState{positionKind: identity.PositionKind}
	var scanErr error
	for {
		record, failure, err := parser.Next()
		if errors.Is(err, io.EOF) && failure == nil {
			break
		}
		if problem := state.processRecord(&source, record, failure, err, reader.bytes); problem != nil {
			return source, fmt.Errorf("processing a scanned record: %w", problem)
		}
		if err != nil {
			source.ReadStopped = true
			scanErr = fmt.Errorf("scanning source records: %w", err)
			break
		}
	}
	if header, ok := parser.(SourceHeaderReader); ok {
		source.FileHeader = cloneRecordFields(header.SourceHeader())
	}
	if failure := scanSizeFailure(m.SizeBytes, reader.bytes); failure != nil {
		failure.SourceContentSha256 = m.ContentSha256
		source.Failures = append(source.Failures, failedRecord{Failure: *failure})
		scanErr = errors.Join(scanErr, fmt.Errorf("scanning source bytes: measured %d, read %d", m.SizeBytes, reader.bytes))
	}
	state.bounds.apply(&source.Scope, state.positionKind, source.ReadStopped || state.mixedPositions)
	if err := finishScanCounts(&source, state.succeeded, state.failed); err != nil {
		return source, fmt.Errorf("finishing the source scan: %w", err)
	}
	return source, scanErr
}

type sourceScanState struct {
	bounds         scanBounds
	succeeded      int64
	failed         int64
	positionKind   core.PositionKind
	mixedPositions bool
}

func (s *sourceScanState) processRecord(source *scannedSource, record ParsedRecord, failure *core.ImportFailure, readErr error, bytesRead int64) error {
	var locator *core.RecordLocator
	if readErr == nil && (failure == nil || failure.Stage != core.FailureStageRead) {
		locator = scannedRecordLocator(record, source.Parser, source.Measurement, source.Plan)
		s.observeLocator(locator)
	}
	if failure != nil {
		copied := *failure
		copied.SourceContentSha256 = source.Measurement.ContentSha256
		copied.RecordRef = withLineBytes(locator, record)
		// 途中で切れた行は、読めた欄を持つレコードとしても取り込む。件数は失敗に数える。
		kept := locator != nil && failure.RecordTruncated && record.Semantics != nil
		failed := failedRecord{Failure: copied, Kept: kept}
		if locator != nil {
			failed.RawText = clonePointer(&record.RawText)
			failed.ObservedAt = cloneTimestampPointer(record.ObservedAt)
			s.failed++
		}
		source.Failures = append(source.Failures, failed)
		if kept {
			source.Records = append(source.Records, RecordEntry{
				Locator: *locator, RawText: record.RawText, ObservedAt: record.ObservedAt,
				Semantics: scannedSemantics(record.Semantics, source.Measurement.ContentSha256),
				Terminal:  cloneRecordFields(record.Terminal),
			})
		}
		return nil
	}
	if readErr != nil {
		source.Failures = append(source.Failures, failedRecord{Failure: scanReadFailure(bytesRead, source.Measurement.ContentSha256, readErr)})
		return nil
	}
	if locator == nil {
		return fmt.Errorf("scanning a successful record without a position")
	}
	source.Records = append(source.Records, RecordEntry{
		Locator: *locator, RawText: record.RawText, ObservedAt: record.ObservedAt,
		Semantics: scannedSemantics(record.Semantics, source.Measurement.ContentSha256),
		Terminal:  cloneRecordFields(record.Terminal),
	})
	s.succeeded++
	if record.MessageUnrendered {
		source.MessageUnrenderedCount++
		source.MessageUnrenderedRecords = append(source.MessageUnrenderedRecords, len(source.Records)-1)
	}
	for _, candidate := range record.TerminalCandidates {
		if source.TerminalCandidates == nil {
			source.TerminalCandidates = make(map[string]int64)
		}
		source.TerminalCandidates[candidate]++
	}
	for _, naming := range record.TerminalNamings {
		source.TerminalNamings = append(source.TerminalNamings,
			recordTerminalNaming{record: len(source.Records) - 1, naming: naming})
	}
	return nil
}

// scannedSemantics は意味付けの結果に、走査の時点で決まる収集元の内容の識別を足す。
// SourceId は識別子を発行した後の段階が埋める。
func scannedSemantics(semantics *RecordSemantics, contentSha256 string) *RecordSemantics {
	scanned := cloneSemantics(semantics)
	if scanned != nil && scanned.ProcessRef != nil {
		scanned.ProcessRef.SourceContentSha256 = contentSha256
	}
	return scanned
}

func (s *sourceScanState) observeLocator(locator *core.RecordLocator) {
	if locator != nil && !s.bounds.seen {
		s.positionKind = locator.PositionKind
	}
	if locator != nil {
		s.mixedPositions = s.mixedPositions || s.positionKind != locator.PositionKind
	}
	s.bounds.observe(locatorPosition(locator))
}

// scannedRecordLocator はレコードの位置を組む。位置を 1 つも持たないレコードでは nil を返す。
//
// 行を持たない形式 (バイナリの file) のレコードは、行番号を持たず byte 範囲だけで指す。
func scannedRecordLocator(record ParsedRecord, identity ParserIdentity, m sourceMeasurement, plan SourcePlan) *core.RecordLocator {
	locator := &core.RecordLocator{
		SourceContentSha256: m.ContentSha256, SourceFileName: plan.FileName,
		LineCount: clonePointer(record.LineCount),
	}
	if record.LineNumber >= 1 {
		line := record.LineNumber
		locator.PositionKind, locator.LineNumber = core.PositionKindLineNumber, &line
	}
	// 宣言した指し方の位置を読めなかったレコードは、行番号で指す。
	switch identity.PositionKind {
	case core.PositionKindSequenceNumber:
		if record.SequenceNumber != nil {
			sequence := *record.SequenceNumber
			locator.PositionKind = core.PositionKindSequenceNumber
			locator.SequenceNumber = &sequence
		}
	case core.PositionKindByteRange:
		if record.ByteLength != nil && *record.ByteLength >= 1 {
			offset := record.ByteOffset
			locator.PositionKind = core.PositionKindByteRange
			locator.ByteOffset = &offset
			locator.ByteLength = clonePointer(record.ByteLength)
		}
	}
	if locator.PositionKind == "" {
		return nil
	}
	return locator
}

// withLineBytes は、行番号で指すレコードの位置の複製に、行の始まりの byte 位置と原文の byte 数を
// 足す。**取り込めなかったレコードの行の範囲を、失敗した byte の位置と分けて示す。**
// 行番号で指す位置は byte の範囲を持たない。byte の範囲を持つ位置と nil はそのまま返す。
func withLineBytes(locator *core.RecordLocator, record ParsedRecord) *core.RecordLocator {
	if locator == nil || locator.PositionKind != core.PositionKindLineNumber || locator.ByteOffset != nil {
		return locator
	}
	copied := cloneLocator(*locator)
	offset := record.ByteOffset
	copied.ByteOffset = &offset
	// 原文が占める byte 数を形式が与えるときはそれを採る。原文と原資料の byte 列が一致しない
	// 形式がある。
	length := int64(len(record.RawText))
	if record.ByteLength != nil {
		length = *record.ByteLength
	}
	if length > 0 {
		copied.ByteLength = &length
	}
	return &copied
}

// locatorPosition は位置の指し方が定める主の位置の値を返す。
// 主の位置を読めないレコードでは nil を返す。
func locatorPosition(locator *core.RecordLocator) *int64 {
	if locator == nil {
		return nil
	}
	switch locator.PositionKind {
	case core.PositionKindSequenceNumber:
		return locator.SequenceNumber
	case core.PositionKindByteRange:
		return locator.ByteOffset
	default:
		return locator.LineNumber
	}
}

func scanReadFailure(offset int64, hash string, err error) core.ImportFailure {
	return core.ImportFailure{
		SourceContentSha256: hash, Stage: core.FailureStageRead, ByteOffset: &offset,
		DiagnosisClass:   core.DiagnosisClassUndetermined,
		Interpretation:   "reading source bytes in record order",
		ExpectedMeaning:  "a complete read through the source end",
		ObservedResult:   err.Error(),
		UnresolvedReason: "the cause of the read failure has not been established independently",
	}
}

func finishScanCounts(source *scannedSource, succeeded, failed int64) error {
	counts := []core.ImportCount{
		{Category: core.ImportCategorySucceeded, Count: succeeded},
		{Category: core.ImportCategoryFailed, Count: failed},
	}
	if !source.ReadStopped {
		counts = append(counts, core.ImportCount{Category: core.ImportCategoryRead, Count: succeeded + failed})
	}
	values, err := core.NewImportCountSet(counts...)
	if err != nil {
		return fmt.Errorf("counting scanned records: %w", err)
	}
	source.Counts = values
	source.FailureCount = int64(len(source.Failures))
	source.DiagnosisCounts = nil
	classes := make(map[core.DiagnosisClass]int64)
	for _, failure := range source.Failures {
		classes[failure.Failure.DiagnosisClass]++
	}
	for _, class := range []core.DiagnosisClass{
		core.DiagnosisClassUndetermined,
		core.DiagnosisClassUnsupportedFormat,
		core.DiagnosisClassInconsistentInputConfirmed,
		core.DiagnosisClassImplementationDefectConfirmed,
	} {
		source.DiagnosisCounts = append(source.DiagnosisCounts, core.DiagnosisCount{
			DiagnosisClass: class, Count: classes[class],
		})
	}
	return nil
}

type scanByteReader struct {
	input io.Reader
	bytes int64
}

func (r *scanByteReader) Read(data []byte) (int, error) {
	n, err := r.input.Read(data)
	r.bytes += int64(n)
	return n, err
}

func scanSizeFailure(measured, scanned int64) *core.ImportFailure {
	if measured == scanned {
		return nil
	}
	return &core.ImportFailure{
		DiagnosisClass: core.DiagnosisClassUndetermined,
		Stage:          core.FailureStageRead, ByteOffset: &scanned,
		Interpretation:   "byte counts from the whole-source measurement and record scan",
		ExpectedMeaning:  fmt.Sprintf("a record scan of %d measured bytes", measured),
		ObservedResult:   fmt.Sprintf("the record scan read %d bytes", scanned),
		UnresolvedReason: "the cause of the byte count difference has not been established independently",
	}
}

// scanBounds は観測した位置の最小値と最大値を保持する。
// core.RecordRange は範囲の両端を持つ。
// 全レコードを含み、core.RecordRange.Validate の from <= to を満たすため数値の上下限を使う。
type scanBounds struct {
	seen       bool
	firstKnown bool
	lastKnown  bool
	lower      *int64
	upper      *int64
}

func (b *scanBounds) observe(position *int64) {
	if !b.seen {
		b.firstKnown = position != nil
		b.seen = true
	}
	b.lastKnown = position != nil
	if position == nil {
		return
	}
	if b.lower == nil || *position < *b.lower {
		b.lower = clonePointer(position)
	}
	if b.upper == nil || *position > *b.upper {
		b.upper = clonePointer(position)
	}
}

func (b scanBounds) apply(scope *core.RecordRange, kind core.PositionKind, stopped bool) {
	scope.RangeKind = core.RangeKindWholeSource
	scope.PositionKind = ""
	scope.FromPosition, scope.ToPosition = nil, nil
	if stopped || !b.firstKnown || !b.lastKnown {
		return
	}
	scope.RangeKind = core.RangeKindPositioned
	scope.PositionKind = kind
	scope.FromPosition, scope.ToPosition = clonePointer(b.lower), clonePointer(b.upper)
}
