package markii_test

import (
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/markii"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// プロセス開始のレコードだけを選ぶ。
func TestIsProcessStartSelectsOnlyTheStartOfAProcess(t *testing.T) {
	cases := map[string]struct {
		line string
		want bool
	}{
		"プロセスの起動": {processStartLine, true},
		"プロセスの終了": {processStopLine, false},
		"通信":      {communicationLine, false},
		"evt を欠く": {processStartWithoutEventLine, false},
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			if got := markii.IsProcessStart(readOneOK(t, want.line)); got != want.want {
				t.Errorf("IsProcessStart = %t, want %t", got, want.want)
			}
		})
	}
}

// 位置とプロセスと端末の識別の材料を持つ。
func TestParseProcessStartCarriesTheIdentifyingValues(t *testing.T) {
	got := parseProcessStartOK(t, processStartLine)

	if got.SequenceNumber == nil {
		t.Fatal("the observation must carry the sequence number")
	}
	if *got.SequenceNumber != processStartSequenceNumber {
		t.Errorf("sequenceNumber = %d, want %d", *got.SequenceNumber, processStartSequenceNumber)
	}
	if got.LineNumber != 1 {
		t.Errorf("lineNumber = %d, want 1", got.LineNumber)
	}
	if got.ProcessGuid != processStartProcessGuid {
		t.Errorf("processGuid = %q, want %q", got.ProcessGuid, processStartProcessGuid)
	}
	if got.TerminalId != processStartTerminalId {
		t.Errorf("terminalId = %q, want %q", got.TerminalId, processStartTerminalId)
	}
}

// 親への参照は文字列のまま持つ。結び付けを行わない。
func TestParseProcessStartCarriesTheParentReferenceAsText(t *testing.T) {
	got := parseProcessStartOK(t, processStartLine)

	if got.ParentGuid.ValueState != core.ValueStatePresent {
		t.Errorf("parentGuid valueState = %q, want %q",
			got.ParentGuid.ValueState, core.ValueStatePresent)
	}
	if raw, ok := got.ParentGuid.RawTextValue(); !ok || raw != processStartParentGuid {
		t.Errorf("parentGuid rawText = %q (present %t), want %q", raw, ok, processStartParentGuid)
	}
}

// parentGUID を欠くレコードでも観測を作る。欄の不在として持つ。
func TestParseProcessStartKeepsAnAbsentParentAsAnAbsentItem(t *testing.T) {
	got := parseProcessStartOK(t, processStartWithoutParentLine)

	if got.ParentGuid.ValueState != core.ValueStateItemAbsent {
		t.Errorf("parentGuid valueState = %q, want %q",
			got.ParentGuid.ValueState, core.ValueStateItemAbsent)
	}
	if _, ok := got.ParentGuid.RawTextValue(); ok {
		t.Error("an absent item must not carry a raw text")
	}
}

// 観測の種別は evt と subEvt の 2 件で、意味が確定している。
func TestParseProcessStartBuildsTheObservationKind(t *testing.T) {
	got := parseProcessStartOK(t, processStartLine)

	if len(got.ObservationKind.Raw) != 2 {
		t.Fatalf("observationKind raw element count = %d, want 2", len(got.ObservationKind.Raw))
	}
	if got.ObservationKind.Raw[0].Name != "evt" || got.ObservationKind.Raw[1].Name != "subEvt" {
		t.Errorf("observationKind raw names = %q and %q, want evt and subEvt",
			got.ObservationKind.Raw[0].Name, got.ObservationKind.Raw[1].Name)
	}
	if got.ObservationKind.Status != core.ObservationKindStatusDetermined {
		t.Errorf("observationKind status = %q, want %q",
			got.ObservationKind.Status, core.ObservationKindStatusDetermined)
	}
	if !got.ObservationKind.Matches(markii.ObservationKindSelectorOf("ps", "start")) {
		t.Error("the observation kind must match the selector for a process start")
	}
}

// 引用符付きの value は引用符を外した正規化値を持ち、引用符無しの value は持たない。
func TestParseProcessStartNormalizesOnlyQuotedValues(t *testing.T) {
	got := parseProcessStartOK(t, processStartLine)

	quoted := fieldNamed(t, got, "psPath")
	if raw, ok := quoted.Text.RawTextValue(); !ok || raw != processStartRawPath {
		t.Errorf("psPath rawText = %q (present %t), want %q", raw, ok, processStartRawPath)
	}
	if normalized, ok := quoted.Text.NormalizedValue(); !ok || normalized != processStartPath {
		t.Errorf("psPath normalized = %q (present %t), want %q", normalized, ok, processStartPath)
	}
	if _, ok := quoted.Text.DerivationValue(); !ok {
		t.Error("a normalized value must carry how it was derived")
	}

	unquoted := fieldNamed(t, got, "psID")
	if raw, ok := unquoted.Text.RawTextValue(); !ok || raw != "4321" {
		t.Errorf("psID rawText = %q (present %t), want %q", raw, ok, "4321")
	}
	if _, ok := unquoted.Text.NormalizedValue(); ok {
		t.Error("an unquoted value must not carry a normalized value")
	}
}

// 値が 0 の欄は present である。欄が無い状態と別の形になる。
func TestParseProcessStartKeepsAZeroValueAsPresent(t *testing.T) {
	got := parseProcessStartOK(t, processStartLine)

	zero := fieldNamed(t, got, "sessionID")
	if zero.Text.ValueState != core.ValueStatePresent {
		t.Errorf("sessionID valueState = %q, want %q", zero.Text.ValueState, core.ValueStatePresent)
	}
	if raw, ok := zero.Text.RawTextValue(); !ok || raw != "0" {
		t.Errorf("sessionID rawText = %q (present %t), want %q", raw, ok, "0")
	}
}

// 引用符 2 個は 1 個へ復号する。原資料の文字列は 2 個のまま残す。
func TestParseProcessStartDecodesDoubledQuotesInAValue(t *testing.T) {
	got := parseProcessStartOK(t, processStartLine)

	command := fieldNamed(t, got, "cmd")
	wantRaw := `"notepad.exe ""C:\work\note.txt"""`
	wantNormalized := `notepad.exe "C:\work\note.txt"`
	if raw, ok := command.Text.RawTextValue(); !ok || raw != wantRaw {
		t.Errorf("cmd rawText = %q (present %t), want %q", raw, ok, wantRaw)
	}
	if normalized, ok := command.Text.NormalizedValue(); !ok || normalized != wantNormalized {
		t.Errorf("cmd normalized = %q (present %t), want %q", normalized, ok, wantNormalized)
	}
}

// 1 件目の項目はヘッダーの時刻で、名前は headerTime である。
func TestParseProcessStartPutsTheHeaderTimeFirst(t *testing.T) {
	got := parseProcessStartOK(t, processStartLine)

	if len(got.Fields) == 0 {
		t.Fatal("the observation must carry at least one field")
	}
	if got.Fields[0].Name != "headerTime" {
		t.Errorf("the first field is %q, want headerTime", got.Fields[0].Name)
	}
	if got.Fields[0].Kind != core.RecordFieldKindTimestamp {
		t.Errorf("headerTime kind = %q, want %q",
			got.Fields[0].Kind, core.RecordFieldKindTimestamp)
	}
	// 残りは原文の並び順を保つ。
	if got.Fields[1].Name != "loc" {
		t.Errorf("the second field is %q, want loc", got.Fields[1].Name)
	}
}

// 省略可の key が 2 回出るレコードで、2 つの値を別々に持つ。
//
// 名前で探し直すと 1 つ目の値を 2 回持ち、2 つ目が応答から消える。
func TestParseProcessStartKeepsBothValuesOfARepeatedOptionalKey(t *testing.T) {
	got := parseProcessStartOK(t, processStartWithRepeatedOptionalKeyLine)

	var values []string
	for _, field := range got.Fields {
		if field.Name != "lv" {
			continue
		}
		raw, ok := field.Text.RawTextValue()
		if !ok {
			t.Fatal("the field lv must carry a raw text")
		}
		values = append(values, raw)
	}
	if len(values) != 2 {
		t.Fatalf("the observation carries %d fields named lv, want 2", len(values))
	}
	if values[0] != "5" || values[1] != "7" {
		t.Errorf("the values of lv are %q and %q, want 5 and 7", values[0], values[1])
	}
}

// 必須の key を欠いたレコードから観測を作らない。
func TestParseProcessStartReportsAMissingRequiredKey(t *testing.T) {
	observation, failure := markii.ParseProcessStart(readOneOK(t, processStartWithoutPathLine))
	if failure == nil {
		t.Fatal("a record without a required key must be reported as a failure")
	}
	if failure.Stage != core.FailureStageFieldMap {
		t.Errorf("stage = %q, want %q", failure.Stage, core.FailureStageFieldMap)
	}
	if failure.DiagnosisClass != core.DiagnosisClassUndetermined {
		t.Errorf("diagnosisClass = %q, want %q",
			failure.DiagnosisClass, core.DiagnosisClassUndetermined)
	}
	if observation.ProcessGuid != "" {
		t.Error("a failed record must not carry an observation")
	}
	if observation.LineNumber != 1 {
		t.Errorf("a failed record must carry its line number, got %d", observation.LineNumber)
	}
}

// 必須の key が 2 回出るレコードから観測を作らない。先勝ちで採らない。
func TestParseProcessStartReportsARepeatedRequiredKey(t *testing.T) {
	_, failure := markii.ParseProcessStart(readOneOK(t, processStartWithRepeatedKeyLine))
	if failure == nil {
		t.Fatal("a record with a repeated required key must be reported as a failure")
	}
	if failure.Stage != core.FailureStageFieldMap {
		t.Errorf("stage = %q, want %q", failure.Stage, core.FailureStageFieldMap)
	}
}

// 必須の key が値を持たないレコードから観測を作らない。
//
// key があって value が空の形は、後の処理の ProcessRef.Validate が必ず失敗する値になる。
func TestParseProcessStartReportsAnEmptyRequiredValue(t *testing.T) {
	observation, failure := markii.ParseProcessStart(readOneOK(t, processStartWithEmptyGuidLine))
	if failure == nil {
		t.Fatal("a required key without a value must be reported as a failure")
	}
	if failure.Stage != core.FailureStageFieldMap {
		t.Errorf("stage = %q, want %q", failure.Stage, core.FailureStageFieldMap)
	}
	if observation.ProcessGuid != "" {
		t.Error("a failed record must not carry an observation")
	}
}

// sn を読めないレコードでは通番を持たない。代用の値を入れない。
func TestParseProcessStartLeavesTheSequenceNumberUnsetWhenItCannotBeRead(t *testing.T) {
	observation, failure := markii.ParseProcessStart(readOneOK(t, processStartWithBadSequenceLine))
	if failure == nil {
		t.Fatal("a sequence number that is not a decimal integer must be reported as a failure")
	}
	if observation.SequenceNumber != nil {
		t.Errorf("sequenceNumber = %d, want it unset", *observation.SequenceNumber)
	}
}

// ヘッダーの時刻を読めない失敗では、先に読めた通番を持つ。
func TestParseProcessStartCarriesTheSequenceNumberOfALaterFailure(t *testing.T) {
	observation, failure := markii.ParseProcessStart(readOneOK(t, processStartWithBadHeaderTimeLine))
	if failure == nil {
		t.Fatal("a header time that cannot be read must be reported as a failure")
	}
	if observation.SequenceNumber == nil {
		t.Fatal("a failure after reading the sequence number must carry it")
	}
	if *observation.SequenceNumber != 900204 {
		t.Errorf("sequenceNumber = %d, want 900204", *observation.SequenceNumber)
	}
}

// 他の種別のレコードを渡した呼び出しは失敗を返す。
func TestParseProcessStartRejectsARecordOfAnotherKind(t *testing.T) {
	_, failure := markii.ParseProcessStart(readOneOK(t, communicationLine))
	if failure == nil {
		t.Fatal("a record of another kind must be reported as a failure")
	}
	if failure.Stage != core.FailureStageFieldMap {
		t.Errorf("stage = %q, want %q", failure.Stage, core.FailureStageFieldMap)
	}
}

// 解析は 4 項目を埋めない。埋めるのは取り込みの実行である。
func TestParseProcessStartLeavesTheItemsItCannotKnowEmpty(t *testing.T) {
	_, failure := markii.ParseProcessStart(readOneOK(t, processStartWithoutPathLine))
	if failure == nil {
		t.Fatal("the record must fail")
	}
	if failure.SourceId != "" {
		t.Errorf("sourceId = %q, want it empty", failure.SourceId)
	}
	if failure.SourceContentSha256 != "" {
		t.Errorf("sourceContentSha256 = %q, want it empty", failure.SourceContentSha256)
	}
	if failure.ParserVersion != "" {
		t.Errorf("parserVersion = %q, want it empty", failure.ParserVersion)
	}
	if failure.SanitizedMessage != "" {
		t.Errorf("sanitizedMessage = %q, want it empty", failure.SanitizedMessage)
	}
	if failure.RecordRef != nil {
		t.Error("recordRef must be empty")
	}
}

// 取り込みの実行が RecordLocator と ProcessRef の収集元の項目を足すと、core の型が
// Validate を通る。
//
// 解析が返す材料だけでは通らないことと、足せば通ることの両方を固定する。
func TestTheItemsTheImportRunAddsCompleteTheContractTypes(t *testing.T) {
	got := parseProcessStartOK(t, processStartLine)

	const (
		sourceId = "source-1"
		// 小文字 16 進 64 桁であることだけが検査の対象になる。
		sha256 = "cc33dd44ee55ff6600778899aa11bb22cc33dd44ee55ff6600778899aa11bb22"
	)
	locator := core.RecordLocator{
		SourceId:            sourceId,
		SourceContentSha256: sha256,
		SourceFileName:      "example.log",
		PositionKind:        core.PositionKindSequenceNumber,
		SequenceNumber:      got.SequenceNumber,
		LineNumber:          &got.LineNumber,
		RecordRawTextRef:    "/api/v0/records?sourceId=source-1&sequenceNumber=900200",
	}
	if err := locator.Validate(); err != nil {
		t.Errorf("the completed RecordLocator must be valid: %v", err)
	}

	processRef := core.ProcessRef{
		SourceId:            sourceId,
		SourceContentSha256: sha256,
		ProcessId:           got.ProcessGuid,
		TerminalId:          got.TerminalId,
	}
	if err := processRef.Validate(); err != nil {
		t.Errorf("the completed ProcessRef must be valid: %v", err)
	}

	if err := got.StartTime.Validate(); err != nil {
		t.Errorf("the header timestamp must be valid on its own: %v", err)
	}
	if err := got.ObservationKind.Validate(); err != nil {
		t.Errorf("the observation kind must be valid on its own: %v", err)
	}
	for _, field := range got.Fields {
		if err := field.Validate(); err != nil {
			t.Errorf("the field %q must be valid on its own: %v", field.Name, err)
		}
	}
}

// fieldNamed は名前で項目を探す。
func fieldNamed(t *testing.T, observation markii.ProcessStart, name string) core.RecordField {
	t.Helper()
	for _, field := range observation.Fields {
		if field.Name == name {
			return field
		}
	}
	t.Fatalf("the observation must carry the field %q", name)
	return core.RecordField{}
}
