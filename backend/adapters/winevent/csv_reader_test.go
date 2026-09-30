package winevent_test

import (
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/winevent"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// イベントビューアーの CSV の見出し。見出しの文字列は Windows が書く文字列である。
const (
	levelHeader   = "レベル,日付と時刻,ソース,イベント ID,タスクのカテゴリ\n"
	keywordHeader = "キーワード,日付と時刻,ソース,イベント ID,タスクのカテゴリ\n"
)

// プロセスの作成の説明。項目名は
// Windows が 4688 の説明に書く文字列である。
const processCreationDescription = "新しいプロセスが作成されました。\n\n" +
	"サブジェクト:\n" +
	"\tセキュリティ ID:\t\tCORP-TEST\\analyst01\n" +
	"\tアカウント名:\t\tHOST01$\n" +
	"\tアカウント ドメイン:\t\tCORP-TEST\n" +
	"\tログオン ID:\t\t0x3e7\n\n" +
	"プロセス情報:\n" +
	"\t新しいプロセス ID:\t\t0x2b\n" +
	"\t新しいプロセス名:\t\tC:\\Example\\child.exe\n" +
	"\tクリエーター プロセス ID:\t0x1f\n\n" +
	"合成した説明の文です。次の値: 項目にしない"

// viewerRecord は 1 件の論理レコードを CSV の 1 行目から組む。説明は引用符で囲む。
func viewerRecord(first, at, provider, eventID, task, description string) string {
	return first + "," + at + "," + provider + "," + eventID + "," + task + `,"` +
		strings.ReplaceAll(description, `"`, `""`) + "\"\n"
}

func readAllCSV(t *testing.T, document string) ([]winevent.Event, []*core.ImportFailure) {
	t.Helper()
	var reader winevent.CSVReader
	reader.Reset(strings.NewReader(document))
	var events []winevent.Event
	var failures []*core.ImportFailure
	for range 100 {
		event, failure, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return events, failures
		}
		if err != nil {
			t.Fatalf("Next() returned error %v", err)
		}
		if failure != nil {
			failures = append(failures, failure)
			continue
		}
		events = append(events, event)
	}
	t.Fatal("Next() did not reach the end of the input")
	return nil, nil
}

func valueNamed(t *testing.T, values []winevent.Value, name string) string {
	t.Helper()
	for _, value := range values {
		if value.Name == name {
			return value.Text
		}
	}
	t.Fatalf("no value named %q in %+v", name, values)
	return ""
}

func TestCSVReaderReadsEveryRecordWithItsPosition(t *testing.T) {
	first := viewerRecord("情報", "2001/02/03 04:05:06", "Microsoft-Windows-Security-Auditing", "4688",
		"プロセス作成", processCreationDescription)
	// 同じ秒に同じイベント ID のレコードが 2 件ある。
	second := viewerRecord("情報", "2001/02/03 04:05:06", "Microsoft-Windows-Security-Auditing", "4688",
		"プロセス作成", strings.Replace(processCreationDescription, "0x2b", "0x2c", 1))
	third := viewerRecord("警告", "2001/2/3 4:05:07", "Example-Provider", "7", "",
		"Example Event:\nRuleName: \nImage: C:\\Example\\other.exe")
	document := "\ufeff" + levelHeader + first + second + third
	events, failures := readAllCSV(t, document)
	if len(failures) != 0 || len(events) != 3 {
		t.Fatalf("events = %d, failures = %+v; want 3 events", len(events), failures)
	}
	for at, want := range []struct {
		raw  string
		line int64
	}{{first, 2}, {second, 2 + int64(strings.Count(first, "\n"))},
		{third, 2 + int64(strings.Count(first+second, "\n"))}} {
		source := events[at].Source
		offset := int64(strings.Index(document, want.raw))
		if source.RawText != want.raw || source.ByteOffset != offset || source.ByteLength != int64(len(want.raw)) ||
			source.LineNumber != want.line || source.LineCount != int64(strings.Count(want.raw, "\n")) {
			t.Errorf("record %d source = %+v, want the record at byte %d on line %d", at, source, offset, want.line)
		}
	}
	creation := events[0]
	if textOf(t, creation.System.ProviderName) != "Microsoft-Windows-Security-Auditing" ||
		textOf(t, creation.System.EventID) != "4688" || textOf(t, creation.System.SystemTime) != "2001/02/03 04:05:06" {
		t.Errorf("System = %+v", creation.System)
	}
	for name, want := range map[string]string{
		"RenderingInfo.Level": "情報", "RenderingInfo.Task": "プロセス作成", "RenderingInfo.Message": processCreationDescription,
	} {
		if got := valueNamed(t, creation.Sections, name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	for name, want := range map[string]string{
		// Windows の項目名へ写した項目。
		"SubjectUserName": "HOST01$", "SubjectDomainName": "CORP-TEST",
		"NewProcessId": "0x2b", "NewProcessName": `C:\Example\child.exe`, "ProcessId": "0x1f",
		"SubjectLogonId": "0x3e7",
		// 写さない項目は見出しと項目名を名前にする。
		"サブジェクト.セキュリティ ID": `CORP-TEST\analyst01`,
	} {
		if got := valueNamed(t, creation.EventData, name); got != want {
			t.Errorf("EventData %s = %q, want %q", name, got, want)
		}
	}
	for _, value := range creation.EventData {
		if strings.Contains(value.Name, "。") {
			t.Errorf("a sentence became the item %q", value.Name)
		}
	}
	if creation.MessageUnrendered {
		t.Error("a rendered description is marked unrendered")
	}
	if valueNamed(t, events[1].EventData, "NewProcessId") != "0x2c" {
		t.Error("the second record of the same second carries the values of the first")
	}
	// 字下げしない項目は項目名だけを名前にする。
	if valueNamed(t, events[2].EventData, "Image") != `C:\Example\other.exe` {
		t.Errorf("EventData = %+v, want Image", events[2].EventData)
	}
}

// 値が空で字下げの無い行は、次の行が字下げを持つときだけ見出しになる。次の行が字下げを
// 持たない行は、値の空の項目として残る。
func TestCSVReaderKeepsAnEmptyItemFollowedByAnUnindentedLine(t *testing.T) {
	events, _ := readAllCSV(t, levelHeader+viewerRecord("情報", "2001/02/03 04:05:06", "Example-Provider", "1", "",
		"Example Event:\nRuleName: \nImage: C:\\Example\\other.exe\n\n見出し:\n\t項目:\t値"))
	data := events[0].EventData
	if valueNamed(t, data, "RuleName") != "" || valueNamed(t, data, "Image") != `C:\Example\other.exe` ||
		valueNamed(t, data, "見出し.項目") != "値" {
		t.Errorf("EventData = %+v, want RuleName kept as an empty item", data)
	}
	for _, value := range data {
		if value.Name == "見出し" || strings.HasPrefix(value.Name, "RuleName.") {
			t.Errorf("EventData carries %q", value.Name)
		}
	}
}

// 項目の後ろに続く、字下げした `名前:` を持たない行は、同じ項目の値である。説明の文の後ろに
// 続く字下げした行は値にしない。
func TestCSVReaderKeepsEveryLineOfAMultilineItem(t *testing.T) {
	description := "特別な特権が割り当てられました。\n\n" +
		"サブジェクト:\n\tアカウント名:\t\texample.user\n\n" +
		"特権:\t\tSeFirstExamplePrivilege\n\t\t\tSeSecondExamplePrivilege\n\t\t\tSeThirdExamplePrivilege\n\n" +
		"合成した説明の文です。\n\t- 値にしない行"
	events, _ := readAllCSV(t, levelHeader+viewerRecord("情報", "2001/02/03 04:05:06",
		"Microsoft-Windows-Security-Auditing", "4672", "特殊ログオン", description))
	var privileges []string
	for _, value := range events[0].EventData {
		if value.Name == "特権" {
			privileges = append(privileges, value.Text)
		}
		if strings.HasPrefix(value.Text, "- ") {
			t.Errorf("EventData carries the sentence line %+v", value)
		}
	}
	want := []string{"SeFirstExamplePrivilege", "SeSecondExamplePrivilege", "SeThirdExamplePrivilege"}
	if !slices.Equal(privileges, want) {
		t.Errorf("特権 = %q, want %q", privileges, want)
	}
}

// 1 行に 1 つの値を持つ一覧でない項目の続きの行は、改行でつないで 1 つの値にする。
func TestCSVReaderJoinsContinuedLinesOfASingleValue(t *testing.T) {
	description := "新しいプロセスが作成されました。\n\n" +
		"プロセス情報:\n\tプロセスのコマンド ライン:\texample.exe -first\n\t-second\n\t-third\n"
	events, _ := readAllCSV(t, levelHeader+viewerRecord("情報", "2001/02/03 04:05:06",
		"Microsoft-Windows-Security-Auditing", "4688", "プロセス作成", description))
	var commandLines []string
	for _, value := range events[0].EventData {
		if value.Name == "CommandLine" {
			commandLines = append(commandLines, value.Text)
		}
	}
	want := []string{"example.exe -first\n-second\n-third"}
	if !slices.Equal(commandLines, want) {
		t.Errorf("CommandLine = %q, want %q", commandLines, want)
	}
}

// 4688 の親のプロセス番号は、Windows のバージョンごとの 3 つの項目名のどれでも ProcessId になる。
func TestCSVReaderMapsEveryCreatorProcessIdLabel(t *testing.T) {
	for _, label := range []string{"クリエーター プロセス ID", "クリエータ プロセス ID", "作成元プロセス ID"} {
		description := strings.Replace(processCreationDescription, "クリエーター プロセス ID", label, 1)
		events, _ := readAllCSV(t, levelHeader+viewerRecord("情報", "2001/02/03 04:05:06",
			"Microsoft-Windows-Security-Auditing", "4688", "プロセス作成", description))
		found := false
		for _, value := range events[0].EventData {
			found = found || (value.Name == "ProcessId" && value.Text == "0x1f")
		}
		if !found {
			t.Errorf("%q: EventData = %+v, want ProcessId", label, events[0].EventData)
		}
	}
}

func TestObserveMapsAViewerProcessCreation(t *testing.T) {
	events, _ := readAllCSV(t, levelHeader+viewerRecord("情報", "2001/02/03 04:05:06",
		"Microsoft-Windows-Security-Auditing", "4688", "プロセス作成", processCreationDescription))
	observation, failure := winevent.Observe(events[0])
	if failure != nil {
		t.Fatalf("Observe() failure = %+v", failure)
	}
	for name, semantic := range map[string]core.SemanticKey{
		"EventData.NewProcessId":      core.SemanticKeyProcessPid,
		"EventData.NewProcessName":    core.SemanticKeyProcessBinaryPath,
		"EventData.ProcessId":         core.SemanticKeyParentProcessPid,
		"EventData.SubjectUserName":   core.SemanticKeySubjectAccountName,
		"EventData.SubjectDomainName": core.SemanticKeySubjectAccountDomain,
		// ソースとイベント ID は XML と同じ System の項目になる。
		"Provider@Name":         core.SemanticKeyWindowsEventProvider,
		"EventID":               core.SemanticKeyWindowsEventId,
		"RenderingInfo.Message": core.SemanticKeyEventMessage,
	} {
		if field := fieldNamed(t, observation.Fields, name); field.Semantic != semantic {
			t.Errorf("%s semantic = %q, want %q", name, field.Semantic, semantic)
		}
	}
	if !observation.ProcessStart {
		t.Error("ProcessStart is false for a process creation")
	}
	// 地方時の文字列を保ち、UTC からのずれを補わない。
	timestamp := observation.EventTime
	if timestamp == nil {
		t.Fatal("EventTime is absent")
	}
	raw, _ := timestamp.RawTextValue()
	normalized, _ := timestamp.NormalizedValue()
	_, hasInstant := timestamp.Instant()
	if raw != "2001/02/03 04:05:06" || normalized != "2001-02-03T04:05:06" || hasInstant ||
		timestamp.OffsetState != core.OffsetStateUndetermined || timestamp.Precision != core.PrecisionSecond {
		t.Errorf("EventTime = %+v, want the local time without an offset", timestamp)
	}
	if !slices.Equal(observation.TerminalCandidates, []string{"HOST01$"}) || len(observation.Terminal) != 0 {
		t.Errorf("TerminalCandidates = %v, Terminal = %+v; want the machine account as a candidate only",
			observation.TerminalCandidates, observation.Terminal)
	}
}

// 管理用の共有の名前は、コンピューターのアカウント名と同じ形でも端末の候補にしない。
func TestObserveLeavesAdministrativeSharesOutOfTheCandidates(t *testing.T) {
	events, _ := readAllCSV(t, levelHeader+viewerRecord("情報", "2001/02/03 04:05:06", "Example-Provider", "1", "",
		"共有:\n\t共有名:\t\t\\\\*\\IPC$\n\t共有のパス:\t\\\\*\\C$\n\tアカウント名:\t\tCORP-TEST\\HOST02$"))
	observation, _ := winevent.Observe(events[0])
	if !slices.Equal(observation.TerminalCandidates, []string{"HOST02$"}) {
		t.Errorf("TerminalCandidates = %v, want only the machine account", observation.TerminalCandidates)
	}
}

// 説明を組めなかったレコードは、欄名の無い 1 つの値に原資料の文字列を保つ。
func TestCSVReaderKeepsAnUnrenderedDescriptionWithoutNames(t *testing.T) {
	inserted := "2001-02-03 04:05:06.789\nvalue: with a colon\n\t\tindented\n\nエラーの文。\n"
	description := `ソース "Example-Provider" からのイベント ID 1 の説明が見つかりません。合成した文。` + "\n\n" +
		"イベントには次の情報が含まれています: \n\n" + inserted
	events, failures := readAllCSV(t, levelHeader+viewerRecord("情報", "2001/02/03 04:05:06",
		"Example-Provider", "1", "", description))
	if len(failures) != 0 || len(events) != 1 {
		t.Fatalf("events = %d, failures = %+v; want 1 event", len(events), failures)
	}
	event := events[0]
	if !event.MessageUnrendered {
		t.Error("MessageUnrendered is false")
	}
	if len(event.EventData) != 1 || event.EventData[0].Name != "" || event.EventData[0].Text != inserted {
		t.Errorf("EventData = %+v, want one unnamed value of the inserted strings", event.EventData)
	}
	observation, _ := winevent.Observe(event)
	if !observation.MessageUnrendered || fieldNamed(t, observation.Fields, "EventData.Data").Semantic != "" {
		t.Errorf("observation = %+v, want an unrendered record with an unnamed value", observation)
	}
	message := fieldNamed(t, observation.Fields, "RenderingInfo.Message")
	if raw, _ := message.Text.RawTextValue(); raw != description {
		t.Errorf("message = %q, want the description as written", raw)
	}
	// 別のプロバイダの説明が見つからない旨の文は、このレコードの説明の欠落ではない。
	other := strings.Replace(description, "Example-Provider", "Other-Provider", 1)
	events, _ = readAllCSV(t, levelHeader+viewerRecord("情報", "2001/02/03 04:05:06",
		"Example-Provider", "1", "", other))
	if events[0].MessageUnrendered {
		t.Error("a description naming another provider is marked unrendered")
	}
}

// 論理レコードの先頭は、見出しの 1 欄目の形ごとの値と日付の文字列で始まる行である。先頭に
// ならない行は、前のレコードの説明の中に残る。
func TestCSVReaderStartsRecordsAtTheFirstColumnAndTheDate(t *testing.T) {
	for _, item := range []struct {
		header    string
		first     string
		candidate string
		starts    bool
	}{
		{levelHeader, "情報", "警告,2001/02/03 04:05:07,", true},
		{levelHeader, "情報", "詳細,2001/2/3 4:05:07,", true},
		{levelHeader, "情報", "成功の監査,2001/02/03 04:05:07,", false},
		{levelHeader, "情報", "情報,2001-02-03 04:05:07,", false},
		{levelHeader, "情報", "情報 2001/02/03 04:05:07", false},
		{levelHeader, "情報", "\t情報,2001/02/03 04:05:07,", false},
		{keywordHeader, "成功の監査", "失敗の監査,2001/02/03 04:05:07,", true},
		{keywordHeader, "成功の監査", ",2001/02/03 04:05:07,", true},
		{keywordHeader, "成功の監査", "成功の監査,2001/02/03,", false},
		{keywordHeader, "成功の監査", `"成功の監査",2001/02/03 04:05:07,`, false},
	} {
		description := "one\n" + item.candidate + "\ntwo"
		events, failures := readAllCSV(t, item.header+viewerRecord(item.first, "2001/02/03 04:05:06",
			"Example-Provider", "1", "", description))
		split := len(events) != 1 || len(failures) != 0
		if split != item.starts {
			t.Errorf("%q after %q: events = %d, failures = %d; want a record start = %v",
				item.candidate, item.header, len(events), len(failures), item.starts)
		}
		if !item.starts && len(events) == 1 && valueNamed(t, events[0].Sections, "RenderingInfo.Message") != description {
			t.Errorf("%q: the description lost the line", item.candidate)
		}
	}
}

// 壊れたレコードは行番号と理由を持つ失敗になり、後ろのレコードを読み続ける。
func TestCSVReaderContinuesAfterBrokenRecords(t *testing.T) {
	good := viewerRecord("情報", "2001/02/03 04:05:06", "Example-Provider", "1", "", "ok")
	unclosed := "情報,2001/02/03 04:05:07,Example-Provider,2,,\"never closed\nsecond line\n"
	bareQuote := "情報,2001/02/03 04:05:08,Example\"Provider,3,,\"x\"\n"
	fiveFields := "情報,2001/02/03 04:05:09,Example-Provider,4,\n"
	trailing := "情報,2001/02/03 04:05:10,Example-Provider,5,,\"closed\" and more\n"
	lineAfter := "情報,2001/02/03 04:05:11,Example-Provider,6,,\"closed\"\nline after the record\n"
	stray := "stray line before any record\n"
	document := keywordHeader + stray + good + unclosed + bareQuote + fiveFields + trailing + lineAfter + good
	events, failures := readAllCSV(t, strings.Replace(document, keywordHeader, levelHeader, 1))
	if len(events) != 2 {
		t.Fatalf("events = %d, want the two good records", len(events))
	}
	lineOf := func(part string) int64 {
		return int64(strings.Count(document[:strings.Index(document, part)], "\n")) + 1
	}
	want := []struct {
		part     string
		observed string
	}{
		{stray, "lines that follow no record start"},
		{unclosed, "CSV syntax error on line"},
		{bareQuote, "CSV syntax error on line"},
		{fiveFields, "has 5 fields, not 6"},
		{trailing, "CSV syntax error on line"},
		{lineAfter, "bytes on line 10 follow the record"},
	}
	if len(failures) != len(want) {
		t.Fatalf("failures = %+v, want %d", failures, len(want))
	}
	for at, item := range want {
		failure := failures[at]
		if failure.Stage != core.FailureStageTokenize || failure.LineNumber == nil ||
			*failure.LineNumber != lineOf(item.part) || !strings.Contains(failure.ObservedResult, item.observed) {
			t.Errorf("failure %d = %+v (line %v), want %q on line %d", at, failure, failure.LineNumber,
				item.observed, lineOf(item.part))
		}
	}
	if !strings.Contains(failures[1].ObservedResult, "line 5:") {
		t.Errorf("the unclosed quote is reported as %q, want the line where the record ends", failures[1].ObservedResult)
	}
}

// scriptBlockDescription は 4104 の説明を組む。見出しと ScriptBlock ID とパスの文字列は
// Windows が 4104 の説明に書く文字列である。
func scriptBlockDescription(number, total, text, path string) string {
	return "Scriptblock テキストを作成しています (" + total + " 個中 " + number + " 個目):\n" + text +
		"\n\nScriptBlock ID: 00000000-0000-4000-8000-000000000001\nパス: " + path
}

// 4104 の説明はスクリプトの本文を、XML の 4104 と同じ `<Data>` の名前の値にする。本文の中の
// `名前: 値` の形の行と空行と、本文が含む ScriptBlock ID の行は本文に残る。
func TestCSVReaderMapsTheScriptBlockDescription(t *testing.T) {
	text := "$name = 'example'\nlabel: not an item\n\n\tWrite-Output $name\n\nScriptBlock ID: inside the script"
	for _, lineBreak := range []string{"\n", "\r\n"} {
		record := viewerRecord("詳細", "2001/02/03 04:05:06", "Microsoft-Windows-PowerShell", "4104",
			"リモート コマンドを実行します", scriptBlockDescription("2", "3", text, `C:\Example\script.ps1`))
		events, failures := readAllCSV(t, strings.ReplaceAll(levelHeader+record, "\n", lineBreak))
		if len(failures) != 0 || len(events) != 1 {
			t.Fatalf("events = %d, failures = %+v; want 1 event", len(events), failures)
		}
		want := []winevent.Value{
			{Name: "MessageNumber", Text: "2"}, {Name: "MessageTotal", Text: "3"},
			{Name: "ScriptBlockText", Text: text},
			{Name: "ScriptBlockId", Text: "00000000-0000-4000-8000-000000000001"},
			{Name: "Path", Text: `C:\Example\script.ps1`},
		}
		if !slices.Equal(events[0].EventData, want) {
			t.Errorf("%q: EventData = %+v, want %+v", lineBreak, events[0].EventData, want)
		}
		observation, _ := winevent.Observe(events[0])
		if raw, _ := fieldNamed(t, observation.Fields, "EventData.ScriptBlockText").Text.RawTextValue(); raw != text {
			t.Errorf("%q: EventData.ScriptBlockText = %q, want %q", lineBreak, raw, text)
		}
	}
}

// 4104 の形を持たない 4104 の説明は、欄名を推測しない。説明の全文は RenderingInfo.Message に残る。
func TestCSVReaderLeavesAnUnexpectedScriptBlockDescriptionUnnamed(t *testing.T) {
	for _, description := range []string{
		"合成した説明の文です。\nlabel: value",
		strings.TrimSuffix(scriptBlockDescription("1", "1", "Get-Item", ""), "\nパス: "),
	} {
		events, _ := readAllCSV(t, levelHeader+viewerRecord("詳細", "2001/02/03 04:05:06",
			"Microsoft-Windows-PowerShell", "4104", "", description))
		if len(events[0].EventData) != 0 || valueNamed(t, events[0].Sections, "RenderingInfo.Message") != description {
			t.Errorf("%q: event = %+v, want no EventData and the description kept", description, events[0])
		}
	}
}

// 最後のレコードの後ろの空行は、そのレコードの範囲に入れない。
func TestCSVReaderEndsTheLastRecordBeforeTrailingBlankLines(t *testing.T) {
	record := viewerRecord("情報", "2001/02/03 04:05:06", "Example-Provider", "1", "", "説明の文\n2 行目")
	for _, trailer := range []string{"\n", "\n\n", " \n\t\n"} {
		for _, ending := range []string{"\n", "\r\n"} {
			document := strings.ReplaceAll(levelHeader+record+trailer, "\n", ending)
			want := strings.ReplaceAll(record, "\n", ending)
			events, failures := readAllCSV(t, document)
			if len(failures) != 0 || len(events) != 1 {
				t.Fatalf("trailer %q: events = %d, failures = %+v; want 1 event", trailer, len(events), failures)
			}
			source := events[0].Source
			if source.RawText != want || source.ByteLength != int64(len(want)) ||
				source.LineCount != int64(strings.Count(record, "\n")) {
				t.Errorf("trailer %q ending %q: source = %+v, want the record without the trailing lines",
					trailer, ending, source)
			}
		}
	}
}

func TestCSVReaderAcceptsTheKeywordHeaderAndCRLF(t *testing.T) {
	record := viewerRecord("成功の監査", "2001/02/03 04:05:06", "Example-Provider", "1", "タスク",
		"見出し:\n\t項目:\t値")
	document := strings.ReplaceAll(keywordHeader+record, "\n", "\r\n")
	events, failures := readAllCSV(t, document)
	if len(failures) != 0 || len(events) != 1 {
		t.Fatalf("events = %d, failures = %+v; want 1 event", len(events), failures)
	}
	if valueNamed(t, events[0].Sections, "RenderingInfo.Keywords.Keyword") != "成功の監査" ||
		valueNamed(t, events[0].EventData, "見出し.項目") != "値" {
		t.Errorf("event = %+v", events[0])
	}
	if events[0].Source.RawText != strings.ReplaceAll(record, "\n", "\r\n") {
		t.Errorf("RawText = %q, want the CRLF bytes as written", events[0].Source.RawText)
	}
}

func TestCSVReaderReportsAReadFailureAtTheBytesRead(t *testing.T) {
	cause := errors.New("device detached")
	read := levelHeader + "情報,2001/02/03"
	var reader winevent.CSVReader
	reader.Reset(io.MultiReader(strings.NewReader(read), iotest.ErrReader(cause)))
	_, failure, err := reader.Next()
	if !errors.Is(err, cause) {
		t.Fatalf("Next() error = %v, want the read error", err)
	}
	if failure == nil || failure.Stage != core.FailureStageRead || failure.ByteOffset == nil ||
		*failure.ByteOffset != int64(len(read)) || failure.LineNumber == nil || *failure.LineNumber != 2 {
		t.Errorf("failure = %+v, want a read failure at byte %d on line 2", failure, len(read))
	}
	if _, _, err := reader.Next(); !errors.Is(err, io.EOF) {
		t.Errorf("Next() after the read failure = %v, want io.EOF", err)
	}
}

func TestCSVReaderReportsAnUnknownHeader(t *testing.T) {
	events, failures := readAllCSV(t, "Level,Date and Time,Source,Event ID,Task Category\n"+
		viewerRecord("情報", "2001/02/03 04:05:06", "Example-Provider", "1", "", "x"))
	if len(events) != 0 || len(failures) != 1 || failures[0].LineNumber == nil || *failures[0].LineNumber != 1 {
		t.Fatalf("events = %d, failures = %+v; want one header failure on line 1", len(events), failures)
	}
	for _, document := range []string{"", "\ufeff", levelHeader} {
		if events, failures := readAllCSV(t, document); len(events) != 0 || len(failures) != 0 {
			t.Errorf("%q: events = %d, failures = %+v; want none", document, len(events), failures)
		}
	}
}
