package pipeline_test

import (
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

const markiiScanLine = `01/02/2024 03:04:05.006 +0000 sn=7 evt=ps subEvt=start psGUID=p tmid=t com=c csid=s psPath=app`

func TestMarkIIParserIdentity(t *testing.T) {
	got := pipeline.NewTestMarkIIParser().Identity()
	if got.ParserID != "markii-client-log" || got.SupportedFormatVersion != "V3.0/V3.2" ||
		got.FormatKey != pipeline.MarkIIFormatKey || got.PositionKind != core.PositionKindSequenceNumber {
		t.Errorf("identity = %+v", got)
	}
}

func TestMarkIIParserResetAndSemantics(t *testing.T) {
	parser := pipeline.NewTestMarkIIParser()
	for range 2 {
		communication := strings.Replace(markiiScanLine, "evt=ps subEvt=start", "evt=net subEvt=con", 1)
		parser.Reset(strings.NewReader(markiiScanLine + "\r\n" + communication))
		for index, raw := range []string{markiiScanLine, communication} {
			record, failure, err := parser.Next()
			if err != nil || failure != nil {
				t.Fatalf("Next = %+v, %v", failure, err)
			}
			if record.RawText != raw || record.LineNumber != int64(index+1) || record.SequenceNumber == nil || *record.SequenceNumber != 7 {
				t.Errorf("record = %+v", record)
			}
			if record.ObservedAt == nil || record.ObservedAt.Normalized == nil || *record.ObservedAt.Normalized != "2024-01-02T03:04:05.006Z" {
				t.Errorf("observed time = %+v", record.ObservedAt)
			}
		}
		record, failure, err := parser.Next()
		empty := record.RawText == "" && record.LineNumber == 0 && record.SequenceNumber == nil &&
			record.ObservedAt == nil && record.Semantics == nil && len(record.Terminal) == 0
		if !errors.Is(err, io.EOF) || failure != nil || !empty {
			t.Errorf("end = %+v, %+v, %v", record, failure, err)
		}
	}
}

func TestMarkIIParserFailureAndOtherEvent(t *testing.T) {
	cases := []struct {
		name        string
		input       string
		stage       core.FailureStage
		hasSequence bool
		hasTime     bool
	}{
		{"tokenize", "broken", core.FailureStageTokenize, false, false},
		{"sequence", strings.Replace(markiiScanLine, "sn=7", "sn=bad", 1), core.FailureStageFieldMap, false, false},
		{"missing_sequence", strings.Replace(markiiScanLine, "sn=7 ", "", 1), core.FailureStageFieldMap, false, false},
		{"time", strings.Replace(markiiScanLine, "01/02/2024", "99/02/2024", 1), core.FailureStageNormalize, true, false},
		{"other_event", strings.Replace(markiiScanLine, "evt=ps", "evt=other", 1), "", true, true},
		{"other_event_missing_sequence", strings.NewReplacer("evt=ps", "evt=other", "sn=7 ", "").Replace(markiiScanLine), core.FailureStageFieldMap, false, false},
		{"other_event_missing_sub_event", strings.NewReplacer("evt=ps", "evt=other", "subEvt=start ", "").Replace(markiiScanLine), core.FailureStageFieldMap, true, false},
		// tmid は種別に依らない意味付けの必須の key ではない。欠けても観測を作る。
		{"other_event_without_terminal_id", strings.NewReplacer("evt=ps", "evt=other", "tmid=t ", "").Replace(markiiScanLine), "", true, true},
		{"other_event_time", strings.NewReplacer("01/02/2024", "99/02/2024", "evt=ps", "evt=other").Replace(markiiScanLine), core.FailureStageNormalize, true, false},
		{"negative_sequence", strings.Replace(markiiScanLine, "sn=7", "sn=-1", 1), core.FailureStageFieldMap, false, false},
		{"duplicate_sequence", strings.Replace(markiiScanLine, "sn=7", "sn=7 sn=8", 1), core.FailureStageFieldMap, false, false},
		{"communication_sequence", strings.NewReplacer("sn=7", "sn=bad", "evt=ps subEvt=start", "evt=net subEvt=con").Replace(markiiScanLine), core.FailureStageFieldMap, false, false},
		{"communication_time", strings.NewReplacer("01/02/2024", "99/02/2024", "evt=ps subEvt=start", "evt=net subEvt=con").Replace(markiiScanLine), core.FailureStageNormalize, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parser := pipeline.NewTestMarkIIParser()
			parser.Reset(strings.NewReader(tc.input))
			record, failure, err := parser.Next()
			if err != nil || (failure != nil) != (tc.stage != "") {
				t.Fatalf("Next = %+v, %v", failure, err)
			}
			if failure != nil && failure.Stage != tc.stage {
				t.Errorf("stage = %s, want %s", failure.Stage, tc.stage)
			}
			if record.RawText != tc.input || record.LineNumber != 1 || (record.SequenceNumber != nil) != tc.hasSequence || (record.ObservedAt != nil) != tc.hasTime {
				t.Errorf("record = %+v", record)
			}
		})
	}
}

func TestMarkIIParserReadFailureKeepsDiagnosis(t *testing.T) {
	cause := errors.New("source interrupted")
	parser := pipeline.NewTestMarkIIParser()
	parser.Reset(io.MultiReader(strings.NewReader("fragment"), iotest.ErrReader(cause)))
	record, failure, err := parser.Next()
	if !errors.Is(err, cause) || failure == nil {
		t.Fatalf("Next = %+v, %v", failure, err)
	}
	if record.RawText != "fragment" || record.LineNumber != 1 || failure.Stage != core.FailureStageRead {
		t.Errorf("interrupted record = %+v, %+v", record, failure)
	}
}

func TestMarkIIParserOtherEventHeaderTime(t *testing.T) {
	for _, tc := range []struct {
		date    string
		hasTime bool
	}{
		{"04/05/2024", true}, {"99/05/2024", false},
	} {
		t.Run(tc.date, func(t *testing.T) {
			parser := pipeline.NewTestMarkIIParser()
			parser.Reset(strings.NewReader(tc.date + " 06:07:08.009 +0000 sn=12 evt=file subEvt=close tmid=t"))
			record, failure, err := parser.Next()
			if err != nil || (failure == nil) != tc.hasTime || (record.ObservedAt != nil) != tc.hasTime {
				t.Fatalf("record=%+v failure=%v error=%v", record, failure, err)
			}
			if !tc.hasTime {
				if failure.Stage != core.FailureStageNormalize {
					t.Fatalf("stage=%s want %s", failure.Stage, core.FailureStageNormalize)
				}
				return
			}
			if err := record.ObservedAt.Validate(); err != nil {
				t.Fatal(err)
			}
			if value, ok := record.ObservedAt.NormalizedValue(); !ok || value != "2024-04-05T06:07:08.009Z" {
				t.Fatalf("time=%q present=%t", value, ok)
			}
		})
	}
}

// 走査器は、種別に依らず取り込みに成功した全レコードに意味付けを与える。
//
// 4 行は 4 つの evt の形に合わせたテスト用の行である。reg/create はどのバージョンの表にも無く、
// 意味を推定した組であるため inferred になる。
func TestMarkIIParserGivesEverySuccessfulRecordSemantics(t *testing.T) {
	const terminal = `tmid=00000000-1111-2222-3333-444444444444`
	cases := []struct {
		name   string
		line   string
		field  string
		value  string
		status core.ObservationKindStatus
	}{
		{"file", `02/01/2000 03:04:05.678 +0900 sn=801 evt=file subEvt=close com="HOST01" ` +
			terminal + ` psPath="System" path="C:\work\report.docx" size=65536`,
			"path", `C:\work\report.docx`, core.ObservationKindStatusDetermined},
		{"session", `02/01/2000 10:08:09.700 +0900 sn=802 evt=session subEvt=loginR com="HOST01" ` +
			terminal + ` usr="HOST01$" usrDomain="EXAMPLE.TEST"`,
			"usrDomain", "EXAMPLE.TEST", core.ObservationKindStatusDetermined},
		{"reg", `02/01/2000 13:07:12.000 +0900 sn=803 evt=reg subEvt=create com="HOST01" ` +
			terminal + ` regPath="HKLM\SOFTWARE\Example"`,
			"regPath", `HKLM\SOFTWARE\Example`, core.ObservationKindStatusInferred},
		{"os", `02/01/2000 13:07:13.000 +0900 sn=804 evt=os subEvt=evtLog com="HOST01" ` +
			terminal + ` evtID=4624 wsName="-"`,
			"evtID", "4624", core.ObservationKindStatusDetermined},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			record := parseOne(t, pipeline.NewTestMarkIIParser(), tc.line)
			semantics := record.Semantics
			if semantics == nil {
				t.Fatalf("the record whose evt is %s carried no semantics", tc.name)
			}
			if semantics.ProcessRef != nil || semantics.Endpoint != nil || semantics.ParentProcessId != nil {
				t.Errorf("semantics carried a reference: %+v", semantics)
			}
			if semantics.ObservationKind.Status != tc.status {
				t.Errorf("status = %q, want %q", semantics.ObservationKind.Status, tc.status)
			}
			if tc.status == core.ObservationKindStatusInferred &&
				semantics.ObservationKind.Meaning == "" {
				t.Error("the inferred observation kind carried no meaning")
			}
			if err := semantics.ObservationKind.Validate(); err != nil {
				t.Error(err)
			}
			if got := comparableOf(t, textOf(t, semantics.Fields, tc.field)); got != tc.value {
				t.Errorf("%s = %q, want %q", tc.field, got, tc.value)
			}
			if record.ObservedAt == nil {
				t.Error("the record carried no observed time")
			}
		})
	}
}
