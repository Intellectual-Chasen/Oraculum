package markii_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/markii"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 通信のレコードだけを選ぶ。evt が net であることだけでは受けない。
func TestIsCommunicationSelectsOnlyTheFiveSubEvents(t *testing.T) {
	cases := map[string]struct {
		line string
		want bool
	}{
		"接続":               {communicationConnectLine, true},
		"接続の受け入れ":          {communicationAcceptLine, true},
		"切断":               {communicationCloseLine, true},
		"接続の成立":            {communicationEstablishLine, true},
		"UDP の口を開く":        {communicationOpenUdpLine, true},
		"net だが未知の subEvt": {communicationWithUnknownSubEventLine, false},
		"subEvt を欠く":       {communicationWithoutSubEventLine, false},
		"プロセスの起動":          {processStartLine, false},
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			if got := markii.IsCommunication(readOneOK(t, want.line)); got != want.want {
				t.Errorf("IsCommunication = %t, want %t", got, want.want)
			}
		})
	}
}

// 位置とプロセスと端末の識別の材料を持つ。
func TestParseCommunicationCarriesTheIdentifyingValues(t *testing.T) {
	got := parseCommunicationOK(t, communicationConnectLine)

	if got.SequenceNumber == nil {
		t.Fatal("the observation must carry the sequence number")
	}
	if *got.SequenceNumber != communicationSequenceNumber {
		t.Errorf("sequenceNumber = %d, want %d", *got.SequenceNumber, communicationSequenceNumber)
	}
	if got.LineNumber != 1 {
		t.Errorf("lineNumber = %d, want 1", got.LineNumber)
	}
	if got.ProcessGuid != communicationProcessGuid {
		t.Errorf("processGuid = %q, want %q", got.ProcessGuid, communicationProcessGuid)
	}
	if got.TerminalId != communicationTerminalId {
		t.Errorf("terminalId = %q, want %q", got.TerminalId, communicationTerminalId)
	}
}

// 接続の相手の 4 つを専用の欄で持つ。
func TestParseCommunicationCarriesTheEndpointsInTheirOwnItems(t *testing.T) {
	got := parseCommunicationOK(t, communicationConnectLine)

	cases := []struct {
		name  string
		value core.RawAndNormalized
		want  string
	}{
		{"srcIP", got.SourceIp, communicationSourceIp},
		{"srcPort", got.SourcePort, communicationSourcePort},
		{"dstIP", got.DestIp, communicationDestIp},
		{"dstPort", got.DestPort, communicationDestPort},
	}
	for _, want := range cases {
		if want.value.ValueState != core.ValueStatePresent {
			t.Errorf("%s valueState = %q, want %q", want.name, want.value.ValueState, core.ValueStatePresent)
			continue
		}
		if raw, ok := want.value.RawTextValue(); !ok || raw != want.want {
			t.Errorf("%s rawText = %q (present %t), want %q", want.name, raw, ok, want.want)
		}
	}
}

// port の文字列を整数へ直さない。原資料の文字列をそのまま持つ。
func TestParseCommunicationKeepsThePortsAsText(t *testing.T) {
	got := parseCommunicationOK(t, communicationConnectLine)

	if _, ok := got.SourcePort.NormalizedValue(); ok {
		t.Error("an unquoted port must not carry a normalized value")
	}
	if _, ok := got.DestPort.NormalizedValue(); ok {
		t.Error("an unquoted port must not carry a normalized value")
	}
}

// 接続の相手を持たないレコードでも観測を作る。欄の不在として持つ。
func TestParseCommunicationKeepsAbsentEndpointsAsAbsentItems(t *testing.T) {
	got := parseCommunicationOK(t, communicationOpenUdpLine)

	cases := map[string]core.RawAndNormalized{
		"srcIP":   got.SourceIp,
		"srcPort": got.SourcePort,
		"dstIP":   got.DestIp,
		"dstPort": got.DestPort,
	}
	for name, value := range cases {
		if value.ValueState != core.ValueStateItemAbsent {
			t.Errorf("%s valueState = %q, want %q", name, value.ValueState, core.ValueStateItemAbsent)
		}
		if _, ok := value.RawTextValue(); ok {
			t.Errorf("%s must not carry a raw text when the key does not appear", name)
		}
	}
}

// openUDP の port は専用の欄を持たず、項目の集合が原文の文字列のまま持つ。
//
// 接続元の port か接続先の port かを確定できないため、どちらの欄にも入れない。
func TestParseCommunicationLeavesTheOpenUdpPortInTheFields(t *testing.T) {
	got := parseCommunicationOK(t, communicationOpenUdpLine)

	port := communicationFieldNamed(t, got, "port")
	if raw, ok := port.Text.RawTextValue(); !ok || raw != "53" {
		t.Errorf("port rawText = %q (present %t), want %q", raw, ok, "53")
	}
}

// 通信量の 2 つは専用の欄を持たず、項目の集合が持つ。
func TestParseCommunicationLeavesTheByteCountsInTheFields(t *testing.T) {
	got := parseCommunicationOK(t, communicationCloseLine)

	received := communicationFieldNamed(t, got, "recv")
	if raw, ok := received.Text.RawTextValue(); !ok || raw != "0" {
		t.Errorf("recv rawText = %q (present %t), want %q", raw, ok, "0")
	}
	sent := communicationFieldNamed(t, got, "send")
	if raw, ok := sent.Text.RawTextValue(); !ok || raw != "1024" {
		t.Errorf("send rawText = %q (present %t), want %q", raw, ok, "1024")
	}
}

// 通信のレコードが定める 6 つの key は、値を持つレコードで present の項目になり、
// key が出ないレコードで item_absent の項目になる。期待値は
// testdata/communication-manifest.json の optionalKeysCompletedInFields が持つ。
func TestParseCommunicationCompletesTheOptionalKeysInTheFields(t *testing.T) {
	cases := map[string]struct {
		line       string
		present    map[string]string
		itemAbsent []string
	}{
		"接続": {
			line: communicationConnectLine,
			present: map[string]string{
				"srcIP": "192.0.2.10", "srcPort": "50417",
				"dstIP": "198.51.100.20", "dstPort": "443",
			},
			itemAbsent: []string{"recv", "send"},
		},
		"切断": {
			line: communicationCloseLine,
			present: map[string]string{
				"srcIP": "192.0.2.10", "srcPort": "50417",
				"dstIP": "198.51.100.20", "dstPort": "443",
				"recv": "0", "send": "1024",
			},
		},
		"UDP の口を開く": {
			line:       communicationOpenUdpLine,
			itemAbsent: []string{"srcIP", "srcPort", "dstIP", "dstPort", "recv", "send"},
		},
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			got := parseCommunicationOK(t, want.line)
			requireSingleFieldPerOptionalKey(t, got)
			for key, rawText := range want.present {
				field := communicationFieldNamed(t, got, key)
				if state := field.Text.ValueState; state != core.ValueStatePresent {
					t.Errorf("%s valueState = %q, want %q", key, state, core.ValueStatePresent)
				}
				if raw, ok := field.Text.RawTextValue(); !ok || raw != rawText {
					t.Errorf("%s rawText = %q (present %t), want %q", key, raw, ok, rawText)
				}
			}
			for _, key := range want.itemAbsent {
				requireItemAbsentField(t, communicationFieldNamed(t, got, key))
			}
		})
	}
}

// requireSingleFieldPerOptionalKey は 6 つの key が 1 件ずつ項目になることを確かめる。
func requireSingleFieldPerOptionalKey(t *testing.T, observation markii.Communication) {
	t.Helper()
	for _, key := range communicationOptionalKeyNames {
		count := 0
		for _, field := range observation.Fields {
			if field.Name == key {
				count++
			}
		}
		if count != 1 {
			t.Errorf("the fields carry %d items named %q, want 1", count, key)
		}
	}
}

// requireItemAbsentField は item_absent の項目が valueState だけを持つことを確かめる。
func requireItemAbsentField(t *testing.T, field core.RecordField) {
	t.Helper()
	if field.Text == nil {
		t.Fatalf("the field %q must carry a text value", field.Name)
	}
	if state := field.Text.ValueState; state != core.ValueStateItemAbsent {
		t.Errorf("%s valueState = %q, want %q", field.Name, state, core.ValueStateItemAbsent)
	}
	if raw, ok := field.Text.RawTextValue(); ok {
		t.Errorf("%s carries the raw text %q on an item_absent value", field.Name, raw)
	}
	if normalized, ok := field.Text.NormalizedValue(); ok {
		t.Errorf("%s carries the normalized value %q on an item_absent value", field.Name, normalized)
	}
	if derivation, ok := field.Text.DerivationValue(); ok {
		t.Errorf("%s carries the derivation %q on an item_absent value", field.Name, derivation)
	}
}

// 形式が意味を定める subEvt は意味が確定している。
func TestParseCommunicationDeterminesTheKindOfTheDocumentedSubEvents(t *testing.T) {
	cases := map[string]struct {
		line   string
		subEvt string
	}{
		"接続":      {communicationConnectLine, "con"},
		"接続の受け入れ": {communicationAcceptLine, "acpt"},
		"切断":      {communicationCloseLine, "dcon"},
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			got := parseCommunicationOK(t, want.line)
			if got.ObservationKind.Status != core.ObservationKindStatusDetermined {
				t.Errorf("observationKind status = %q, want %q",
					got.ObservationKind.Status, core.ObservationKindStatusDetermined)
			}
			if !got.ObservationKind.Matches(
				markii.ObservationKindSelectorOf("net", want.subEvt)) {
				t.Errorf("the observation kind must match the selector for %s", want.subEvt)
			}
		})
	}
}

// 形式が意味を定めない subEvt は、推定した意味として返す。
//
// **推定した意味の文字列も確かめる。** 通信の意味付けは observationKindOfCommunication が
// 種別を組み、種別に依らない観測とは別の経路になる。
func TestParseCommunicationMarksTheUndocumentedSubEventsInferred(t *testing.T) {
	cases := map[string]string{
		"接続の成立":     communicationEstablishLine,
		"UDP の口を開く": communicationOpenUdpLine,
	}
	for name, line := range cases {
		t.Run(name, func(t *testing.T) {
			got := parseCommunicationOK(t, line)
			if got.ObservationKind.Status != core.ObservationKindStatusInferred {
				t.Errorf("observationKind status = %q, want %q",
					got.ObservationKind.Status, core.ObservationKindStatusInferred)
			}
			if got.ObservationKind.Meaning == "" {
				t.Error("the inferred observation kind carries no meaning")
			}
			if err := got.ObservationKind.Validate(); err != nil {
				t.Error(err)
			}
		})
	}
}

// 観測の種別は evt と subEvt の 2 件を、この順で持つ。
func TestParseCommunicationBuildsTheObservationKindRaw(t *testing.T) {
	got := parseCommunicationOK(t, communicationConnectLine)

	if len(got.ObservationKind.Raw) != 2 {
		t.Fatalf("observationKind raw element count = %d, want 2", len(got.ObservationKind.Raw))
	}
	if got.ObservationKind.Raw[0].Name != "evt" || got.ObservationKind.Raw[1].Name != "subEvt" {
		t.Errorf("observationKind raw names = %q and %q, want evt and subEvt",
			got.ObservationKind.Raw[0].Name, got.ObservationKind.Raw[1].Name)
	}
}

// ヘッダーの 29 文字から Timestamp の 9 項目を組む。
func TestParseCommunicationBuildsTheHeaderTimestamp(t *testing.T) {
	got := parseCommunicationOK(t, communicationConnectLine)

	if raw, ok := got.EventTime.RawTextValue(); !ok || raw != communicationRawHeaderTime {
		t.Errorf("rawText = %q (present %t), want %q", raw, ok, communicationRawHeaderTime)
	}
	if normalized, ok := got.EventTime.NormalizedValue(); !ok || normalized != communicationNormalizedTime {
		t.Errorf("normalized = %q (present %t), want %q", normalized, ok, communicationNormalizedTime)
	}
	if got.EventTime.NormalizedForm != core.NormalizedFormRFC3339Absolute {
		t.Errorf("normalizedForm = %q, want %q",
			got.EventTime.NormalizedForm, core.NormalizedFormRFC3339Absolute)
	}
	if got.EventTime.Precision != core.PrecisionMillisecond {
		t.Errorf("precision = %q, want %q", got.EventTime.Precision, core.PrecisionMillisecond)
	}
	if got.EventTime.OffsetState != core.OffsetStateInValue {
		t.Errorf("offsetState = %q, want %q", got.EventTime.OffsetState, core.OffsetStateInValue)
	}
	if offset, ok := got.EventTime.OffsetTextValue(); !ok || offset != communicationOffsetText {
		t.Errorf("offsetText = %q (present %t), want %q", offset, ok, communicationOffsetText)
	}
	if got.EventTime.Clock != core.ClockTerminalLocal {
		t.Errorf("clock = %q, want %q", got.EventTime.Clock, core.ClockTerminalLocal)
	}
	if got.EventTime.ValueState != core.ValueStatePresent {
		t.Errorf("valueState = %q, want %q", got.EventTime.ValueState, core.ValueStatePresent)
	}
}

// 時刻の意味は event である。**そのレコードが記録した事象の時刻を表す。**
//
// 切断のレコードでも同じ意味を持つ。開始時刻を表す意味を入れる経路を持たない。
func TestParseCommunicationMarksTheTimeAsTheTimeOfTheEvent(t *testing.T) {
	for name, line := range map[string]string{
		"接続": communicationConnectLine,
		"切断": communicationCloseLine,
	} {
		t.Run(name, func(t *testing.T) {
			got := parseCommunicationOK(t, line)
			if got.EventTime.Meaning != core.MeaningEvent {
				t.Errorf("meaning = %q, want %q", got.EventTime.Meaning, core.MeaningEvent)
			}
		})
	}
}

// 関連付けに使う時刻を導ける。UTC からのずれが文字列にあるためである。
func TestParseCommunicationDerivesTheInstant(t *testing.T) {
	got := parseCommunicationOK(t, communicationConnectLine)

	instant, ok := got.EventTime.Instant()
	if !ok {
		t.Fatal("the header timestamp must derive an instant")
	}
	want := time.Date(2000, time.February, 1, 4, 30, 0, 500_000_000, time.UTC)
	if !instant.Equal(want) {
		t.Errorf("instant = %s, want %s", instant.UTC().Format(time.RFC3339Nano),
			want.Format(time.RFC3339Nano))
	}
}

// 引用符付きの value は引用符を外した正規化値を持ち、引用符無しの value は持たない。
func TestParseCommunicationNormalizesOnlyQuotedValues(t *testing.T) {
	got := parseCommunicationOK(t, communicationConnectLine)

	quoted := communicationFieldNamed(t, got, "psPath")
	if raw, ok := quoted.Text.RawTextValue(); !ok || raw != communicationRawPath {
		t.Errorf("psPath rawText = %q (present %t), want %q", raw, ok, communicationRawPath)
	}
	if normalized, ok := quoted.Text.NormalizedValue(); !ok || normalized != communicationPath {
		t.Errorf("psPath normalized = %q (present %t), want %q", normalized, ok, communicationPath)
	}

	unquoted := communicationFieldNamed(t, got, "dstPort")
	if _, ok := unquoted.Text.NormalizedValue(); ok {
		t.Error("an unquoted value must not carry a normalized value")
	}
}

// 引用符付きの srcIP は、専用の欄でも引用符を外した正規化値を持つ。
func TestParseCommunicationNormalizesAQuotedSourceIp(t *testing.T) {
	got := parseCommunicationOK(t, communicationWithQuotedSourceIpLine)

	if raw, ok := got.SourceIp.RawTextValue(); !ok || raw != `"192.0.2.10"` {
		t.Errorf("srcIP rawText = %q (present %t), want %q", raw, ok, `"192.0.2.10"`)
	}
	if normalized, ok := got.SourceIp.NormalizedValue(); !ok || normalized != communicationSourceIp {
		t.Errorf("srcIP normalized = %q (present %t), want %q", normalized, ok, communicationSourceIp)
	}
	if _, ok := got.SourceIp.DerivationValue(); !ok {
		t.Error("a normalized value must carry how it was derived")
	}
}

// 1 件目の項目はヘッダーの時刻で、名前は headerTime である。残りは原文の並び順を保つ。
func TestParseCommunicationPutsTheHeaderTimeFirst(t *testing.T) {
	got := parseCommunicationOK(t, communicationConnectLine)

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
	if got.Fields[1].Name != "loc" {
		t.Errorf("the second field is %q, want loc", got.Fields[1].Name)
	}
}

// 必須の key を欠いたレコードから観測を作らない。
func TestParseCommunicationReportsAMissingRequiredKey(t *testing.T) {
	observation, failure := markii.ParseCommunication(readOneOK(t, communicationWithoutPathLine))
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

// 必須の 8 key のどの 1 つが欠けても、値が空でも、2 回出ても観測を作らない。
//
// 1 つの key だけを検査に掛けると、一覧から別の key を外す変更が検査を通ってしまう。
// 8 key それぞれに 3 つの形を掛ける。
func TestParseCommunicationRequiresEachKeyOfTheList(t *testing.T) {
	// 先に、組み立てた行がそのままなら観測になることを確かめる。
	// これが成り立たないと、以下の失敗が本当に狙った形によるものか分からなくなる。
	if _, failure := markii.ParseCommunication(
		readOneOK(t, communicationLineWithout(t, ""))); failure != nil {
		t.Fatalf("the assembled record must parse: %+v", *failure)
	}

	for _, token := range communicationRequiredKeyTokens {
		forms := map[string]string{
			"欠ける":   communicationLineWithout(t, token.key),
			"値が空":   communicationLineReplacing(t, token.key, ""),
			"2 回出る": communicationLineWithout(t, "") + " " + token.key + "=" + token.value,
		}
		for form, line := range forms {
			t.Run(token.key+"/"+form, func(t *testing.T) {
				observation, failure := markii.ParseCommunication(readOneOK(t, line))
				if failure == nil {
					t.Fatal("the record must be reported as a failure")
				}
				if failure.Stage != core.FailureStageFieldMap {
					t.Errorf("stage = %q, want %q", failure.Stage, core.FailureStageFieldMap)
				}
				positionOnly := markii.Communication{
					SequenceNumber: observation.SequenceNumber,
					LineNumber:     observation.LineNumber,
				}
				if !reflect.DeepEqual(observation, positionOnly) {
					t.Errorf("a failed observation carries more than its position: %+v", observation)
				}
			})
		}
	}
}

// communicationLineWithout は必須の key を 1 つ外した行を組み立てる。
// 空の名前を渡すと 8 key をすべて持つ行を返す。
func communicationLineWithout(t *testing.T, omitted string) string {
	t.Helper()
	line := communicationHeaderText
	for _, token := range communicationRequiredKeyTokens {
		if token.key == omitted {
			continue
		}
		line += " " + token.key + "=" + token.value
	}
	return line
}

// communicationLineReplacing は必須の key の 1 つを別の値へ置き換えた行を組み立てる。
func communicationLineReplacing(t *testing.T, replaced, value string) string {
	t.Helper()
	line := communicationHeaderText
	for _, token := range communicationRequiredKeyTokens {
		if token.key == replaced {
			line += " " + token.key + "=" + value
			continue
		}
		line += " " + token.key + "=" + token.value
	}
	return line
}

// 必須の key が 2 回出るレコードから観測を作らない。先勝ちで採らない。
func TestParseCommunicationReportsARepeatedRequiredKey(t *testing.T) {
	_, failure := markii.ParseCommunication(readOneOK(t, communicationWithRepeatedKeyLine))
	if failure == nil {
		t.Fatal("a record with a repeated required key must be reported as a failure")
	}
	if failure.Stage != core.FailureStageFieldMap {
		t.Errorf("stage = %q, want %q", failure.Stage, core.FailureStageFieldMap)
	}
}

// sn を読めないレコードでは通番を持たない。代用の値を入れない。
func TestParseCommunicationLeavesTheSequenceNumberUnsetWhenItCannotBeRead(t *testing.T) {
	observation, failure := markii.ParseCommunication(readOneOK(t, communicationWithBadSequenceLine))
	if failure == nil {
		t.Fatal("a sequence number that is not a decimal integer must be reported as a failure")
	}
	if observation.SequenceNumber != nil {
		t.Errorf("sequenceNumber = %d, want it unset", *observation.SequenceNumber)
	}
}

// 日時として解釈できないヘッダーは normalize の失敗になる。先に読めた通番は持つ。
func TestParseCommunicationReportsAHeaderTimeItCannotRead(t *testing.T) {
	observation, failure := markii.ParseCommunication(readOneOK(t, communicationWithBadHeaderTimeLine))
	if failure == nil {
		t.Fatal("a header time that is not a date and time must be reported as a failure")
	}
	if failure.Stage != core.FailureStageNormalize {
		t.Errorf("stage = %q, want %q", failure.Stage, core.FailureStageNormalize)
	}
	if failure.LineNumber == nil {
		t.Error("the failure must carry the line number")
	}
	if observation.SequenceNumber == nil {
		t.Fatal("a failure after reading the sequence number must carry it")
	}
	if *observation.SequenceNumber != 900307 {
		t.Errorf("sequenceNumber = %d, want 900307", *observation.SequenceNumber)
	}
}

// 受け付けない subEvt を持つレコードから観測を作らない。
//
// 確かめていない形を通知せずに通すと、別のバージョンが足した種別を意味を確かめずに観測にする。
func TestParseCommunicationRejectsAnUnknownSubEvent(t *testing.T) {
	_, failure := markii.ParseCommunication(readOneOK(t, communicationWithUnknownSubEventLine))
	if failure == nil {
		t.Fatal("a subEvt the parser does not read must be reported as a failure")
	}
	if failure.Stage != core.FailureStageFieldMap {
		t.Errorf("stage = %q, want %q", failure.Stage, core.FailureStageFieldMap)
	}
}

// 他の種別のレコードを渡した呼び出しは失敗を返す。
func TestParseCommunicationRejectsARecordOfAnotherKind(t *testing.T) {
	_, failure := markii.ParseCommunication(readOneOK(t, processStartLine))
	if failure == nil {
		t.Fatal("a record of another kind must be reported as a failure")
	}
	if failure.Stage != core.FailureStageFieldMap {
		t.Errorf("stage = %q, want %q", failure.Stage, core.FailureStageFieldMap)
	}
}

// 解析は 4 項目を埋めない。埋めるのは取り込みの実行である。
func TestParseCommunicationLeavesTheItemsItCannotKnowEmpty(t *testing.T) {
	_, failure := markii.ParseCommunication(readOneOK(t, communicationWithoutPathLine))
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

// 取り込みの実行が sourceId と sourceContentSha256 を足すと、core の型が Validate を通る。
func TestTheItemsTheImportRunAddsCompleteTheCommunicationTypes(t *testing.T) {
	got := parseCommunicationOK(t, communicationConnectLine)

	const (
		sourceId = "source-2"
		// 小文字 16 進 64 桁であることだけが検査の対象になる。
		sha256 = "dd44ee55ff6600778899aa11bb22cc33dd44ee55ff6600778899aa11bb22cc33"
	)
	locator := core.RecordLocator{
		SourceId:            sourceId,
		SourceContentSha256: sha256,
		SourceFileName:      "example.log",
		PositionKind:        core.PositionKindSequenceNumber,
		SequenceNumber:      got.SequenceNumber,
		LineNumber:          &got.LineNumber,
		RecordRawTextRef:    "/api/v0/records?sourceId=source-2&sequenceNumber=900300",
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

	if err := got.EventTime.Validate(); err != nil {
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

// parseCommunicationOK は 1 行を文字列に分割して解析し、成功した観測を返す。
func parseCommunicationOK(t *testing.T, line string) markii.Communication {
	t.Helper()
	got, failure := markii.ParseCommunication(readOneOK(t, line))
	if failure != nil {
		t.Fatalf("parsing the communication record: %+v", *failure)
	}
	return got
}

// communicationFieldNamed は名前で項目を探す。
func communicationFieldNamed(t *testing.T, observation markii.Communication, name string) core.RecordField {
	t.Helper()
	for _, field := range observation.Fields {
		if field.Name == name {
			return field
		}
	}
	t.Fatalf("the observation must carry the field %q", name)
	return core.RecordField{}
}
