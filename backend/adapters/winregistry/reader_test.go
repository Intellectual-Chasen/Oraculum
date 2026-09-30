package winregistry

import (
	"bytes"
	"encoding/binary"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const (
	synthName     = "synth"
	synthPathText = `\Alpha`
	oldText       = "old-text-1"
	midText       = "mid-text-2"
	newText       = "new-text-3"
)

// synthHive は root の下に Alpha (値と子の Leaf を持つ) と Beta.Gamma を持つ hive bins data を組む。
// Alpha の値 Text の文字列だけを text にし、ほかの配置は text の長さが同じ限り変わらない。
func synthHive(text string, extraKey bool) (bins []byte, root uint32) {
	b := newHiveBuilder()
	leaf := b.key("Leaf", synthLastWritten, nil, nil)
	alpha := b.key("Alpha", synthLastWritten, []uint32{leaf}, []uint32{
		b.value("", regSZ, utf16Bytes("default")),
		b.value("Text", regSZ, utf16Bytes(text)),
		b.value("Name.With.Dot", regDword, []byte{0x39, 0x30, 0xFF, 0xFF}),
		b.value("Q", regQword, []byte{1, 0, 0, 0, 0, 0, 0, 0}),
		b.value("Multi", regMultiSZ, append(utf16Bytes("a"), utf16Bytes("b")...)),
		b.value("Bin", regBinary, []byte{1, 2, 3, 4, 5, 6, 7, 8}),
		b.value("Big", regBinary, bytes.Repeat([]byte{0xAB}, bigDataSegmentSize+10)),
	})
	beta := b.key("Beta.Gamma", 0, nil, nil)
	root = b.key("ROOT", synthLastWritten, []uint32{alpha, beta}, nil)
	if !extraKey {
		return b.binsData(), root
	}
	// 最初の hbin の後ろに key を足し、root の subkey list をその key を含む list に差し替える。
	b.padTo(3 * hiveBinAlignment)
	extra := b.key("Extra", synthLastWritten, nil, nil)
	list := b.list("lh", []uint32{alpha, beta, extra})
	binary.LittleEndian.PutUint32(b.bins[root+4+20:], 3)
	binary.LittleEndian.PutUint32(b.bins[root+4+28:], list)
	return b.binsData(), root
}

func primaryName() string { return synthName }
func log1Name() string    { return synthName + ".LOG1" }
func log2Name() string    { return synthName + ".LOG2" }

func TestReaderReadsKeysAndValuesOfACleanHive(t *testing.T) {
	bins, root := synthHive(oldText, false)
	records, failures, reader := readAll(t, synthFile{primaryName(), primaryFile(bins, 5, 5, root)})
	if len(failures) != 0 {
		t.Fatalf("failures = %v", failures)
	}
	var paths []string
	for _, record := range records {
		path, _ := fieldText(record.Fields, fieldKeyPath)
		paths = append(paths, path)
	}
	if got := strings.Join(paths, " "); got != `\ \Alpha \Alpha\Leaf \Beta.Gamma` {
		t.Fatalf("paths = %s", got)
	}
	alpha := recordAt(t, records, synthPathText)
	for name, want := range map[string]string{
		"Value":                    "default",
		"Value.Text":               oldText,
		"Value.Name.With.Dot":      "4294914105 (0xFFFF3039)",
		"Value.Q":                  "1 (0x0000000000000001)",
		"Value.Multi":              "a\nb",
		"Value.Bin":                "0102030405060708",
		"Value.Big":                strings.Repeat("ab", bigDataSegmentSize+10),
		"ValueType[Name.With.Dot]": "REG_DWORD",
		"ValueType[]":              "REG_SZ",
		"LastWrittenTime":          "2001-02-03T04:05:06.000000Z",
	} {
		if got, _ := fieldText(alpha.Fields, name); got != want {
			t.Errorf("%s = %.80q, want %.80q", name, got, want)
		}
	}
	if alpha.ObservedAt == nil || *alpha.ObservedAt.Normalized != "2001-02-03T04:05:06.000000Z" {
		t.Errorf("ObservedAt = %v", alpha.ObservedAt)
	}
	if _, ok := fieldText(alpha.Fields, fieldAppliedLogEntry); ok {
		t.Error("a clean hive carries AppliedLogEntry")
	}
	// 位置は主 file の base block の後ろの cell を指し、原文はその cell の byte である。
	if alpha.ByteOffset < baseBlockSize || alpha.ByteLength != int64(len(alpha.RawText)/2) {
		t.Errorf("alpha at %d+%d with %d hex digits", alpha.ByteOffset, alpha.ByteLength, len(alpha.RawText))
	}
	beta := recordAt(t, records, `\Beta.Gamma`)
	if beta.ObservedAt != nil {
		t.Error("a key written at FILETIME 0 carries a time")
	}
	header := headerMap(reader)
	for name, want := range map[string]string{
		"Recovery": "not_needed", "Dirty": "false", "HiveFileName": synthName,
		"Log." + log1Name(): "absent", "Log." + log2Name(): "absent",
	} {
		if header[name] != want {
			t.Errorf("header %s = %q, want %q", name, header[name], want)
		}
	}
}

func TestReaderPointsEveryValueCellIntoTheSource(t *testing.T) {
	bins, root := synthHive(oldText, false)
	primary := primaryFile(bins, 5, 5, root)
	records, _, _ := readAll(t, synthFile{primaryName(), primary})
	alpha := recordAt(t, records, synthPathText)
	cells, _ := fieldText(alpha.Fields, "ValueCell[Text]")
	// vk cell と data cell の 2 つの範囲を持ち、各範囲の byte は主 file の同じ位置の byte である。
	parts := strings.Fields(cells)
	if len(parts) != 2 {
		t.Fatalf("ValueCell[Text] = %q", cells)
	}
	offset, length := parseSpan(t, parts[1])
	if !bytes.Contains(primary[offset:offset+length], utf16Bytes(oldText)) {
		t.Errorf("the data cell at %d+%d does not hold the value", offset, length)
	}
	if big, _ := fieldText(alpha.Fields, "ValueCell[Big]"); len(strings.Fields(big)) != 5 {
		t.Errorf("ValueCell[Big] = %q, want vk, db, segment list and 2 segments", big)
	}
}

func TestReaderAppliesALogToADirtyHive(t *testing.T) {
	oldBins, root := synthHive(oldText, false)
	newBins, _ := synthHive(newText, false)
	primary := primaryFile(oldBins, 11, 10, root)
	log := logBytes(root, synthEntry{sequence: 11, binsSize: uint32(len(newBins)), pages: changedPages(oldBins, newBins)})
	records, failures, reader := readAll(t, synthFile{primaryName(), primary}, synthFile{log1Name(), log})
	if len(failures) != 0 {
		t.Fatalf("failures = %v", failures)
	}
	alpha := recordAt(t, records, synthPathText)
	if got, _ := fieldText(alpha.Fields, "Value.Text"); got != newText {
		t.Errorf("Value.Text = %q", got)
	}
	if got, _ := fieldText(alpha.Fields, fieldAppliedLogEntry); got != log1Name()+" sequence 11" {
		t.Errorf("AppliedLogEntry = %q", got)
	}
	// 値の data cell は log の page から来て、位置は連結した byte 列の log の範囲を指す。
	cells, _ := fieldText(alpha.Fields, "ValueCell[Text]")
	offset, length := parseSpan(t, strings.Fields(cells)[1])
	content := append(bytes.Clone(primary), log...)
	if offset < int64(len(primary)) || !bytes.Contains(content[offset:offset+length], utf16Bytes(newText)) {
		t.Errorf("the data cell at %d+%d does not lie in the log", offset, length)
	}
	header := headerMap(reader)
	for name, want := range map[string]string{
		"Recovery": "applied", "Dirty": "true", "AppliedSequenceRange": "11-11",
		"PrimarySequenceNumber": "11", "SecondarySequenceNumber": "10",
		"Log." + log2Name(): "absent",
	} {
		if header[name] != want {
			t.Errorf("header %s = %q, want %q", name, header[name], want)
		}
	}
	if !strings.HasSuffix(header["Log."+log1Name()], "applied 11-11") {
		t.Errorf("log status = %q", header["Log."+log1Name()])
	}
}

func TestReaderAppliesBothLogsFromTheEarlierSequence(t *testing.T) {
	oldBins, root := synthHive(oldText, false)
	midBins, _ := synthHive(midText, false)
	newBins, _ := synthHive(newText, false)
	primary := primaryFile(oldBins, 11, 10, root)
	later := logBytes(root, synthEntry{sequence: 12, binsSize: uint32(len(newBins)), pages: changedPages(midBins, newBins)})
	earlier := logBytes(root, synthEntry{sequence: 11, binsSize: uint32(len(midBins)), pages: changedPages(oldBins, midBins)})
	for _, tc := range []struct {
		name      string
		laterSeq  uint32
		wantText  string
		wantRange string
	}{
		{name: "continues", laterSeq: 12, wantText: newText, wantRange: "11-12"},
		{name: "gap", laterSeq: 13, wantText: midText, wantRange: "11-11"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			log1 := later
			if tc.laterSeq != 12 {
				log1 = logBytes(root, synthEntry{sequence: tc.laterSeq, binsSize: uint32(len(newBins)), pages: changedPages(midBins, newBins)})
			}
			records, _, reader := readAll(t, synthFile{primaryName(), primary},
				synthFile{log1Name(), log1}, synthFile{log2Name(), earlier})
			if got, _ := fieldText(recordAt(t, records, synthPathText).Fields, "Value.Text"); got != tc.wantText {
				t.Errorf("Value.Text = %q, want %q", got, tc.wantText)
			}
			if got := headerMap(reader)["AppliedSequenceRange"]; got != tc.wantRange {
				t.Errorf("AppliedSequenceRange = %q, want %q", got, tc.wantRange)
			}
		})
	}
}

func TestReaderGrowsTheHiveBinsDataFromALog(t *testing.T) {
	oldBins, root := synthHive(oldText, false)
	newBins, _ := synthHive(oldText, true)
	primary := primaryFile(oldBins, 11, 10, root)
	log := logBytes(root, synthEntry{sequence: 11, binsSize: uint32(len(newBins)), pages: changedPages(oldBins, newBins)})
	records, failures, reader := readAll(t, synthFile{primaryName(), primary}, synthFile{log1Name(), log})
	if len(failures) != 0 {
		t.Fatalf("failures = %v", failures)
	}
	extra := recordAt(t, records, `\Extra`)
	if extra.ByteOffset < int64(len(primary)) {
		t.Errorf("the key added by the log lies at %d, inside the primary file", extra.ByteOffset)
	}
	if got := headerMap(reader)["RecoveredHiveBinsDataSize"]; got != strconv.Itoa(len(newBins)) {
		t.Errorf("RecoveredHiveBinsDataSize = %q", got)
	}
}

func TestReaderStopsAtAnEntryWithAWrongHash(t *testing.T) {
	oldBins, root := synthHive(oldText, false)
	midBins, _ := synthHive(midText, false)
	newBins, _ := synthHive(newText, false)
	log := logBytes(root,
		synthEntry{sequence: 11, binsSize: uint32(len(midBins)), pages: changedPages(oldBins, midBins)},
		synthEntry{sequence: 12, binsSize: uint32(len(newBins)), pages: changedPages(midBins, newBins)},
	)
	log[len(log)-1] ^= 0xFF
	records, _, reader := readAll(t, synthFile{primaryName(), primaryFile(oldBins, 11, 10, root)}, synthFile{log1Name(), log})
	if got, _ := fieldText(recordAt(t, records, synthPathText).Fields, "Value.Text"); got != midText {
		t.Errorf("Value.Text = %q", got)
	}
	if status := headerMap(reader)["Log."+log1Name()]; !strings.Contains(status, "Hash-1 mismatch") {
		t.Errorf("log status = %q", status)
	}
}

func TestReaderStopsAtAnEntryThatGrowsPastItsPages(t *testing.T) {
	oldBins, root := synthHive(oldText, false)
	newBins, _ := synthHive(newText, false)
	const grown = 1 << 30
	for name, pages := range map[string][]synthPage{
		"no page past the old size": changedPages(oldBins, newBins),
		// 末尾の page 1 つだけが、広げた範囲の端を指す。
		"one page at the far end": append(changedPages(oldBins, newBins),
			synthPage{rel: uint32(len(newBins)) + grown - unitSize, data: make([]byte, unitSize)}),
	} {
		t.Run(name, func(t *testing.T) {
			assertStopsGrowing(t, oldBins, root, logBytes(root, synthEntry{
				sequence: 11, binsSize: uint32(len(newBins)) + grown, pages: pages,
			}))
		})
	}
}

func assertStopsGrowing(t *testing.T, oldBins []byte, root uint32, log []byte) {
	t.Helper()
	records, failures, reader := readAll(t, synthFile{primaryName(), primaryFile(oldBins, 11, 10, root)}, synthFile{log1Name(), log})
	if len(failures) != 1 {
		t.Fatalf("failures = %v, want the dirty hive left unrecovered", failures)
	}
	if got, _ := fieldText(recordAt(t, records, synthPathText).Fields, "Value.Text"); got != oldText {
		t.Errorf("Value.Text = %q", got)
	}
	if status := headerMap(reader)["Log."+log1Name()]; !strings.Contains(status, "grows the hive bins data past its pages") {
		t.Errorf("log status = %q", status)
	}
}

func TestReaderReportsADirtyHiveWithoutApplicableLogs(t *testing.T) {
	oldBins, root := synthHive(oldText, false)
	newBins, _ := synthHive(newText, false)
	legacy := baseBlockBytes(logBaseBlockSize, 30, 30, fileTypeLogLegacy, root, uint32(len(oldBins)))
	stale := logBytes(root, synthEntry{sequence: 11, binsSize: uint32(len(newBins)), pages: changedPages(oldBins, newBins)})
	for _, tc := range []struct {
		name       string
		logs       []synthFile
		wantStatus map[string]string
	}{
		{name: "empty and legacy", logs: []synthFile{{log1Name(), nil}, {log2Name(), legacy}},
			wantStatus: map[string]string{log1Name(): "empty", log2Name(): "legacy format (file type 1) unsupported"}},
		{name: "older than the primary file", logs: []synthFile{{log1Name(), stale}},
			wantStatus: map[string]string{log2Name(): "absent"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := append([]synthFile{{primaryName(), primaryFile(oldBins, 21, 20, root)}}, tc.logs...)
			records, failures, reader := readAll(t, files...)
			if len(failures) != 1 || *failures[0].ByteOffset != 0 {
				t.Fatalf("failures = %v, want 1 at the base block", failures)
			}
			if got, _ := fieldText(recordAt(t, records, synthPathText).Fields, "Value.Text"); got != oldText {
				t.Errorf("Value.Text = %q, want the primary file value", got)
			}
			header := headerMap(reader)
			if header["Recovery"] != "not_applied" {
				t.Errorf("Recovery = %q", header["Recovery"])
			}
			for name, want := range tc.wantStatus {
				if header["Log."+name] != want {
					t.Errorf("Log.%s = %q, want %q", name, header["Log."+name], want)
				}
			}
		})
	}
}

func TestReaderUsesTheLatestLogWhenThePrimaryBaseBlockIsInvalid(t *testing.T) {
	oldBins, root := synthHive(oldText, false)
	midBins, _ := synthHive(midText, false)
	newBins, _ := synthHive(newText, false)
	primary := primaryFile(oldBins, 10, 10, root)
	primary[checksumOffset] ^= 0xFF
	earlier := logBytes(root, synthEntry{sequence: 11, binsSize: uint32(len(midBins)), pages: changedPages(oldBins, midBins)})
	later := logBytes(root, synthEntry{sequence: 12, binsSize: uint32(len(newBins)), pages: changedPages(oldBins, newBins)})
	records, _, reader := readAll(t, synthFile{primaryName(), primary},
		synthFile{log1Name(), later}, synthFile{log2Name(), earlier})
	if got, _ := fieldText(recordAt(t, records, synthPathText).Fields, "Value.Text"); got != newText {
		t.Errorf("Value.Text = %q", got)
	}
	header := headerMap(reader)
	if header["BaseBlockChecksum"] != "invalid" || header["AppliedSequenceRange"] != "12-12" {
		t.Errorf("header = %v", header)
	}
	if !strings.Contains(header["Log."+log2Name()], "not applied") {
		t.Errorf("the earlier log status = %q", header["Log."+log2Name()])
	}
}

func TestReaderReportsUnreadableCellsAndKeepsTheRest(t *testing.T) {
	b := newHiveBuilder()
	broken := b.rawKey("Broken", synthLastWritten, 1, 0x7FFFFFF0, 0, noCell)
	loop := b.rawKey("Loop", synthLastWritten, 0, noCell, 0, noCell)
	sibling := b.key("Sibling", synthLastWritten, nil, nil)
	root := b.key("ROOT", synthLastWritten, []uint32{broken, loop, sibling}, nil)
	// Loop の subkey list が root を指し、同じ key に 2 回着く。
	loopList := b.list("li", []uint32{root})
	binary.LittleEndian.PutUint32(b.bins[loop+4+20:], 1)
	binary.LittleEndian.PutUint32(b.bins[loop+4+28:], loopList)
	records, failures, _ := readAll(t, synthFile{primaryName(), primaryFile(b.binsData(), 1, 1, root)})
	if len(records) != 4 {
		t.Errorf("records = %d, want root, Broken, Loop and Sibling", len(records))
	}
	if len(failures) != 2 {
		t.Fatalf("failures = %d, want the subkey list of Broken and the loop", len(failures))
	}
	if got := *failures[0].ByteOffset; got != baseBlockSize+int64(broken) {
		t.Errorf("the subkey list failure lies at %d, want the key of Broken", got)
	}
	if !strings.Contains(failures[1].ObservedResult, "referenced twice") {
		t.Errorf("loop failure = %q", failures[1].ObservedResult)
	}
}

// key ごとに同じ subkey list と value list を指させても、2 回目からの参照は key 1 つに失敗 1 件
// であり、list を展開し直さない。
func TestReaderReadsASharedListOnce(t *testing.T) {
	const keys = 30
	b := newHiveBuilder()
	value := b.value("V", regDword, []byte{0x5A, 0, 0, 0})
	valueList := b.cell([]byte{byte(value), byte(value >> 8), byte(value >> 16), byte(value >> 24)})
	offsets := make([]uint32, keys)
	for i := range offsets {
		offsets[i] = b.rawKey("K"+strconv.Itoa(i), synthLastWritten, 0, noCell, 1, valueList)
	}
	shared := b.list("li", offsets)
	for _, offset := range offsets {
		binary.LittleEndian.PutUint32(b.bins[offset+4+20:], keys)
		binary.LittleEndian.PutUint32(b.bins[offset+4+28:], shared)
	}
	root := b.rawKey("ROOT", synthLastWritten, keys, shared, 0, noCell)
	records, failures, _ := readAll(t, synthFile{primaryName(), primaryFile(b.binsData(), 1, 1, root)})
	// root が list を読み、K0 から K29 を 1 回ずつ読む。K ごとの list と value list の 2 回目の
	// 参照は失敗になる。
	if len(records) != keys+1 || len(failures) != 2*keys-1 {
		t.Errorf("records = %d, failures = %d, want %d and %d", len(records), len(failures), keys+1, 2*keys-1)
	}
}

func TestReaderRejectsBigDataLargerThanItsSegments(t *testing.T) {
	b := newHiveBuilder()
	value := b.value("Big", regBinary, bytes.Repeat([]byte{0xAB}, bigDataSegmentSize+10))
	// 宣言の byte 数を segment 2 つが持てる数より大きくする。
	binary.LittleEndian.PutUint32(b.bins[value+4+4:], 0x7FFFFFFF)
	root := b.key("ROOT", synthLastWritten, nil, []uint32{value})
	records, failures, _ := readAll(t, synthFile{primaryName(), primaryFile(b.binsData(), 1, 1, root)})
	if len(records) != 1 || len(failures) != 1 || !strings.Contains(failures[0].ObservedResult, "more than 2 segments hold") {
		t.Fatalf("records = %d, failures = %v", len(records), failures)
	}
	if got, _ := fieldText(records[0].Fields, fieldUnreadableValueCount); got != "1" {
		t.Errorf("UnreadableValueCount = %q", got)
	}
}

func TestReaderRejectsAFileWithoutABaseBlock(t *testing.T) {
	records, failures, reader := readAll(t, synthFile{primaryName(), bytes.Repeat([]byte{0x41}, 5000)})
	if len(records) != 0 || len(failures) != 1 || len(reader.SourceHeader()) != 0 {
		t.Errorf("records = %d, failures = %d, header = %d", len(records), len(failures), len(reader.SourceHeader()))
	}
}

func TestReaderNamesTheTerminalFromTheComputerNameKey(t *testing.T) {
	b := newHiveBuilder()
	computerName := b.key("ComputerName", synthLastWritten, nil, []uint32{
		b.value("ComputerName", regSZ, utf16Bytes("HOST-A")),
	})
	path := []string{"ControlSet007", "Control", "ComputerName"}
	child := computerName
	for i := len(path) - 1; i >= 0; i-- {
		child = b.key(path[i], synthLastWritten, []uint32{child}, nil)
	}
	root := b.key("ROOT", synthLastWritten, []uint32{child}, nil)
	records, _, _ := readAll(t, synthFile{primaryName(), primaryFile(b.binsData(), 1, 1, root)})
	record := recordAt(t, records, `\ControlSet007\Control\ComputerName\ComputerName`)
	if len(record.TerminalCandidates) != 1 || record.TerminalCandidates[0] != "HOST-A" {
		t.Errorf("TerminalCandidates = %v", record.TerminalCandidates)
	}
	if !slices.Equal(record.TerminalNames, []string{"HOST-A"}) {
		t.Errorf("TerminalNames = %v", record.TerminalNames)
	}
}

func TestReaderNamesTheTerminalFromTheTcpipHostname(t *testing.T) {
	b := newHiveBuilder()
	parameters := b.key("Parameters", synthLastWritten, nil, []uint32{
		b.value("Domain", regSZ, utf16Bytes("example.test")),
		b.value("Hostname", regSZ, utf16Bytes("host-a")),
		b.value("Hostname", regDword, []byte{1, 0, 0, 0}),
	})
	other := b.key("Other", synthLastWritten, nil, []uint32{b.value("Hostname", regSZ, utf16Bytes("host-z"))})
	path := []string{"ControlSet007", "Services", "Tcpip"}
	child := parameters
	for i := len(path) - 1; i >= 0; i-- {
		child = b.key(path[i], synthLastWritten, []uint32{child}, nil)
	}
	root := b.key("ROOT", synthLastWritten, []uint32{child, other}, nil)
	records, _, _ := readAll(t, synthFile{primaryName(), primaryFile(b.binsData(), 1, 1, root)})
	record := recordAt(t, records, `\ControlSet007\Services\Tcpip\Parameters`)
	if !slices.Equal(record.TerminalNames, []string{"host-a"}) || len(record.TerminalCandidates) != 0 {
		t.Errorf("TerminalNames = %v, TerminalCandidates = %v", record.TerminalNames, record.TerminalCandidates)
	}
	if names := recordAt(t, records, `\Other`).TerminalNames; len(names) != 0 {
		t.Errorf("a Hostname value outside the TCP/IP parameters named the terminal %v", names)
	}
}

func headerMap(reader *Reader) map[string]string {
	header := map[string]string{}
	for _, field := range reader.SourceHeader() {
		if field.Text != nil {
			header[field.Name] = *field.Text.RawText
		} else {
			header[field.Name] = *field.Timestamp.Normalized
		}
	}
	return header
}

func parseSpan(t *testing.T, text string) (int64, int64) {
	t.Helper()
	offset, length, ok := strings.Cut(text, "+")
	o, err1 := strconv.ParseInt(offset, 10, 64)
	l, err2 := strconv.ParseInt(length, 10, 64)
	if !ok || err1 != nil || err2 != nil {
		t.Fatalf("span %q", text)
	}
	return o, l
}
