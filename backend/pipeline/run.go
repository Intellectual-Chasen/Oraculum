package pipeline

import (
	"bytes"
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// Config は取り込みの実行に注入する依存である。
type Config struct {
	// Open は取得元の byte 列を開く。
	Open func(originPath string) (io.ReadCloser, error)
	// Parsers は入力形式ごとに新しい走査器を作る。
	Parsers map[core.FormatKey]ParserFactory
	// Minter は収集元とパーサーと解析実行の識別子を発行する。
	Minter IdentityMinter
	// Ordinals は取得元と内容の組に対する取り込み通番を発行する。
	Ordinals ImportOrdinalSource
	// Sanitize は診断メッセージを無害化する。
	Sanitize func(string) string
	// Revision は実行するコードの commit である。
	Revision string
	// SettingsDigest は解析設定の識別である。
	SettingsDigest string
	// Progress は収集元ごとの進行を受け取る。nil のときは報告しない。
	//
	// **読んだ byte 数を、読むたびに累計で報告する。** 走査と識別を終えた収集元は Scanned を
	// 真にして 1 回報告する。呼び出しは Run を呼んだ goroutine の上で行う。
	Progress func(SourceProgress)
}

// SourceProgress は取り込みの計画 1 件の進行である。
type SourceProgress struct {
	// Plan は Run に渡した計画の並びの位置である。
	Plan int
	// ReadBytes はその収集元から読んだ byte 数の累計である。
	ReadBytes int64
	// Scanned は、その収集元の走査と識別を終えたかである。
	Scanned bool
}

// progressReader は読んだ byte 数を累計で報告する reader である。
type progressReader struct {
	io.Reader
	plan     int
	read     int64
	progress func(SourceProgress)
}

func (r *progressReader) Read(buffer []byte) (int, error) {
	count, err := r.Reader.Read(buffer)
	if count > 0 {
		r.read += int64(count)
		r.progress(SourceProgress{Plan: r.plan, ReadBytes: r.read})
	}
	return count, err
}

// sourceImportError は、収集元 1 件の走査、識別、取り込みの状態の組み立て、端末の指定の適用の
// どれかの失敗である。失敗した収集元を計画の path で指す。
type sourceImportError struct {
	// step は失敗した処理の名前である。Error は step、収集元の path、元の失敗の順に並べる。
	step       string
	originPath string
	err        error
}

func (e *sourceImportError) Error() string {
	return fmt.Sprintf("%s source %q: %v", e.step, e.originPath, e.err)
}

func (e *sourceImportError) Unwrap() error { return e.err }

// Runner は収集元の測定、走査、識別と公開判定を実行する。
type Runner struct{ config Config }

// NewRunner は依存の欠けを検査して取り込みの実行器を作る。
func NewRunner(config Config) (*Runner, error) {
	if config.Open == nil || len(config.Parsers) == 0 || nilDependency(config.Minter) ||
		nilDependency(config.Ordinals) || config.Sanitize == nil {
		return nil, errors.New("creating runner: open, parsers, minter, ordinals and sanitize are required")
	}
	for format, factory := range config.Parsers {
		if format == "" || factory == nil {
			return nil, fmt.Errorf("creating runner: invalid parser registration for %q", format)
		}
	}
	config.Parsers = maps.Clone(config.Parsers)
	return &Runner{config: config}, nil
}

func nilDependency(value any) bool {
	if value == nil {
		return true
	}
	switch reflect.TypeOf(value).Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflect.ValueOf(value).IsNil()
	default:
		return false
	}
}

// Run は入力順に取り込み、診断と公開範囲を確定する。
// 収集元の失敗には originPath を付け、実行全体の操作名は呼び出し元が付ける。
func (r *Runner) Run(plans []SourcePlan) (ImportResult, error) {
	if r == nil || r.config.Open == nil {
		return ImportResult{}, errors.New("runner is not initialized")
	}
	if err := checkPlanCases(plans); err != nil {
		return ImportResult{}, err
	}
	// 表示名は走査より前に決める。レコードの位置と収集元の一覧が同じ名前を持つ。
	plans = DistinguishFileNames(plans)
	// 欄の並びの指定は収集元より前に読む。読めない指定の起動で収集元の byte を 1 つも
	// 読まない。
	parsers := make([]SourceParser, len(plans))
	for i, plan := range plans {
		factory := r.config.Parsers[plan.FormatKey]
		if factory == nil {
			return ImportResult{}, fmt.Errorf("no parser for %q", plan.FormatKey)
		}
		parser, err := factory(plan.FormatSpec)
		if err != nil {
			return ImportResult{}, err
		}
		if nilDependency(parser) {
			return ImportResult{}, errors.New("creating source parser: factory returned nil")
		}
		parsers[i] = parser
	}
	sources := make([]scannedSource, len(plans))
	manifest := make([]RunManifestEntry, len(plans))
	identities := make(map[string]core.SourceIdentity, len(plans))
	for i, plan := range plans {
		source, err := r.scan(i, plan, parsers[i])
		if err != nil {
			return ImportResult{}, &sourceImportError{step: "importing", originPath: plan.OriginPath, err: err}
		}
		sources[i] = source
		entry, err := r.identify(source)
		if err != nil {
			return ImportResult{}, &sourceImportError{step: "identifying", originPath: plan.OriginPath, err: err}
		}
		manifest[i] = entry
		if r.config.Progress != nil {
			// 進行の分母は主 file の byte 数である。付属の file の byte を数えない。
			readBytes := source.Measurement.SizeBytes
			if len(source.Members) > 0 {
				readBytes = source.Members[0].SizeBytes
			}
			r.config.Progress(SourceProgress{Plan: i, ReadBytes: readBytes, Scanned: true})
		}
		identity := sourceIdentity(source, entry.SourceId)
		if err := identity.Validate(); err != nil {
			return ImportResult{}, fmt.Errorf("validating source identity: %w", err)
		}
		if _, exists := identities[entry.SourceId]; exists {
			return ImportResult{}, fmt.Errorf("identifying source: duplicate source identity %q", entry.SourceId)
		}
		identities[entry.SourceId] = identity
	}
	for _, adjust := range scannedSourcesAdjusters {
		adjust(sources)
	}
	runRef, err := r.config.Minter.AnalysisRunRef(manifest)
	if err != nil {
		return ImportResult{}, fmt.Errorf("minting analysis run reference: %w", err)
	}
	index := rawTextIndex{references: make(map[rawTextKey]string), texts: make(map[string]indexedRawText)}
	statuses := make([]core.ImportStatus, len(plans))
	for i, source := range sources {
		entry := manifest[i]
		status, err := buildImportStatus(source, runRef, entry.ParserVersion, entry.SourceId,
			r.config.Sanitize, index.add)
		if err != nil {
			return ImportResult{}, &sourceImportError{
				step: "building the status of", originPath: source.Plan.OriginPath, err: err,
			}
		}
		statuses[i] = status
	}
	result, err := newImportResult(sources, statuses, runRef, index.add)
	if err != nil {
		return ImportResult{}, fmt.Errorf("settling run: %w", err)
	}
	result.identities, result.rawTexts = identities, index
	for i, plan := range plans {
		if plan.Terminal == nil {
			continue
		}
		sourceId := manifest[i].SourceId
		// レコードを持たず失敗だけを持つ収集元は、端末に置くものを持たない。端末を付けずに
		// 取り込み、失敗は収集元の状態に残す。レコードも失敗も持たない収集元は、指定の誤りと
		// して止める。
		onlyFailures := len(result.publications[i].records) == 0 && statuses[i].FailureCount > 0
		if plan.Terminal.HasTerminal() && !onlyFailures {
			assignment, err := importSpecifiedAssignment(*plan.Terminal, identities[sourceId], result.localRangeOf(sourceId))
			if err != nil {
				return ImportResult{}, &sourceImportError{
					step: "applying the terminal of", originPath: plan.OriginPath, err: err,
				}
			}
			result.importAssignments = append(result.importAssignments, assignment)
		}
		if plan.Terminal.TimeOffset != nil {
			if result.importOffsets == nil {
				result.importOffsets = map[string]core.UtcOffset{}
			}
			result.importOffsets[sourceId] = *plan.Terminal.TimeOffset
		}
	}
	return result, nil
}

// localRangeOf は収集元の地方時の文字列の最も早い値と最も遅い値を返す。持たない収集元では nil である。
func (r ImportResult) localRangeOf(sourceId string) *core.TimeRange {
	for _, publication := range r.publications {
		if publication.status.SourceId == sourceId {
			return publication.localRange
		}
	}
	return nil
}

// importSpecifiedAssignment は、利用者が取り込みの起動で指定した端末を、その収集元 1 件に
// 付ける割当にする。
//
// **割当を適用してよい期間は、その収集元の観測期間である** (applicableRangeOf)。UTC からの
// ずれを持たない収集元は、地方時の文字列の範囲 (localRange) を期間にする。期間はグラフを組む
// ときに、その収集元の時刻の解釈 (起動で指定したずれか、画面で記録した解釈) で読む
// (withInterpretedRange)。解釈を持たない間は、期間を時点と比べる判定に使われない。
// 時刻を持つレコードが無い収集元には端末を付けられない。
func importSpecifiedAssignment(
	terminal SourceTerminal, identity core.SourceIdentity, localRange *core.TimeRange,
) (core.TerminalAssignment, error) {
	validRange, usable := applicableRangeOf(identity)
	if !usable && localRange != nil {
		validRange = core.TimeRange{From: cloneTimestamp(localRange.From), To: cloneTimestamp(localRange.To)}
		usable = validRange.Validate() == nil
	}
	if !usable {
		return core.TerminalAssignment{}, errors.New("the source has no record time, " +
			"and a terminal specified at import applies only within the observed range of the source; " +
			"import the source without its terminal to place its records on the unknown terminal of the file")
	}
	assignment := core.TerminalAssignment{
		ClientIp: terminal.Ip, TerminalId: terminal.TerminalId,
		TerminalHostname: terminal.TerminalHostname,
		SourceId:         identity.SourceId, SourceContentSha256: identity.ContentSha256,
		AssignmentValidRange: validRange,
		Origin:               core.TerminalAssignmentOriginImportSpecified,
		AppliesToSourceId:    identity.SourceId,
	}
	if err := assignment.Validate(); err != nil {
		return core.TerminalAssignment{}, err
	}
	return assignment, nil
}

func (r *Runner) scan(index int, plan SourcePlan, parser SourceParser) (scannedSource, error) {
	// Run が収集元の文脈を付ける。Open が持つ操作名や path は重ねない。
	data, err := r.readWhole(plan.OriginPath, index)
	if err != nil {
		return scannedSource{}, err
	}
	var members []core.SourceMember
	if companion, ok := parser.(CompanionFileParser); ok {
		data, members, err = r.appendCompanions(plan.OriginPath, data, companion.CompanionSuffixes())
		if err != nil {
			return scannedSource{}, err
		}
		companion.SetMembers(slices.Clone(members))
	}
	// 内容の識別と記録した指定との照合は、付属の file を連結した後の byte 列について行う。
	measurement, err := measureWholeSource(bytes.NewReader(data))
	if err != nil {
		return scannedSource{}, fmt.Errorf("measuring source: %w", err)
	}
	// 記録した指定と別の byte 列を読んだ取り込みは、記録を再現しない。取り込み全体を止め、
	// どの収集元も公開しない。
	if expected := plan.ExpectedContentSha256; expected != nil && *expected != measurement.ContentSha256 {
		return scannedSource{}, fmt.Errorf("content sha256 mismatch: recorded %s, read %s",
			*expected, measurement.ContentSha256)
	}
	source, err := scanSource(parser, bytes.NewReader(data), measurement, plan)
	if err != nil && !source.ReadStopped {
		return scannedSource{}, fmt.Errorf("scanning source: %w", err)
	}
	source.Members = members
	// 途中で止まった走査は、保持された read 診断と件数を公開判定へ渡す。
	return source, nil
}

// readWhole は path の byte 列を全部読む。plan が 0 以上のときは読んだ byte 数を進行に報告する。
func (r *Runner) readWhole(path string, plan int) ([]byte, error) {
	input, err := r.config.Open(path)
	if err != nil {
		return nil, err
	}
	if nilDependency(input) {
		return nil, errors.New("opening source: opener returned nil")
	}
	var reader io.Reader = input
	if r.config.Progress != nil && plan >= 0 {
		reader = &progressReader{Reader: input, plan: plan, progress: r.config.Progress}
	}
	// 既知の制限: 収集元 1 件の byte 列を全部メモリに保持する, 収集元の大きさの上限が未決で実運用の最大メモリ量を測れない, 収集元の大きさの上限が決まったときに見直す
	data, readErr := io.ReadAll(reader)
	closeErr := input.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		return nil, fmt.Errorf("reading and closing whole source: %w", err)
	}
	return data, nil
}

// appendCompanions は主 file の path に接尾辞を足した付属の file を並びの順に読み、在る file を
// data の後ろに連結する。返す members の先頭は主 file である。
//
// **無い file だけを飛ばす。** 開けない file とほかの読み取りの失敗は、取り込み全体を止める。
// 付属の file を通知せずに欠いた収集元は、読んだ結果が収集時点の状態と食い違う。
func (r *Runner) appendCompanions(
	originPath string, data []byte, suffixes []string,
) ([]byte, []core.SourceMember, error) {
	members := []core.SourceMember{{
		OriginPath: originPath, ContentSha256: sha256Hex(data), SizeBytes: int64(len(data)),
	}}
	for _, suffix := range suffixes {
		path := originPath + suffix
		part, err := r.readWhole(path, -1)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, nil, fmt.Errorf("reading the companion file %q: %w", path, err)
		}
		members = append(members, core.SourceMember{
			OriginPath: path, ContentSha256: sha256Hex(part),
			ByteOffset: int64(len(data)), SizeBytes: int64(len(part)),
		})
		data = append(data, part...)
	}
	return data, members, nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (r *Runner) identify(source scannedSource) (RunManifestEntry, error) {
	ordinal, err := r.config.Ordinals.Next(source.Plan.OriginPath, source.Measurement.ContentSha256)
	if err != nil {
		return RunManifestEntry{}, fmt.Errorf("issuing import ordinal: %w", err)
	}
	id, err := r.config.Minter.SourceId(source.Plan.OriginPath, source.Measurement.ContentSha256, ordinal)
	if err != nil {
		return RunManifestEntry{}, fmt.Errorf("minting source identity: %w", err)
	}
	version, err := r.config.Minter.ParserVersion(source.Parser, r.config.Revision, r.config.SettingsDigest)
	if err != nil {
		return RunManifestEntry{}, fmt.Errorf("minting parser version: %w", err)
	}
	if version == "" {
		return RunManifestEntry{}, errors.New("minting parser version: empty version")
	}
	entry := RunManifestEntry{SourceId: id, ParserVersion: version}
	if source.Plan.CaseId != nil {
		entry.CaseId = *source.Plan.CaseId
	}
	return entry, nil
}

// checkPlanCases は収集元に付けた案件の文字列と、案件の有無が全収集元で揃うことを確かめる。
//
// 案件を付けない収集元は、どの案件のレコードと関連付けるかが決まらない。一部の収集元にだけ
// 案件を付けた取り込みを受け付けない。
func checkPlanCases(plans []SourcePlan) error {
	for _, plan := range plans {
		if (plan.CaseId != nil) != (plans[0].CaseId != nil) {
			return errors.New("checking cases: some sources carry a case and others do not")
		}
		if plan.CaseId == nil {
			continue
		}
		if err := core.ValidateCaseId("case", *plan.CaseId); err != nil {
			return fmt.Errorf("checking the case of source %q: %w", plan.OriginPath, err)
		}
	}
	return nil
}

// sourceIdentity は全レコードの read 件数を RecordCount に使い、末尾に達する前に停止した場合は省略する。
// FormatVersion は省略する。markii 形式と Squid の原資料は入力形式のバージョンを読み取る項目を持たない。
// ObservedRange は、絶対時刻として読める根拠の最小と最大を使う。1 件も無い収集元では
// 2 つとも出ない。**解析に失敗したレコードも母集団に含む。** 収録範囲は収集元がいつから
// いつまでを記録したかであり、こちらが値を読めたかとは別の事実である。
//
// **行の並びで両端を採らない。** 行の並びと時刻の並びが一致しない収集元では、行の先頭が
// 収録の開始を指さない。両端は端末の割当を適用してよい期間 (applicableRangeOf) に入り、
// 段階 1 の terminal_ip_assignment の条件を通って調査の結論に直接作用する。
func sourceIdentity(source scannedSource, id string) core.SourceIdentity {
	m, plan := source.Measurement, source.Plan
	identity := core.SourceIdentity{SourceId: id, ContentSha256: m.ContentSha256,
		OriginPath: plan.OriginPath, FileName: plan.FileName, FormatKey: plan.FormatKey,
		SizeBytes: m.SizeBytes, NewlineCount: m.NewlineCount, EndsWithNewline: m.EndsWithNewline, LineEnding: m.LineEnding}
	if spec := source.Parser.FormatSpec; spec != "" {
		identity.FormatSpec = &spec
	}
	identity.CaseId = clonePointer(plan.CaseId)
	identity.CollectionPath = plan.CollectionPath
	identity.RawTextConverted = source.Parser.RawTextConverted
	identity.FileHeader = cloneRecordFields(source.FileHeader)
	identity.Members = slices.Clone(source.Members)
	if count, ok := source.Counts.Count(core.ImportCategoryRead); ok && !source.ReadStopped {
		identity.RecordCount = &count
	}
	var first, last time.Time
	observe := func(timestamp *core.Timestamp) {
		if timestamp == nil {
			return
		}
		at, absolute := timestamp.Instant()
		if !absolute {
			return
		}
		if identity.ObservedRangeFirst == nil || at.Before(first) {
			identity.ObservedRangeFirst, first = cloneTimestampPointer(timestamp), at
		}
		if identity.ObservedRangeLast == nil || at.After(last) {
			identity.ObservedRangeLast, last = cloneTimestampPointer(timestamp), at
		}
	}
	for _, record := range source.Records {
		observe(record.ObservedAt)
	}
	for _, failed := range source.Failures {
		observe(failed.ObservedAt)
	}
	if source.Parser.CountsUnrenderedMessages {
		count := source.MessageUnrenderedCount
		identity.MessageUnrenderedCount = &count
	}
	identity.TerminalCandidates = terminalCandidatesOf(source.TerminalCandidates)
	return identity
}

// terminalCandidatesOf は候補の名前ごとの件数を、件数の多い順、同数は名前の昇順に並べる。
// 候補が無いときは nil を返す。
func terminalCandidatesOf(counts map[string]int64) []core.TerminalCandidate {
	var candidates []core.TerminalCandidate
	for name, count := range counts {
		candidates = append(candidates, core.TerminalCandidate{Name: name, RecordCount: count})
	}
	slices.SortFunc(candidates, func(a, b core.TerminalCandidate) int {
		if byCount := cmp.Compare(b.RecordCount, a.RecordCount); byCount != 0 {
			return byCount
		}
		return strings.Compare(a.Name, b.Name)
	})
	return candidates
}
