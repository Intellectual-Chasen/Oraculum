package winregistry

import (
	"encoding/binary"
	"slices"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// テスト用の識別子やパスである。
const (
	synthSha1      = "0123456789abcdef0123456789abcdef01234567"
	synthExePath   = `c:\synth\tool.exe`
	synthTaskName  = `\SynthFolder\SynthTask`
	synthTaskClsid = "0123456789ABCDEF"
)

// keyAt は path の各要素の key を root から順に置き、最後の key に values を持たせ、root を返す。
func keyAt(b *hiveBuilder, path []string, values []uint32) uint32 {
	child := b.key(path[len(path)-1], synthLastWritten, nil, values)
	for i := len(path) - 2; i >= 0; i-- {
		child = b.key(path[i], synthLastWritten, []uint32{child}, nil)
	}
	return b.key("ROOT", synthLastWritten, []uint32{child}, nil)
}

// fieldNamed は名前が name の項目を返す。
func fieldNamed(t *testing.T, fields []core.RecordField, name string) core.RecordField {
	t.Helper()
	for _, field := range fields {
		if field.Name == name {
			return field
		}
	}
	t.Fatalf("no field %s", name)
	return core.RecordField{}
}

// assertSemanticValue は項目 name が semantic を持ち、比べる値が want であることを確かめる。
func assertSemanticValue(t *testing.T, fields []core.RecordField, name string, semantic core.SemanticKey, want string) {
	t.Helper()
	field := fieldNamed(t, fields, name)
	if field.Semantic != semantic {
		t.Errorf("%s semantic = %q, want %q", name, field.Semantic, semantic)
	}
	var got string
	if field.Timestamp != nil {
		got = *field.Timestamp.Normalized
	} else {
		got, _ = field.Text.ComparableValue()
	}
	if got != want {
		t.Errorf("%s = %q, want %q", name, got, want)
	}
}

func amcacheValues(b *hiveBuilder, fileId string) []uint32 {
	size := make([]byte, qwordSize)
	binary.LittleEndian.PutUint64(size, 4096)
	return []uint32{
		b.value("LowerCaseLongPath", regSZ, utf16Bytes(synthExePath)),
		b.value("Name", regSZ, utf16Bytes("Tool.exe")),
		b.value("FileId", regSZ, utf16Bytes(fileId)),
		b.value("Size", regQword, size),
		b.value("LinkDate", regSZ, utf16Bytes("02/03/2001 04:05:06")),
		b.value("ProductName", regSZ, utf16Bytes("synth product")),
		b.value("OriginalFileName", regSZ, utf16Bytes("tool.exe")),
		b.value("Publisher", regSZ, utf16Bytes("synth publisher")),
	}
}

func TestReaderMapsTheAmcacheFileEntryToFileItems(t *testing.T) {
	b := newHiveBuilder()
	root := keyAt(b, []string{"Root", "InventoryApplicationFile", "tool.exe|00aa"}, amcacheValues(b, "0000"+synthSha1))
	records, _, _ := readAll(t, synthFile{primaryName(), primaryFile(b.binsData(), 1, 1, root)})
	fields := recordAt(t, records, `\Root\InventoryApplicationFile\tool.exe|00aa`).Fields
	assertSemanticValue(t, fields, "Value.LowerCaseLongPath", core.SemanticKeyFilePath, synthExePath)
	assertSemanticValue(t, fields, "Value.Name", core.SemanticKeyFileName, "Tool.exe")
	assertSemanticValue(t, fields, "Value.FileId", core.SemanticKeyFileSha1, synthSha1)
	assertSemanticValue(t, fields, "Value.Size", core.SemanticKeyFileSizeBytes, "4096")
	assertSemanticValue(t, fields, "Value.LinkDate", core.SemanticKeyFileLinkTime, "2001-02-03T04:05:06Z")
	assertSemanticValue(t, fields, "Value.ProductName", core.SemanticKeyFileProductName, "synth product")
	assertSemanticValue(t, fields, "Value.OriginalFileName", core.SemanticKeyFileOriginalFileName, "tool.exe")
	assertSemanticValue(t, fields, "Value.Publisher", core.SemanticKeyFilePublisher, "synth publisher")
	// 原資料の文字列は変えない。
	if raw, _ := fieldText(fields, "Value.FileId"); raw != "0000"+synthSha1 {
		t.Errorf("Value.FileId raw = %q", raw)
	}
	// レコードは端末の範囲のファイルのノードを指す。
	terminal := core.NodeKey{Kind: core.NodeKindTerminal, Form: core.NodeKeyFormTerminalId,
		Values: []core.NodeIdentityValue{{Semantic: core.SemanticKeyTerminalId, Value: "synth-terminal"}}}
	graph := core.NewRecordGraphInScope(fields, core.RecordScope{Terminal: &terminal})
	if !slices.ContainsFunc(graph.Nodes, func(key core.NodeKey) bool { return key.Kind == core.NodeKindFile }) {
		t.Errorf("the record names no file node: %v", graph.Nodes)
	}
}

// FileId が 4 個の 0 と SHA-1 の形を持たないとき、また key が InventoryApplicationFile の下に無い
// ときは語彙へ写さない。
func TestReaderLeavesOtherAmcacheValuesUnmapped(t *testing.T) {
	b := newHiveBuilder()
	root := keyAt(b, []string{"Root", "InventoryApplicationFile", "tool.exe|00aa"}, amcacheValues(b, "1111"+synthSha1))
	records, _, _ := readAll(t, synthFile{primaryName(), primaryFile(b.binsData(), 1, 1, root)})
	fields := recordAt(t, records, `\Root\InventoryApplicationFile\tool.exe|00aa`).Fields
	if field := fieldNamed(t, fields, "Value.FileId"); field.Semantic != "" {
		t.Errorf("Value.FileId semantic = %q", field.Semantic)
	}

	b = newHiveBuilder()
	root = keyAt(b, []string{"Root", "OtherInventory", "tool.exe|00aa"}, amcacheValues(b, "0000"+synthSha1))
	records, _, _ = readAll(t, synthFile{primaryName(), primaryFile(b.binsData(), 1, 1, root)})
	for _, field := range recordAt(t, records, `\Root\OtherInventory\tool.exe|00aa`).Fields {
		if field.Semantic != "" {
			t.Errorf("%s semantic = %q", field.Name, field.Semantic)
		}
	}
}

// actionString は Actions の値の文字列 (uint32 の byte 数と、NUL を持たない UTF-16LE) である。
func actionString(text string) []byte {
	units := utf16Bytes(text)
	units = units[:len(units)-2]
	return append(binary.LittleEndian.AppendUint32(nil, uint32(len(units))), units...)
}

// execAction は実行の操作である。形式 3 では flag を後ろに置く。
func execAction(version uint16, command, arguments string) []byte {
	data := binary.LittleEndian.AppendUint16(nil, taskActionExec)
	for _, text := range []string{"", command, arguments, ""} {
		data = append(data, actionString(text)...)
	}
	if version == taskActionsContextVersion {
		data = binary.LittleEndian.AppendUint16(data, 0)
	}
	return data
}

func comAction() []byte {
	data := binary.LittleEndian.AppendUint16(nil, taskActionComHandler)
	data = append(data, actionString("")...)
	data = append(data, synthTaskClsid...)
	return append(data, actionString("synth-data")...)
}

// actionsValue は形式 3 の Actions の値である。
func actionsValue(actions ...[]byte) []byte {
	data := binary.LittleEndian.AppendUint16(nil, taskActionsContextVersion)
	data = append(data, actionString("SynthUsers")...)
	for _, action := range actions {
		data = append(data, action...)
	}
	return data
}

var taskCachePath = []string{"Microsoft", "Windows NT", "CurrentVersion", "Schedule", "TaskCache", "Tasks",
	"{00000000-1111-2222-3333-444444444444}"}

func TestReaderMapsTheTaskCacheTaskToScheduledTaskItems(t *testing.T) {
	b := newHiveBuilder()
	root := keyAt(b, taskCachePath, []uint32{
		b.value("Path", regSZ, utf16Bytes(synthTaskName)),
		b.value("Actions", regBinary, actionsValue(
			execAction(3, `C:\synth\run.exe`, "-synth"),
			comAction(),
			execAction(3, `%SystemRoot%\synth.exe`, ""),
		)),
	})
	records, _, _ := readAll(t, synthFile{primaryName(), primaryFile(b.binsData(), 1, 1, root)})
	record := recordAt(t, records, `\`+strings.Join(taskCachePath, `\`))
	fields := record.Fields
	assertSemanticValue(t, fields, "Value.Path", core.SemanticKeyScheduledTaskName, synthTaskName)
	assertSemanticValue(t, fields, "Action.Command", core.SemanticKeyScheduledTaskCommand, `C:\synth\run.exe`)
	assertSemanticValue(t, fields, "Action.CommandPath", core.SemanticKeyFilePath, `C:\synth\run.exe`)
	assertSemanticValue(t, fields, "Action.Arguments", "", "-synth")
	assertSemanticValue(t, fields, "Action.Command#2", core.SemanticKeyScheduledTaskCommand, `%SystemRoot%\synth.exe`)
	// 環境変数で始まるコマンドはファイルのノードにせず、空の引数は項目にしない。
	for _, name := range []string{"Action.CommandPath#2", "Action.Arguments#2"} {
		if slices.ContainsFunc(fields, func(f core.RecordField) bool { return f.Name == name }) {
			t.Errorf("the record carries %s", name)
		}
	}
	// DynamicInfo を持たない key のレコードの時刻は、key の最終更新の時刻である。
	if record.ObservedAt == nil || *record.ObservedAt.Normalized != "2001-02-03T04:05:06.000000Z" {
		t.Errorf("ObservedAt = %v", record.ObservedAt)
	}
}

// DynamicInfo を持つ key のレコードの時刻は、DynamicInfo が書く登録の時刻である。
func TestReaderTimesTheTaskCacheTaskAtItsRegistration(t *testing.T) {
	// 登録の時刻は key の最終更新の 1 時間前 (2001-02-03T03:05:06Z) である。
	dynamicInfo := binary.LittleEndian.AppendUint32(nil, 3)
	dynamicInfo = binary.LittleEndian.AppendUint64(dynamicInfo, synthLastWritten-36_000_000_000)
	dynamicInfo = append(dynamicInfo, make([]byte, 24)...)
	b := newHiveBuilder()
	root := keyAt(b, taskCachePath, []uint32{
		b.value("Path", regSZ, utf16Bytes(synthTaskName)),
		b.value("DynamicInfo", regBinary, dynamicInfo),
	})
	records, _, _ := readAll(t, synthFile{primaryName(), primaryFile(b.binsData(), 1, 1, root)})
	record := recordAt(t, records, `\`+strings.Join(taskCachePath, `\`))
	const registered = "2001-02-03T03:05:06.000000Z"
	assertSemanticValue(t, record.Fields, fieldTaskRegisteredTime, core.SemanticKeyEventTime, registered)
	if record.ObservedAt == nil || *record.ObservedAt.Normalized != registered {
		t.Errorf("ObservedAt = %v", record.ObservedAt)
	}
	if got, _ := fieldText(record.Fields, fieldLastWrittenTime); got != "2001-02-03T04:05:06.000000Z" {
		t.Errorf("LastWrittenTime = %q", got)
	}
}

// SystemRootOf は CurrentVersion の key のレコードだけから SystemRoot の値を返す。
func TestSystemRootOfReadsTheCurrentVersionKey(t *testing.T) {
	b := newHiveBuilder()
	root := keyAt(b, []string{"Microsoft", "Windows NT", "CurrentVersion"}, []uint32{
		b.value("SystemRoot", regSZ, utf16Bytes(`Y:\Synth`)),
	})
	records, _, _ := readAll(t, synthFile{primaryName(), primaryFile(b.binsData(), 1, 1, root)})
	if got, ok := SystemRootOf(recordAt(t, records, `\Microsoft\Windows NT\CurrentVersion`).Fields); !ok || got != `Y:\Synth` {
		t.Errorf("SystemRootOf() = %q, %v; want the SystemRoot value", got, ok)
	}
	if _, ok := SystemRootOf(recordAt(t, records, `\Microsoft\Windows NT`).Fields); ok {
		t.Error("the parent key gave a SystemRoot")
	}
}

func TestParseTaskActionsReadsTheLayouts(t *testing.T) {
	version1 := binary.LittleEndian.AppendUint16(nil, 1)
	version1 = append(version1, execAction(1, `C:\a.exe`, "x")...)
	full := actionsValue(execAction(3, `C:\a.exe`, "x"), execAction(3, `C:\b.exe`, ""))
	cases := map[string]struct {
		data []byte
		want []taskAction
	}{
		"形式 1 は文脈と flag を持たない": {version1, []taskAction{{`C:\a.exe`, "x"}}},
		"形式 3 の 2 つの操作":        {full, []taskAction{{`C:\a.exe`, "x"}, {`C:\b.exe`, ""}}},
		// 2 つ目の操作の途中で切れた値は、1 つ目の操作だけを返す。
		"途中で切れた値":            {full[:len(full)-3], []taskAction{{`C:\a.exe`, "x"}}},
		"知らない種類の操作で止まる":      {actionsValue(execAction(3, `C:\a.exe`, ""), []byte{0x88, 0x88, 1, 2}), []taskAction{{`C:\a.exe`, ""}}},
		"文字列の byte 数が残りを超える": {actionsValue([]byte{0x66, 0x66, 0xFF, 0xFF, 0xFF, 0xFF}), nil},
		"形式 0": {[]byte{0, 0}, nil},
		"形式 4": {[]byte{4, 0}, nil},
		"空":    {nil, nil},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := parseTaskActions(c.data); !slices.Equal(got, c.want) {
				t.Errorf("parseTaskActions = %q, want %q", got, c.want)
			}
		})
	}
}

// FuzzTaskActions は任意の byte 列の Actions の値を読んでもパニックしないことを確かめる。
func FuzzTaskActions(f *testing.F) {
	f.Add(actionsValue(execAction(3, `C:\a.exe`, "x"), comAction()))
	f.Add([]byte{1, 0, 0x66, 0x66})
	f.Add([]byte{3, 0, 0xFF, 0xFF, 0xFF, 0x7F})
	f.Fuzz(func(t *testing.T, data []byte) {
		for _, action := range parseTaskActions(data) {
			if len(action.command) > 2*len(data) {
				t.Fatalf("command %d bytes from %d bytes", len(action.command), len(data))
			}
		}
	})
}
