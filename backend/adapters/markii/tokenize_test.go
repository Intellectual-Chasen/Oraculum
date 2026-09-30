package markii_test

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/markii"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// readOne は 1 行を Reader へ流し、レコードと失敗を返す。
func readOne(t *testing.T, line string) (markii.Record, *core.ImportFailure) {
	t.Helper()
	var reader markii.Reader
	reader.Reset(strings.NewReader(line))
	record, failure, err := reader.Next()
	if err != nil {
		t.Fatalf("reading one record: %v", err)
	}
	return record, failure
}

// readOneOK は 1 行を Reader へ流し、文字列の分割が成功したレコードを返す。
//
// 失敗を期待する test は readOne を使う。
func readOneOK(t *testing.T, line string) markii.Record {
	t.Helper()
	record, failure := readOne(t, line)
	if failure != nil {
		t.Fatalf("tokenizing the record: %+v", *failure)
	}
	return record
}

// 解析した field を連結すると原文に戻る。
//
// 漏れと拾いすぎのどちらも無いことを、この 1 つの検査が示す。
// **保存した原文を返すだけの比較にしない。** 解析した field 列から組み直して比べる。
func TestTokenizeRestoresTheOriginalText(t *testing.T) {
	lines := map[string]string{
		"通信のレコードの全体":      fullShapeLine,
		"引用符 2 個のエスケープ":   escapedQuoteLine,
		"素朴な grep が誤る文字列": naiveGrepLine,
		"欠測の 3 通り":        absenceLine,
		"値が 0":            zeroValueLine,
		"同じ key が 2 回":    duplicateKeyLine,
		"引用符付きの srcIP":    quotedSrcIPLine,
		"引用符無しの srcIP":    unquotedSrcIPLine,
	}
	for name, line := range lines {
		t.Run(name, func(t *testing.T) {
			record := readOneOK(t, line)
			restored, hasHeader := restoreOf(record)
			if !hasHeader {
				t.Fatal("the record must carry the date and time text and the zone text of its header")
			}
			if restored != line {
				t.Errorf("restored = %q, want %q", restored, line)
			}
		})
	}
}

// 引用符 2 個は引用符 1 個へ復号する。原資料の文字列は引用符 2 個のまま残す。
//
// 引用符 1 個を value の終端として読む処理は、この形の value を途中で切る。
func TestTokenizeDecodesDoubledQuotes(t *testing.T) {
	record := readOneOK(t, escapedQuoteLine)

	cmd, ok := record.Field("cmd")
	if !ok {
		t.Fatal("the record must carry the key cmd")
	}
	if want := `"/c ""echo hello"" done"`; cmd.RawValue() != want {
		t.Errorf("cmd RawValue = %q, want %q", cmd.RawValue(), want)
	}
	if want := `/c "echo hello" done`; cmd.Value() != want {
		t.Errorf("cmd Value = %q, want %q", cmd.Value(), want)
	}
	if !cmd.Quoted() {
		t.Error("cmd must be reported as quoted")
	}

	// 引用符 2 個で終わる value。閉じ引用符の 1 個と escape の 2 個が並ぶ。
	winTitle, ok := record.Field("winTitle")
	if !ok {
		t.Fatal("the record must carry the key winTitle")
	}
	if want := `"say ""hi"""`; winTitle.RawValue() != want {
		t.Errorf("winTitle RawValue = %q, want %q", winTitle.RawValue(), want)
	}
	if want := `say "hi"`; winTitle.Value() != want {
		t.Errorf("winTitle Value = %q, want %q", winTitle.Value(), want)
	}
}

// 引用符付き value の内側の文字列を key として拾わない。
//
// 素朴な正規表現は、引用符付き value の内側にある key=value と同じ字面まで key として
// 拾う。本 package はその字面を key にしない。
func TestTokenizeIgnoresKeysInsideAQuotedValue(t *testing.T) {
	record := readOneOK(t, naiveGrepLine)

	wantKeys := []string{"evt", "subEvt", "sn", "cmd", "lv"}
	gotKeys := make([]string, 0, len(wantKeys))
	for _, field := range record.Fields() {
		gotKeys = append(gotKeys, field.Key())
	}
	if len(gotKeys) != len(wantKeys) {
		t.Fatalf("keys = %v, want %v", gotKeys, wantKeys)
	}
	for index := range wantKeys {
		if gotKeys[index] != wantKeys[index] {
			t.Errorf("key at %d = %q, want %q", index, gotKeys[index], wantKeys[index])
		}
	}

	// value の内側にある 3 個の字面が key として出ない。
	for _, absent := range []string{"group", "enable", "SubSystemType"} {
		if count := record.FieldCount(absent); count != 0 {
			t.Errorf("FieldCount(%q) = %d, want 0", absent, count)
		}
		if _, ok := record.Field(absent); ok {
			t.Errorf("Field(%q) must report the key as absent", absent)
		}
	}

	// value は内側の文字列をそのまま持つ。
	cmd, ok := record.Field("cmd")
	if !ok {
		t.Fatal("the record must carry the key cmd")
	}
	if want := `netsh advfirewall set group=all enable=yes SubSystemType=Windows`; cmd.Value() != want {
		t.Errorf("cmd Value = %q, want %q", cmd.Value(), want)
	}
}

// 欠測の 3 通りを別の形で返す。
//
// key が無い / key="" / key="-" の 3 つである。
// 原資料に実在する空文字列と "-" を欄の不在の代用にしない。
func TestTokenizeSeparatesTheThreeFormsOfAbsence(t *testing.T) {
	record := readOneOK(t, absenceLine)

	// 1. key が無い。
	if count := record.FieldCount("usrDomain"); count != 0 {
		t.Errorf("FieldCount(\"usrDomain\") = %d, want 0", count)
	}
	if _, ok := record.Field("usrDomain"); ok {
		t.Error("usrDomain must be reported as absent")
	}

	// 2. key="" である。値が空文字列で、key は出ている。
	evtDomain, ok := record.Field("evtDomain")
	if !ok {
		t.Fatal("evtDomain must be reported as present")
	}
	if evtDomain.Value() != "" {
		t.Errorf("evtDomain Value = %q, want an empty string", evtDomain.Value())
	}
	if evtDomain.RawValue() != `""` {
		t.Errorf("evtDomain RawValue = %q, want %q", evtDomain.RawValue(), `""`)
	}
	if !evtDomain.Quoted() {
		t.Error("evtDomain must be reported as quoted")
	}

	// 3. key="-" である。値は "-" の 1 文字であり、空文字列と別の形である。
	for _, key := range []string{"srcCom", "srcIP"} {
		field, ok := record.Field(key)
		if !ok {
			t.Fatalf("%s must be reported as present", key)
		}
		if field.Value() != "-" {
			t.Errorf("%s Value = %q, want %q", key, field.Value(), "-")
		}
		if field.RawValue() != `"-"` {
			t.Errorf("%s RawValue = %q, want %q", key, field.RawValue(), `"-"`)
		}
	}
}

// 値が 0 の欄は、欄が無い状態と別の形である。
func TestTokenizeKeepsAZeroValue(t *testing.T) {
	record := readOneOK(t, zeroValueLine)

	recv, ok := record.Field("recv")
	if !ok {
		t.Fatal("recv must be reported as present")
	}
	if recv.Value() != "0" {
		t.Errorf("recv Value = %q, want %q", recv.Value(), "0")
	}
	if recv.Quoted() {
		t.Error("recv must be reported as not quoted")
	}
	if count := record.FieldCount("recv"); count != 1 {
		t.Errorf("FieldCount(\"recv\") = %d, want 1", count)
	}
}

// key の並びが違う 2 行で同じ key が同じ値を返す。
//
// key の並びは固定でない。evt が file で subEvt が close のレコードだけでも並びが複数ある。
func TestTokenizeDoesNotDependOnTheOrderOfKeys(t *testing.T) {
	recordA := readOneOK(t, orderLineA)
	recordB := readOneOK(t, orderLineB)

	evtA, okA := recordA.Field("evt")
	evtB, okB := recordB.Field("evt")
	if !okA || !okB {
		t.Fatal("both records must carry the key evt")
	}
	if evtA.Value() != "file" || evtB.Value() != "file" {
		t.Errorf("evt = %q and %q, want %q for both", evtA.Value(), evtB.Value(), "file")
	}

	// 並びは原文のままである。2 行の 1 つ目の key が違う。
	fieldsA := recordA.Fields()
	fieldsB := recordB.Fields()
	if fieldsA[0].Key() != "evt" {
		t.Errorf("the first key of the first record = %q, want %q", fieldsA[0].Key(), "evt")
	}
	if fieldsB[0].Key() != "sn" {
		t.Errorf("the first key of the second record = %q, want %q", fieldsB[0].Key(), "sn")
	}
}

// 同じ key が 2 回出たレコードで、個数を数えられる。
//
// map の上書きで重複を消さない。
func TestTokenizeKeepsARepeatedKey(t *testing.T) {
	record := readOneOK(t, duplicateKeyLine)

	if count := record.FieldCount("lv"); count != 2 {
		t.Errorf("FieldCount(\"lv\") = %d, want 2", count)
	}
	// Field は原文の並びで最初のものを返す。
	lv, ok := record.Field("lv")
	if !ok {
		t.Fatal("lv must be reported as present")
	}
	if lv.Value() != "5" {
		t.Errorf("lv Value = %q, want %q (the first one in the record)", lv.Value(), "5")
	}
}

// 同じ key が引用符付きの形と引用符無しの形の両方で現れる。2 つが別の出所の値を持つ
// ため、復号後の値が同じでも引用符の有無を残す。
func TestTokenizeReportsWhetherAValueWasQuoted(t *testing.T) {
	quotedRecord := readOneOK(t, quotedSrcIPLine)
	unquotedRecord := readOneOK(t, unquotedSrcIPLine)

	quotedField, ok := quotedRecord.Field("srcIP")
	if !ok {
		t.Fatal("the quoted record must carry the key srcIP")
	}
	unquotedField, ok := unquotedRecord.Field("srcIP")
	if !ok {
		t.Fatal("the unquoted record must carry the key srcIP")
	}

	if !quotedField.Quoted() {
		t.Error("the srcIP of the first record must be reported as quoted")
	}
	if unquotedField.Quoted() {
		t.Error("the srcIP of the second record must be reported as not quoted")
	}
	// 復号後の値は 2 行で同じである。引用符の有無だけが違う。
	if quotedField.Value() != unquotedField.Value() {
		t.Errorf("values = %q and %q, want them equal", quotedField.Value(), unquotedField.Value())
	}
	if quotedField.Value() != "192.0.2.10" {
		t.Errorf("srcIP Value = %q, want %q", quotedField.Value(), "192.0.2.10")
	}
}

// 通信のレコードの文字列構造を持つ 1 行の field の個数と、key ごとの値を固定する。
func TestTokenizeReadsARecordOfTheFullShape(t *testing.T) {
	record := readOneOK(t, fullShapeLine)

	if got := len(record.Fields()); got != fullShapeLineFieldCount {
		t.Errorf("field count = %d, want %d", got, fullShapeLineFieldCount)
	}
	if dateTime, _ := record.DateTimeText(); dateTime != "02/01/2000 10:11:12.181" {
		t.Errorf("date and time = %q, want %q", dateTime, "02/01/2000 10:11:12.181")
	}
	if zone, _ := record.ZoneText(); zone != "+0900" {
		t.Errorf("zone = %q, want %q", zone, "+0900")
	}

	wanted := map[string]struct {
		value  string
		quoted bool
	}{
		"sn":      {"900100", false},
		"evt":     {"net", false},
		"subEvt":  {"est", false},
		"com":     {"HOST01", true},
		"psPath":  {`C:\Tools\agent.exe`, true},
		"srcIP":   {"192.0.2.10", false},
		"dstPort": {"443", false},
	}
	for key, want := range wanted {
		field, ok := record.Field(key)
		if !ok {
			t.Errorf("the record must carry the key %q", key)
			continue
		}
		if field.Value() != want.value {
			t.Errorf("%s Value = %q, want %q", key, field.Value(), want.value)
		}
		if field.Quoted() != want.quoted {
			t.Errorf("%s Quoted = %t, want %t", key, field.Quoted(), want.quoted)
		}
	}
}

// value の byte offset は原文の位置を指す。
//
// 単位は byte である。引用符で囲む形では開く引用符の位置を指す。
func TestFieldByteOffsetsPointAtTheOriginalText(t *testing.T) {
	record := readOneOK(t, zeroValueLine)

	for _, field := range record.Fields() {
		keyStart := field.KeyByteOffset()
		if got := record.RawText()[keyStart : keyStart+int64(len(field.Key()))]; got != field.Key() {
			t.Errorf("the text at the key offset of %s = %q, want %q", field.Key(), got, field.Key())
		}
		valueStart := field.ValueByteOffset()
		if got := record.RawText()[valueStart : valueStart+field.ValueByteLength()]; got != field.RawValue() {
			t.Errorf("the text at the value offset of %s = %q, want %q",
				field.Key(), got, field.RawValue())
		}
		// key の直後に = が 1 個入る。
		if sign := record.RawText()[keyStart+int64(len(field.Key()))]; sign != '=' {
			t.Errorf("the byte after the key %s = %q, want %q", field.Key(), sign, "=")
		}
	}

	// 引用符で囲む形では開く引用符の位置を指す。
	quotedRecord, _ := readOne(t, quotedSrcIPLine)
	srcIP, ok := quotedRecord.Field("srcIP")
	if !ok {
		t.Fatal("the record must carry the key srcIP")
	}
	if got := quotedRecord.RawText()[srcIP.ValueByteOffset()]; got != '"' {
		t.Errorf("the byte at the value offset = %q, want the opening quote", got)
	}
}

// 文字列の分割が続けられない 7 つの形を、失敗として返す。
func TestTokenizeReportsTheFormsItCannotRead(t *testing.T) {
	cases := map[string]string{
		"ヘッダーが短い":        `02/01/2000 10:11:12`,
		"ヘッダーの後が空白でない":   `02/01/2000 10:11:12.181 +0900Xevt=net`,
		"key で始まらない":     `02/01/2000 10:11:12.181 +0900 =net`,
		"= が無い":          `02/01/2000 10:11:12.181 +0900 evt`,
		"閉じ引用符が無い":       `02/01/2000 10:11:12.181 +0900 cmd="unterminated`,
		"value の後が空白でない": `02/01/2000 10:11:12.181 +0900 cmd="closed"X`,
		// fuzz が見つけた形。ヘッダーと半角空白だけで field が 0 個である。
		// corpus は testdata/fuzz/FuzzTokenizeRecord/ が持つ。
		"field が 0 個": `02/01/2000 10:11:12.181 +0900 `,
		// key が数字で始まる形。key の文法は [A-Za-z_][A-Za-z0-9_]* である。
		"key が数字で始まる": `02/01/2000 10:11:12.181 +0900 1evt=net`,
	}
	for name, line := range cases {
		t.Run(name, func(t *testing.T) {
			record, failure := readOne(t, line)
			if failure == nil {
				t.Fatalf("the record %q must be reported as a failure", line)
			}
			if failure.Stage != core.FailureStageTokenize {
				t.Errorf("stage = %q, want %q", failure.Stage, core.FailureStageTokenize)
			}
			if failure.DiagnosisClass != core.DiagnosisClassUndetermined {
				t.Errorf("diagnosisClass = %q, want %q",
					failure.DiagnosisClass, core.DiagnosisClassUndetermined)
			}
			// 原文と位置は失敗したレコードでも返る。
			if record.RawText() != line {
				t.Errorf("RawText = %q, want %q", record.RawText(), line)
			}
			if record.LineNumber() != 1 {
				t.Errorf("lineNumber = %d, want 1", record.LineNumber())
			}
		})
	}
}

// ヘッダーの 29 byte に多 byte 文字が入る形を、ASCII の検査が捕まえる。
//
// **byte 長と区切りの位置を保ったまま非 ASCII の byte を入れる。** 長さや区切りを崩すと、
// ASCII の検査より前の検査で失敗し、ASCII の検査を外しても通ってしまう。
func TestTokenizeRejectsANonASCIIHeader(t *testing.T) {
	// 先頭 23 byte の末尾 2 byte を 2 byte の文字 1 個に置き換える。
	// "02/01/2000 10:11:12.1" が 21 byte、"é" が 2 byte で、合計 23 byte である。
	const line = "02/01/2000 10:11:12.1é +0900 evt=net"
	if got := len(line[:zoneEndForTest]); got != zoneEndForTest {
		t.Fatalf("the header is %d bytes, want %d", got, zoneEndForTest)
	}
	if line[dateTimeEndForTest] != ' ' {
		t.Fatalf("the byte at offset %d = %q, want a space", dateTimeEndForTest, line[dateTimeEndForTest])
	}
	if line[zoneEndForTest] != ' ' {
		t.Fatalf("the byte at offset %d = %q, want a space", zoneEndForTest, line[zoneEndForTest])
	}

	_, failure := readOne(t, line)
	if failure == nil {
		t.Fatal("a header carrying a non ASCII byte must be reported as a failure")
	}
	if failure.Stage != core.FailureStageTokenize {
		t.Errorf("stage = %q, want %q", failure.Stage, core.FailureStageTokenize)
	}
	// 失敗の位置は多 byte 文字の先頭の byte である。
	if failure.ByteOffset == nil {
		t.Fatal("the diagnosis must carry the byte offset of the non ASCII byte")
	}
	if *failure.ByteOffset != 21 {
		t.Errorf("byteOffset = %d, want 21", *failure.ByteOffset)
	}
	if want := "an ASCII byte inside the 29 byte header"; failure.ExpectedMeaning != want {
		t.Errorf("expectedMeaning = %q, want %q", failure.ExpectedMeaning, want)
	}
	if want := "the byte 0xc3"; failure.ObservedResult != want {
		t.Errorf("observedResult = %q, want %q", failure.ObservedResult, want)
	}
}

// ヘッダーの位置。record.go の非公開の定数と同じ値を test 側で固定する。
const (
	dateTimeEndForTest = 23
	zoneEndForTest     = 29
)

// 末尾が区切りで終わるレコードでも、読めた field を返す。
//
// 読めた範囲を捨てない。
func TestTokenizeKeepsTheFieldsItReadBeforeATrailingSeparator(t *testing.T) {
	record, failure := readOne(t, `02/01/2000 10:11:12.181 +0900 evt=net sn=123 `)
	if failure == nil {
		t.Fatal("a record ending with a separator must be reported as a failure")
	}

	// 完成した 2 つの field は返る。
	if got := len(record.Fields()); got != 2 {
		t.Fatalf("field count = %d, want 2", got)
	}
	for key, want := range map[string]string{"evt": "net", "sn": "123"} {
		field, ok := record.Field(key)
		if !ok {
			t.Errorf("the record must carry the key %q even though it failed", key)
			continue
		}
		if field.Value() != want {
			t.Errorf("%s Value = %q, want %q", key, field.Value(), want)
		}
	}
	// 失敗の位置は末尾の区切りである。原文は 45 byte で、区切りは 44 byte 目にある。
	if failure.ByteOffset == nil {
		t.Fatal("the diagnosis must carry the byte offset of the trailing separator")
	}
	if *failure.ByteOffset != 44 {
		t.Errorf("byteOffset = %d, want 44", *failure.ByteOffset)
	}
}

// 上限を超えた行を打ち切らず、読み込みの失敗として返す。
//
// 上限を超えた行を「読めた」にしない。
func TestReaderRejectsARecordOverTheByteLimit(t *testing.T) {
	// 改行を 1 個も持たない 2 MiB の入力。
	const overLimit = 2 << 20
	var reader markii.Reader
	reader.Reset(strings.NewReader(strings.Repeat("0", overLimit)))

	_, failure, err := reader.Next()
	if err == nil {
		t.Fatal("a record over the byte limit must be reported through the error")
	}
	if failure == nil {
		t.Fatal("a record over the byte limit must carry a diagnosis")
	}
	if failure.Stage != core.FailureStageRead {
		t.Errorf("stage = %q, want %q", failure.Stage, core.FailureStageRead)
	}
	// 上限を超えた位置で止まる。入力の全体を読み切らない。
	if failure.ByteOffset == nil {
		t.Fatal("the diagnosis must carry the byte offset where reading stopped")
	}
	if *failure.ByteOffset >= overLimit {
		t.Errorf("byteOffset = %d, want it below the size of the input (%d)",
			*failure.ByteOffset, overLimit)
	}
}

// 上限ちょうどのレコードは、改行の文字列が 3 通りのどれであっても読める。
//
// 上限はレコードの原文に掛かる。改行の文字列を原文に数えると、同じ長さのレコードが
// LF では読めて CR LF では読めなくなる。
func TestReaderReadsARecordOfExactlyTheByteLimit(t *testing.T) {
	const limit = 1 << 20
	// ヘッダー 29 文字と半角空白 1 個の後に key= を置き、残りを value で埋める。
	const head = "02/01/2000 10:11:12.181 +0900 v="
	body := head + strings.Repeat("0", limit-len(head))
	if len(body) != limit {
		t.Fatalf("the synthesized record is %d bytes, want %d", len(body), limit)
	}

	for _, lineEnding := range []struct {
		name string
		text string
	}{
		{"改行無し", ""},
		{"LF", "\n"},
		{"CR LF", "\r\n"},
	} {
		t.Run(lineEnding.name, func(t *testing.T) {
			var reader markii.Reader
			reader.Reset(strings.NewReader(body + lineEnding.text))

			record, failure, err := reader.Next()
			if err != nil {
				t.Fatalf("reading a record of exactly the byte limit: %v", err)
			}
			if failure != nil {
				t.Fatalf("a record of exactly the byte limit must not fail: %+v", *failure)
			}
			if record.RawText() != body {
				t.Errorf("the record is %d bytes, want %d", len(record.RawText()), len(body))
			}
			if record.LineEnding() != lineEnding.text {
				t.Errorf("line ending = %q, want %q", record.LineEnding(), lineEnding.text)
			}
		})
	}
}

// 行途中の読み込みの失敗で、診断の位置が止まった位置になる。
//
// 読めた断片の byte 数を足した位置である。
func TestReadFailureCarriesTheOffsetWhereItStopped(t *testing.T) {
	const fragment = "abc"
	var reader markii.Reader
	reader.Reset(io.MultiReader(strings.NewReader(fragment), errorReader{}))

	record, failure, err := reader.Next()
	if err == nil {
		t.Fatal("a read failure must be reported through the error")
	}
	if failure == nil {
		t.Fatal("a read failure must carry a diagnosis")
	}
	if failure.ByteOffset == nil {
		t.Fatal("the diagnosis must carry the byte offset where reading stopped")
	}
	if *failure.ByteOffset != int64(len(fragment)) {
		t.Errorf("byteOffset = %d, want %d", *failure.ByteOffset, len(fragment))
	}
	// レコードの位置は行頭のままである。
	if record.ByteOffset() != 0 {
		t.Errorf("the byteOffset of the record = %d, want 0", record.ByteOffset())
	}
	if record.RawText() != fragment {
		t.Errorf("RawText = %q, want %q", record.RawText(), fragment)
	}
}

// 失敗の値は、文字列の分割が知り得ない 4 項目を埋めない。
//
// 埋めるのはプロセス開始記録と通信記録のパーサーと取り込みの実行である。
// 未知の必須情報を zero value で補わない。
func TestFailureLeavesTheItemsTheTokenizerCannotKnow(t *testing.T) {
	_, failure := readOne(t, `02/01/2000 10:11:12.181 +0900 cmd="unterminated`)
	if failure == nil {
		t.Fatal("the record must be reported as a failure")
	}

	if failure.SourceId != "" {
		t.Errorf("sourceId = %q, want it left empty", failure.SourceId)
	}
	if failure.SourceContentSha256 != "" {
		t.Errorf("sourceContentSha256 = %q, want it left empty", failure.SourceContentSha256)
	}
	if failure.ParserVersion != "" {
		t.Errorf("parserVersion = %q, want it left empty", failure.ParserVersion)
	}
	if failure.SanitizedMessage != "" {
		t.Errorf("sanitizedMessage = %q, want it left empty", failure.SanitizedMessage)
	}
	if failure.RecordRef != nil {
		t.Error("recordRef must be left absent")
	}
	if failure.RawTextRef != "" {
		t.Errorf("rawTextRef = %q, want it left empty", failure.RawTextRef)
	}

	// 文字列の分割が埋める 8 項目は入っている。
	if failure.LineNumber == nil {
		t.Error("lineNumber must be filled by the tokenizer")
	}
	if failure.ByteOffset == nil {
		t.Error("byteOffset must be filled by the tokenizer")
	}
	for name, value := range map[string]string{
		"interpretation":   failure.Interpretation,
		"expectedMeaning":  failure.ExpectedMeaning,
		"observedResult":   failure.ObservedResult,
		"unresolvedReason": failure.UnresolvedReason,
	} {
		if value == "" {
			t.Errorf("%s must be filled by the tokenizer", name)
		}
	}

	// **そのままでは Validate を通らない。** 通らないのは sourceId が空であるためである。
	if err := failure.Validate(); !errors.Is(err, core.ErrMissingRequiredItem) {
		t.Errorf("error = %v, want one wrapping %v", err, core.ErrMissingRequiredItem)
	}
}

// 診断に原資料の byte 列を差し込まない。
//
// 無害化は出力境界の責務であり、adapters は backend/output を import できない。
// 原資料側の証拠は observedResult が持つ。
func TestFailureCarriesNoControlCharacter(t *testing.T) {
	// value の後に制御文字がある行。
	_, failure := readOne(t, "02/01/2000 10:11:12.181 +0900 cmd=\"closed\"\x01")
	if failure == nil {
		t.Fatal("the record must be reported as a failure")
	}
	for name, value := range map[string]string{
		"expectedMeaning":  failure.ExpectedMeaning,
		"observedResult":   failure.ObservedResult,
		"unresolvedReason": failure.UnresolvedReason,
		"interpretation":   failure.Interpretation,
	} {
		if strings.ContainsAny(value, "\x00\x01\x02\x03\x04\x05\x06\x07\x08\n\r\v\f") {
			t.Errorf("%s carries a control character: %q", name, value)
		}
	}
}

// 1 レコードの失敗で走査を止めない。
func TestReaderKeepsScanningAfterAFailedRecord(t *testing.T) {
	source := strings.Join([]string{orderLineA, `02/01/2000 10:11:12.181 +0900 cmd="unterminated`, orderLineB}, "\r\n")
	var reader markii.Reader
	reader.Reset(strings.NewReader(source))

	type observed struct {
		lineNumber int64
		byteOffset int64
		failed     bool
	}
	var got []observed
	for {
		record, failure, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("reading the source: %v", err)
		}
		got = append(got, observed{record.LineNumber(), record.ByteOffset(), failure != nil})
	}

	want := []observed{
		{1, 0, false},
		{2, int64(len(orderLineA)) + 2, true},
		{3, int64(len(orderLineA)) + 2 + int64(len(`02/01/2000 10:11:12.181 +0900 cmd="unterminated`)) + 2, false},
	}
	if len(got) != len(want) {
		t.Fatalf("read %d records, want %d", len(got), len(want))
	}
	for index := range want {
		if got[index] != want[index] {
			t.Errorf("record at %d = %+v, want %+v", index, got[index], want[index])
		}
	}
}

// CR LF と LF の両方を読む。CR を原文に入れず、byte 数には算入する。
func TestReaderHandlesBothLineEndings(t *testing.T) {
	cases := map[string]struct {
		source     string
		lineEnding string
		nextOffset int64
	}{
		"CR LF": {orderLineA + "\r\n" + orderLineB, "\r\n", int64(len(orderLineA)) + 2},
		"LF":    {orderLineA + "\n" + orderLineB, "\n", int64(len(orderLineA)) + 1},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			var reader markii.Reader
			reader.Reset(strings.NewReader(testCase.source))

			first, failure, err := reader.Next()
			if err != nil || failure != nil {
				t.Fatalf("reading the first record: err = %v, failure = %+v", err, failure)
			}
			if first.RawText() != orderLineA {
				t.Errorf("RawText = %q, want %q", first.RawText(), orderLineA)
			}
			if first.LineEnding() != testCase.lineEnding {
				t.Errorf("LineEnding = %q, want %q", first.LineEnding(), testCase.lineEnding)
			}
			// 最後の field の value に CR が入らない。
			fields := first.Fields()
			last := fields[len(fields)-1]
			if strings.ContainsRune(last.Value(), '\r') {
				t.Errorf("the value of the last field carries a CR: %q", last.Value())
			}

			second, failure, err := reader.Next()
			if err != nil || failure != nil {
				t.Fatalf("reading the second record: err = %v, failure = %+v", err, failure)
			}
			if second.ByteOffset() != testCase.nextOffset {
				t.Errorf("the byteOffset of the second record = %d, want %d",
					second.ByteOffset(), testCase.nextOffset)
			}
		})
	}
}

// 末尾に改行が無い最後のレコードを返す。
func TestReaderReadsTheLastRecordWithoutALineEnding(t *testing.T) {
	var reader markii.Reader
	reader.Reset(strings.NewReader(orderLineA))

	record, failure, err := reader.Next()
	if err != nil || failure != nil {
		t.Fatalf("reading the record: err = %v, failure = %+v", err, failure)
	}
	if record.RawText() != orderLineA {
		t.Errorf("RawText = %q, want %q", record.RawText(), orderLineA)
	}
	if record.LineEnding() != "" {
		t.Errorf("LineEnding = %q, want it empty", record.LineEnding())
	}

	if _, _, err := reader.Next(); !errors.Is(err, io.EOF) {
		t.Errorf("error = %v, want %v", err, io.EOF)
	}
}

// 末尾の DOS EOF marker はレコードではない。原 byte と位置を失敗に残し、成功した
// レコードを捨てずに部分公開できるよう、入力不整合として分類する。
func TestReaderDiagnosesATrailingDOSEOFMarker(t *testing.T) {
	var reader markii.Reader
	reader.Reset(strings.NewReader(orderLineA + "\r\n\x1a"))

	if _, failure, err := reader.Next(); err != nil || failure != nil {
		t.Fatalf("reading the valid record: err = %v, failure = %+v", err, failure)
	}

	record, failure, err := reader.Next()
	if err != nil {
		t.Fatalf("reading the trailing marker: %v", err)
	}
	if failure == nil {
		t.Fatal("the trailing DOS EOF marker must be diagnosed")
	}
	if failure.Stage != core.FailureStageTokenize {
		t.Errorf("stage = %q, want %q", failure.Stage, core.FailureStageTokenize)
	}
	if failure.DiagnosisClass != core.DiagnosisClassInconsistentInputConfirmed {
		t.Errorf("diagnosisClass = %q, want %q", failure.DiagnosisClass, core.DiagnosisClassInconsistentInputConfirmed)
	}
	if failure.UnresolvedReason != "" {
		t.Errorf("unresolvedReason = %q, want empty", failure.UnresolvedReason)
	}
	if record.RawText() != "\x1a" {
		t.Errorf("RawText = %q, want the marker byte", record.RawText())
	}
	if record.LineNumber() != 2 {
		t.Errorf("lineNumber = %d, want 2", record.LineNumber())
	}
	wantOffset := int64(len(orderLineA) + 2)
	if failure.ByteOffset == nil || *failure.ByteOffset != wantOffset {
		t.Errorf("byteOffset = %v, want %d", failure.ByteOffset, wantOffset)
	}
	if want := "a single DOS EOF marker byte 0x1a, not a markii record"; failure.ObservedResult != want {
		t.Errorf("observedResult = %q, want %q", failure.ObservedResult, want)
	}
}

// 末尾 marker が CR LF で終端されていても、物理的な入力末尾なら入力不整合として分類する。
func TestReaderDiagnosesATrailingDOSEOFMarkerWithALineEnding(t *testing.T) {
	var reader markii.Reader
	reader.Reset(strings.NewReader(orderLineA + "\r\n\x1a\r\n"))

	if _, failure, err := reader.Next(); err != nil || failure != nil {
		t.Fatalf("reading the valid record: err = %v, failure = %+v", err, failure)
	}

	record, failure, err := reader.Next()
	if err != nil {
		t.Fatalf("reading the trailing marker: %v", err)
	}
	if failure == nil {
		t.Fatal("the trailing DOS EOF marker must be diagnosed")
	}
	if failure.DiagnosisClass != core.DiagnosisClassInconsistentInputConfirmed {
		t.Errorf("diagnosisClass = %q, want %q", failure.DiagnosisClass, core.DiagnosisClassInconsistentInputConfirmed)
	}
	if record.RawText() != "\x1a" {
		t.Errorf("RawText = %q, want the marker byte", record.RawText())
	}

	if _, _, err := reader.Next(); !errors.Is(err, io.EOF) {
		t.Errorf("error after the marker = %v, want %v", err, io.EOF)
	}
}

// レコード内部の DOS EOF byte は末尾 marker ではない。末尾 marker と同じ byte でも、
// 独立した根拠が無いので通常の未判定失敗に残す。
func TestReaderKeepsADOSEOFByteInsideARecordUndetermined(t *testing.T) {
	var reader markii.Reader
	reader.Reset(strings.NewReader(`02/01/2000 13:14:00.000 +0900 cmd="value` + "\x1a"))

	record, failure, err := reader.Next()
	if err != nil {
		t.Fatalf("reading the malformed record: %v", err)
	}
	if failure == nil {
		t.Fatal("the record containing a DOS EOF byte must be diagnosed")
	}
	if failure.DiagnosisClass != core.DiagnosisClassUndetermined {
		t.Errorf("diagnosisClass = %q, want %q", failure.DiagnosisClass, core.DiagnosisClassUndetermined)
	}
	if failure.UnresolvedReason == "" {
		t.Error("an undetermined tokenization failure must carry unresolvedReason")
	}
	if record.RawText() != `02/01/2000 13:14:00.000 +0900 cmd="value`+"\x1a" {
		t.Errorf("RawText = %q, want the original record", record.RawText())
	}
}

// 単独の DOS EOF byte が物理的な末尾以外にある場合も、末尾 marker ではない。通常の
// 未判定失敗として返し、後続のレコードを読み続ける。
func TestReaderKeepsANonTrailingDOSEOFMarkerUndetermined(t *testing.T) {
	var reader markii.Reader
	reader.Reset(strings.NewReader("\x1a\r\n" + orderLineA))

	record, failure, err := reader.Next()
	if err != nil {
		t.Fatalf("reading the non-trailing marker: %v", err)
	}
	if failure == nil {
		t.Fatal("the non-trailing DOS EOF marker must be diagnosed")
	}
	if failure.DiagnosisClass != core.DiagnosisClassUndetermined {
		t.Errorf("diagnosisClass = %q, want %q", failure.DiagnosisClass, core.DiagnosisClassUndetermined)
	}
	if failure.UnresolvedReason == "" {
		t.Error("an undetermined tokenization failure must carry unresolvedReason")
	}
	if record.RawText() != "\x1a" {
		t.Errorf("RawText = %q, want the marker byte", record.RawText())
	}

	if record, failure, err = reader.Next(); err != nil || failure != nil {
		t.Fatalf("reading the record after the marker: err = %v, failure = %+v", err, failure)
	}
	if record.RawText() != orderLineA {
		t.Errorf("following RawText = %q, want %q", record.RawText(), orderLineA)
	}
}

// Reset を呼ぶ前の Next は error を返す。
func TestReaderRequiresReset(t *testing.T) {
	var reader markii.Reader
	if _, _, err := reader.Next(); err == nil {
		t.Error("calling Next before Reset must return an error")
	}
}

// 読み込みが続けられない失敗は stage が read で、文字列の分割の失敗と別の経路に出る。
func TestReaderReportsAReadFailureThroughTheError(t *testing.T) {
	var reader markii.Reader
	reader.Reset(io.MultiReader(strings.NewReader(orderLineA+"\r\n"), errorReader{}))

	if _, failure, err := reader.Next(); err != nil || failure != nil {
		t.Fatalf("reading the first record: err = %v, failure = %+v", err, failure)
	}

	_, failure, err := reader.Next()
	if err == nil {
		t.Fatal("a read failure must be reported through the error")
	}
	if failure == nil {
		t.Fatal("a read failure must carry a diagnosis")
	}
	if failure.Stage != core.FailureStageRead {
		t.Errorf("stage = %q, want %q", failure.Stage, core.FailureStageRead)
	}
}

// errorReader は読み込みの失敗を作る。
type errorReader struct{}

// Read はつねに失敗を返す。
func (errorReader) Read([]byte) (int, error) {
	return 0, errBrokenSource
}

// errBrokenSource は test が作る読み込みの失敗である。
var errBrokenSource = errors.New("markii_test: the source cannot be read")
