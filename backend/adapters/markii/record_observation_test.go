package markii_test

import (
	"strconv"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/markii"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// parseRecordObservationOK は 1 行を読み、意味付けが成功した観測を返す。
func parseRecordObservationOK(t *testing.T, line string) markii.RecordObservation {
	t.Helper()
	observation, failure := markii.ParseRecordObservation(readOneOK(t, line))
	if failure != nil {
		t.Fatalf("reading the record observation: %+v", *failure)
	}
	return observation
}

// textValueOfField は名前で指した項目の比較に使う値を返す。
func textValueOfField(t *testing.T, fields []core.RecordField, name string) string {
	t.Helper()
	for _, field := range fields {
		if field.Name != name {
			continue
		}
		if field.Text == nil {
			t.Fatalf("the field %q carries no text value", name)
		}
		value, ok := field.Text.ComparableValue()
		if !ok {
			t.Fatalf("the field %q carries nothing to compare: %+v", name, *field.Text)
		}
		return value
	}
	t.Fatalf("the field %q is absent from the set", name)
	return ""
}

// Recorder が出力する組のそれぞれで、意味の状態が組を載せた表に一致し、undetermined の組が
// 残らない。期待値は testdata/record-observation-manifest.json の observedKinds が持つ。
func TestParseRecordObservationStatusOfEveryObservedEventKind(t *testing.T) {
	inferred := map[string]struct{}{}
	for _, kind := range inferredObservedEventKinds {
		inferred[kind[0]+"/"+kind[1]] = struct{}{}
	}
	for index, kind := range observedEventKinds {
		name := kind.event + "/" + kind.subEvent
		_, listed := inferred[name]
		if listed != (kind.status == core.ObservationKindStatusInferred) {
			t.Errorf("%s: the inferred list says %t, the kind table says %q",
				name, listed, kind.status)
		}
		if kind.status == core.ObservationKindStatusUndetermined {
			t.Errorf("%s: the kind table leaves an observed pair undetermined", name)
		}
		delete(inferred, name)
		t.Run(kind.event+"/"+kind.subEvent, func(t *testing.T) {
			line := "02/01/2000 03:04:05.678 +0900 sn=" + strconv.Itoa(700000+index) +
				" evt=" + kind.event + " subEvt=" + kind.subEvent +
				" com=\"HOST01\" tmid=" + recordObservationTerminal
			got := parseRecordObservationOK(t, line)

			if err := got.ObservationKind.Validate(); err != nil {
				t.Fatal(err)
			}
			if got.ObservationKind.Status != kind.status {
				t.Errorf("status = %q, want %q", got.ObservationKind.Status, kind.status)
			}
			if len(got.ObservationKind.Raw) != 2 {
				t.Fatalf("observation kind raw = %+v, want 2 items", got.ObservationKind.Raw)
			}
			if value := textValueOfField(t, got.ObservationKind.Raw, "evt"); value != kind.event {
				t.Errorf("evt = %q, want %q", value, kind.event)
			}
			if value := textValueOfField(t, got.ObservationKind.Raw, "subEvt"); value != kind.subEvent {
				t.Errorf("subEvt = %q, want %q", value, kind.subEvent)
			}
		})
	}
	for name := range inferred {
		t.Errorf("the inferred list names %s, which the kind table omits", name)
	}
}

// 本 package が返す inferred の種別は、Recorder が出力する組すべてで意味を持つ。core は
// inferred の種別に意味を必須とする (core の validateMeaning)。
func TestParseRecordObservationFillsTheMeaningOfEveryInferredKind(t *testing.T) {
	checked := 0
	for index, kind := range observedEventKinds {
		if kind.status != core.ObservationKindStatusInferred {
			continue
		}
		checked++
		t.Run(kind.event+"/"+kind.subEvent, func(t *testing.T) {
			line := "02/01/2000 03:04:05.678 +0900 sn=" + strconv.Itoa(710000+index) +
				" evt=" + kind.event + " subEvt=" + kind.subEvent +
				" com=\"HOST01\" tmid=" + recordObservationTerminal
			got := parseRecordObservationOK(t, line).ObservationKind

			if got.Status != core.ObservationKindStatusInferred {
				t.Fatalf("status = %q, want %q", got.Status, core.ObservationKindStatusInferred)
			}
			if got.Meaning == "" {
				t.Error("the inferred kind carries no meaning")
			}
		})
	}
	// 1 件も掛けていない状態を成功にしない。
	if checked != len(inferredObservedEventKinds) {
		t.Errorf("checked %d inferred kinds, want the %d the list names",
			checked, len(inferredObservedEventKinds))
	}
}

// 推定した組は、推定した意味を応答へ載せる。形式が意味を定めた組は載せない。
func TestParseRecordObservationCarriesTheInferredMeaning(t *testing.T) {
	inferredLine := "02/01/2000 03:04:05.678 +0900 sn=790100 evt=file subEvt=create " +
		"com=\"HOST01\" tmid=" + recordObservationTerminal
	inferred := parseRecordObservationOK(t, inferredLine).ObservationKind
	if inferred.Status != core.ObservationKindStatusInferred {
		t.Fatalf("status = %q, want %q", inferred.Status, core.ObservationKindStatusInferred)
	}
	if inferred.Meaning == "" {
		t.Error("the inferred kind carries no meaning")
	}
	if err := inferred.Validate(); err != nil {
		t.Error(err)
	}

	documentedLine := "02/01/2000 03:04:05.678 +0900 sn=790101 evt=file subEvt=close " +
		"com=\"HOST01\" tmid=" + recordObservationTerminal
	documented := parseRecordObservationOK(t, documentedLine).ObservationKind
	if documented.Status != core.ObservationKindStatusDetermined {
		t.Fatalf("status = %q, want %q", documented.Status, core.ObservationKindStatusDetermined)
	}
	if documented.Meaning != "" {
		t.Errorf("the documented kind carries the meaning %q, want none", documented.Meaning)
	}
}

// どちらの表にも無い組は undetermined になる。組はテスト用であり、Recorder が出力しない文字列を使う。
func TestParseRecordObservationLeavesAnUnknownKindUndetermined(t *testing.T) {
	for _, kind := range []struct{ event, subEvent string }{
		// どちらの表にも無い evt。
		{"exampleEvent", "exampleSubEvent"},
		// 表にある evt と、その evt の表に無い subEvt。
		{"file", "exampleSubEvent"},
		// 推定した組を持つ evt と、その evt のどちらの表にも無い subEvt。
		{"net", "exampleSubEvent"},
	} {
		t.Run(kind.event+"/"+kind.subEvent, func(t *testing.T) {
			line := "02/01/2000 03:04:05.678 +0900 sn=790000 evt=" + kind.event +
				" subEvt=" + kind.subEvent +
				" com=\"HOST01\" tmid=" + recordObservationTerminal
			got := parseRecordObservationOK(t, line)

			if err := got.ObservationKind.Validate(); err != nil {
				t.Fatal(err)
			}
			if got.ObservationKind.Status != core.ObservationKindStatusUndetermined {
				t.Errorf("status = %q, want %q",
					got.ObservationKind.Status, core.ObservationKindStatusUndetermined)
			}
		})
	}
}

// evt が file のレコードは path を項目として持つ。
func TestParseRecordObservationCarriesTheFileEventFields(t *testing.T) {
	got := parseRecordObservationOK(t, fileCloseLine)

	if got.SequenceNumber == nil || *got.SequenceNumber != fileCloseSequenceNumber {
		t.Fatalf("sequenceNumber = %v, want %d", got.SequenceNumber, fileCloseSequenceNumber)
	}
	if value := textValueOfField(t, got.Fields, "path"); value != fileClosePath {
		t.Errorf("path = %q, want %q", value, fileClosePath)
	}
	if value := textValueOfField(t, got.Fields, "psPath"); value != `C:\Tools\writer.exe` {
		t.Errorf("psPath = %q, want %q", value, `C:\Tools\writer.exe`)
	}
	if got.ObservationKind.Status != core.ObservationKindStatusDetermined {
		t.Errorf("status = %q, want %q",
			got.ObservationKind.Status, core.ObservationKindStatusDetermined)
	}
}

// evt が session のレコードは usr と usrDomain を項目として持つ。
// psGUID と psPath を持たないレコードからも観測を作る。
func TestParseRecordObservationCarriesTheSessionEventFields(t *testing.T) {
	got := parseRecordObservationOK(t, sessionLoginRLine)

	if got.SequenceNumber == nil || *got.SequenceNumber != sessionLoginRSequenceNum {
		t.Fatalf("sequenceNumber = %v, want %d", got.SequenceNumber, sessionLoginRSequenceNum)
	}
	if value := textValueOfField(t, got.Fields, "usr"); value != sessionLoginRUser {
		t.Errorf("usr = %q, want %q", value, sessionLoginRUser)
	}
	if value := textValueOfField(t, got.Fields, "usrDomain"); value != sessionLoginRUserDomain {
		t.Errorf("usrDomain = %q, want %q", value, sessionLoginRUserDomain)
	}
	for _, name := range []string{"psGUID", "psPath"} {
		for _, field := range got.Fields {
			if field.Name == name {
				t.Errorf("the field %q must be absent from the set", name)
			}
		}
	}
}

// 1 件目はヘッダーの時刻で、名前は headerTime である。残りは原文の並び順を
// 保つ。**当該レコードに出ない key の項目を足さない。**
func TestParseRecordObservationKeepsTheRecordKeysInOrder(t *testing.T) {
	got := parseRecordObservationOK(t, fileCloseLine)

	header := got.Fields[0]
	if header.Name != "headerTime" || header.Timestamp == nil {
		t.Fatalf("the first field = %+v", header)
	}
	if header.Timestamp.Normalized == nil || *header.Timestamp.Normalized != fileCloseNormalizedTime {
		t.Errorf("headerTime = %+v, want %q", header.Timestamp, fileCloseNormalizedTime)
	}
	wantOrder := []string{"loc", "type", "sn", "lv", "evt", "subEvt", "os", "com"}
	for index, name := range wantOrder {
		if got.Fields[index+1].Name != name {
			t.Errorf("the field at %d = %q, want %q", index+1, got.Fields[index+1].Name, name)
		}
	}
	// 通信のレコードだけが補う 6 key は、evt が file のレコードの項目に入らない。
	for _, name := range []string{"srcIP", "srcPort", "dstIP", "dstPort", "recv", "send"} {
		for _, field := range got.Fields {
			if field.Name == name {
				t.Errorf("the field %q must be absent from the set", name)
			}
		}
	}
}

// 必須の key を欠いたレコード、値が空のレコード、key が 2 回出るレコードから観測を
// 作らない。ヘッダーを日時として読めないレコードは normalize の失敗になる。
func TestParseRecordObservationRejectsTheRecordsItCannotRead(t *testing.T) {
	cases := map[string]struct {
		line  string
		stage core.FailureStage
	}{
		"sn を欠く":       {fileCloseWithoutSequenceLine, core.FailureStageFieldMap},
		"evt を欠く":      {fileCloseWithoutEventLine, core.FailureStageFieldMap},
		"subEvt を欠く":   {fileCloseWithoutSubEventLine, core.FailureStageFieldMap},
		"subEvt の値が空":  {fileCloseWithEmptySubEventLine, core.FailureStageFieldMap},
		"subEvt が 2 回": {fileCloseWithRepeatedSubEventLine, core.FailureStageFieldMap},
		"ヘッダーを読めない":    {fileCloseWithBadHeaderTimeLine, core.FailureStageNormalize},
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			observation, failure := markii.ParseRecordObservation(readOneOK(t, want.line))
			if failure == nil {
				t.Fatalf("the record must be rejected: %+v", observation)
			}
			if failure.Stage != want.stage {
				t.Errorf("stage = %q, want %q", failure.Stage, want.stage)
			}
			if failure.DiagnosisClass != core.DiagnosisClassUndetermined {
				t.Errorf("diagnosisClass = %q, want %q",
					failure.DiagnosisClass, core.DiagnosisClassUndetermined)
			}
			if failure.LineNumber == nil || *failure.LineNumber != 1 {
				t.Errorf("lineNumber = %v, want 1", failure.LineNumber)
			}
			if len(observation.Fields) != 0 || observation.LineNumber != 1 {
				t.Errorf("the rejected observation = %+v", observation)
			}
		})
	}
}

// tmid を欠くレコードからも観測を作る。本型が tmid を持つ欄を持たないため、必須にしない。
func TestParseRecordObservationReadsTheRecordWithoutATerminalId(t *testing.T) {
	got := parseRecordObservationOK(t, fileCloseWithoutTerminalIdLine)

	if got.SequenceNumber == nil || *got.SequenceNumber != 800103 {
		t.Fatalf("sequenceNumber = %v, want 800103", got.SequenceNumber)
	}
	if value := textValueOfField(t, got.Fields, "path"); value != fileClosePath {
		t.Errorf("path = %q, want %q", value, fileClosePath)
	}
	for _, field := range got.Fields {
		if field.Name == "tmid" {
			t.Errorf("the field %q must be absent from the set", field.Name)
		}
	}
}

// 失敗のときも読めた位置を返す。ヘッダーを読めない失敗では通番まで読めている。
func TestParseRecordObservationKeepsThePositionItRead(t *testing.T) {
	observation, failure := markii.ParseRecordObservation(
		readOneOK(t, fileCloseWithBadHeaderTimeLine))
	if failure == nil {
		t.Fatal("the record must be rejected")
	}
	if observation.SequenceNumber == nil || *observation.SequenceNumber != 800106 {
		t.Errorf("sequenceNumber = %v, want 800106", observation.SequenceNumber)
	}
	// sn を読む前に止まった失敗では通番が nil のままである。
	withoutSequence, failure := markii.ParseRecordObservation(
		readOneOK(t, fileCloseWithoutSequenceLine))
	if failure == nil {
		t.Fatal("the record without sn must be rejected")
	}
	if withoutSequence.SequenceNumber != nil {
		t.Errorf("sequenceNumber = %v, want none", *withoutSequence.SequenceNumber)
	}
}

// 成功した観測のすべての項目が core の検査を通る。
func TestParseRecordObservationBuildsValidFields(t *testing.T) {
	for _, line := range []string{fileCloseLine, sessionLoginRLine} {
		got := parseRecordObservationOK(t, line)
		if len(got.Fields) == 0 {
			t.Fatalf("the observation carries no field: %+v", got)
		}
		for _, field := range got.Fields {
			if err := field.Validate(); err != nil {
				t.Errorf("the field %q: %v", field.Name, err)
			}
		}
		if err := got.EventTime.Validate(); err != nil {
			t.Error(err)
		}
	}
}
