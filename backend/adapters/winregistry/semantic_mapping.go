package winregistry

import (
	"encoding/binary"
	"encoding/hex"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 語彙へ写す key の path。path は hive の root の key の名前を含まない。
//
// Amcache の InventoryApplicationFile の key 1 つは、端末が実行したか置いた実行ファイル 1 つを
// 記録する。TaskCache の Tasks の key 1 つは、登録されたタスク 1 つを記録する。
//
// 既知の制限: ほかの key と値を語彙へ写さない。hive は端末の設定のすべてを key に持ち、
// key ごとにノードを作るとグラフが key で埋まる。値は原資料の名前の検索で見つかる, 写す key を決める
// 要求が 2 つの key の種類にしか無く測る対象が無い, 別の key の値を関係に使う要求が出たとき
var (
	amcacheFileKey   = regexp.MustCompile(`(?i)^\\Root\\InventoryApplicationFile\\[^\\]+$`)
	taskCacheTaskKey = regexp.MustCompile(
		`(?i)^\\Microsoft\\Windows NT\\CurrentVersion\\Schedule\\TaskCache\\Tasks\\\{[^\\]+\}$`)
)

// ItemSemantics は本 package のレコードが持ちうる語彙の項目を返す。
func ItemSemantics() []core.SemanticKey {
	return []core.SemanticKey{
		core.SemanticKeyEventTime, core.SemanticKeyFilePath, core.SemanticKeyFileName, core.SemanticKeyFileSha1,
		core.SemanticKeyFileSizeBytes, core.SemanticKeyFileProductName,
		core.SemanticKeyFileOriginalFileName, core.SemanticKeyFilePublisher, core.SemanticKeyFileLinkTime,
		core.SemanticKeyScheduledTaskName, core.SemanticKeyScheduledTaskCommand,
	}
}

// amcacheTextSemantics は、Amcache の文字列の値のうち、文字列をそのまま語彙の値にする値である。
var amcacheTextSemantics = map[string]core.SemanticKey{
	"LowerCaseLongPath": core.SemanticKeyFilePath,
	"Name":              core.SemanticKeyFileName,
	"ProductName":       core.SemanticKeyFileProductName,
	"OriginalFileName":  core.SemanticKeyFileOriginalFileName,
	"Publisher":         core.SemanticKeyFilePublisher,
}

// 導き方の文。
const (
	derivationFileIdSha1  = "SHA-1 after the four leading zeros of the file id"
	derivationQwordNumber = "decimal number of the QWORD value"
	derivationTaskAction  = "read from an exec action of the task Actions value"
)

// amcacheFileIdPrefix は、Amcache の FileId が SHA-1 の 16 進の前に置く文字列である。
const amcacheFileIdPrefix = "0000"

// amcacheLinkDateLayout は Amcache の LinkDate の書式である。値は UTC の日時である。
const amcacheLinkDateLayout = "01/02/2006 15:04:05"

// semanticValueField は、語彙へ写す key の値 1 つを、語彙の項目を持つ値の項目にする。ok が偽に
// なるのは、key と値が語彙へ写す組でないときと、値が語彙の値の形を持たないときである。
//
// 値の名前が重複した 2 つ目以降の値 (`#<番号>` を付けた値) は写さない。呼び出し側が first で渡す。
func semanticValueField(path, fieldName string, value keyValue, first bool) (core.RecordField, bool) {
	if !first {
		return core.RecordField{}, false
	}
	stringType := value.valueType == regSZ || value.valueType == regExpandSZ
	text := valueText(value.valueType, value.data)
	switch {
	case amcacheFileKey.MatchString(path):
		if semantic, mapped := amcacheTextSemantics[value.name]; mapped && stringType && text != "" {
			return semanticTextField(fieldName, semantic, text), true
		}
		switch {
		case value.name == "FileId" && stringType:
			if sha1, cut := strings.CutPrefix(text, amcacheFileIdPrefix); cut && isSha1Hex(sha1) {
				return normalizedField(fieldName, core.SemanticKeyFileSha1, text, sha1, derivationFileIdSha1), true
			}
		case value.name == "Size" && value.valueType == regQword && len(value.data) == qwordSize:
			size := strconv.FormatUint(binary.LittleEndian.Uint64(value.data), 10)
			return normalizedField(fieldName, core.SemanticKeyFileSizeBytes, text, size, derivationQwordNumber), true
		case value.name == "LinkDate" && stringType:
			return linkTimeField(fieldName, text)
		}
	case taskCacheTaskKey.MatchString(path):
		// Path はタスクの名前であり、フォルダーを `\` で区切った `\<名前>` の形を持つ。イベント
		// ログの TaskName と同じ形である。
		if value.name == "Path" && stringType && strings.HasPrefix(text, `\`) {
			return semanticTextField(fieldName, core.SemanticKeyScheduledTaskName, text), true
		}
	}
	return core.RecordField{}, false
}

// taskCacheFields は、TaskCache の key の値から、タスクの登録の時刻と、実行の操作のコマンドと
// 引数の項目を組む。コマンドが絶対 path のときは、コマンドを file.path の項目にもする。
//
// registered はタスクを登録した時刻であり、DynamicInfo の値が持つ。key の最終更新の時刻は
// タスクを実行するたびに進む。DynamicInfo は uint32 の形式の後ろに登録の時刻の FILETIME (UTC) を
// 置く。
func taskCacheFields(path string, keyValues []keyValue) (fields []core.RecordField, registered *core.Timestamp) {
	if !taskCacheTaskKey.MatchString(path) {
		return nil, nil
	}
	for _, value := range keyValues {
		if value.name != "DynamicInfo" || value.valueType != regBinary || len(value.data) < dwordSize+qwordSize {
			continue
		}
		created := binary.LittleEndian.Uint64(value.data[dwordSize:])
		if created == 0 {
			break
		}
		if field, timestamp, ok := fileTimeField(fieldTaskRegisteredTime, created); ok {
			// 登録の時刻は端末の時計が書いた事象の時刻である。
			timestamp.Clock, timestamp.Meaning = core.ClockTerminalLocal, core.MeaningEvent
			field, _ = core.NewTimestampField(field.Name, core.SemanticKeyEventTime, timestamp)
			fields, registered = append(fields, field), &timestamp
		}
		break
	}
	for _, value := range keyValues {
		if value.name != "Actions" || value.valueType != regBinary {
			continue
		}
		for index, action := range parseTaskActions(value.data) {
			if action.command == "" {
				continue
			}
			fields = append(fields,
				derivedField(numbered("Action.Command", index), core.SemanticKeyScheduledTaskCommand, action.command))
			if isAbsoluteWindowsPath(action.command) {
				fields = append(fields,
					derivedField(numbered("Action.CommandPath", index), core.SemanticKeyFilePath, action.command))
			}
			if action.arguments != "" {
				fields = append(fields, derivedField(numbered("Action.Arguments", index), "", action.arguments))
			}
		}
		break
	}
	return fields, registered
}

// fieldTaskRegisteredTime は、TaskCache の key のタスクの登録の時刻の項目の名前である。
const fieldTaskRegisteredTime = "Task.RegisteredTime"

// taskAction は、タスクが持つ実行の操作である。
type taskAction struct {
	command, arguments string
}

// Actions の値の操作の種類。
const (
	taskActionExec       = 0x6666
	taskActionComHandler = 0x7777
)

// taskActionsContextVersion は、操作の前に実行の文脈の文字列を置く Actions の形式である。
const taskActionsContextVersion = 3

// comClassIdSize は COM の handler の CLSID の byte 数である。
const comClassIdSize = 16

// wordSize は uint16 の byte 数である。
const wordSize = 2

// parseTaskActions は TaskCache の Actions の値から実行の操作を読む。
//
// 値は uint16 の形式 (1 から 3) で始まる。形式 3 は、次に実行の文脈の文字列を置く。続いて、
// uint16 の種類で始まる操作を並べる。文字列は uint32 の byte 数と UTF-16LE である。実行の
// 操作 (0x6666) は識別子、コマンド、引数、作業ディレクトリの文字列を持ち、形式 3 では uint16 の
// flag が続く。COM の handler (0x7777) は識別子の文字列、16 byte の CLSID、data の文字列を持つ。
//
// 読めない byte に達したときと、ほかの種類の操作に達したときは、それまでに読んだ操作を返す。
//
// 既知の制限: 電子メールの送信 (0x8888) とメッセージの表示 (0x9999) の操作で読むのを止め、後ろの
// 操作を読まない, 2 つの操作は Windows 8 以降で廃止され、その操作を持つ Actions の値を repo の
// 中で作れず測れない, 2 つの操作を持つ Actions の値が見つかったとき
func parseTaskActions(data []byte) []taskAction {
	r := byteCursor{data: data}
	version, ok := r.uint16()
	if !ok || version < 1 || version > taskActionsContextVersion {
		return nil
	}
	if version == taskActionsContextVersion {
		if _, ok := r.text(); !ok {
			return nil
		}
	}
	var actions []taskAction
	for len(r.data) > 0 {
		kind, ok := r.uint16()
		if !ok {
			return actions
		}
		switch kind {
		case taskActionExec:
			texts := make([]string, 4)
			for i := range texts {
				if texts[i], ok = r.text(); !ok {
					return actions
				}
			}
			if version == taskActionsContextVersion {
				if _, ok := r.uint16(); !ok {
					return actions
				}
			}
			actions = append(actions, taskAction{command: texts[1], arguments: texts[2]})
		case taskActionComHandler:
			if _, ok := r.text(); !ok || !r.skip(comClassIdSize) {
				return actions
			}
			if _, ok := r.text(); !ok {
				return actions
			}
		default:
			return actions
		}
	}
	return actions
}

// byteCursor は byte 列を先頭から読む。読めないときは位置を進めない。
type byteCursor struct {
	data []byte
}

func (c *byteCursor) uint16() (uint16, bool) {
	if len(c.data) < wordSize {
		return 0, false
	}
	value := binary.LittleEndian.Uint16(c.data)
	c.data = c.data[wordSize:]
	return value, true
}

func (c *byteCursor) skip(size int) bool {
	if len(c.data) < size {
		return false
	}
	c.data = c.data[size:]
	return true
}

// text は uint32 の byte 数と UTF-16LE の文字列を読む。
func (c *byteCursor) text() (string, bool) {
	if len(c.data) < dwordSize {
		return "", false
	}
	size, rest := binary.LittleEndian.Uint32(c.data), c.data[dwordSize:]
	if int64(size) > int64(len(rest)) {
		return "", false
	}
	text, ok := utf16Text(rest[:size])
	if !ok {
		return "", false
	}
	c.data = rest[size:]
	return text, true
}

// isSha1Hex は text が SHA-1 の 16 進 (40 文字) であるかである。
func isSha1Hex(text string) bool {
	_, err := hex.DecodeString(text)
	return len(text) == 40 && err == nil
}

// isAbsoluteWindowsPath は text がドライブ文字か `\\` で始まる path であるかである。
// 環境変数で始まる path は、展開した値を記録から決められないため含めない。
func isAbsoluteWindowsPath(text string) bool {
	if strings.HasPrefix(text, `\\`) {
		return true
	}
	return len(text) >= 3 && text[1] == ':' && text[2] == '\\' &&
		('A' <= text[0] && text[0] <= 'Z' || 'a' <= text[0] && text[0] <= 'z')
}

// linkTimeField は Amcache の LinkDate を file.link_time の時刻の項目にする。
func linkTimeField(name, text string) (core.RecordField, bool) {
	at, err := time.Parse(amcacheLinkDateLayout, text)
	if err != nil {
		return core.RecordField{}, false
	}
	normalized := at.UTC().Format(time.RFC3339)
	timestamp, err := core.NewTimestamp(core.Timestamp{
		RawText: &text, Normalized: &normalized,
		NormalizedForm: core.NormalizedFormRFC3339Absolute, Precision: core.PrecisionSecond,
		OffsetState: core.OffsetStateFormatDefined, Clock: core.ClockFileProperty, Meaning: core.MeaningProperty,
		ValueState: core.ValueStatePresent,
	})
	if err != nil {
		return core.RecordField{}, false
	}
	field, err := core.NewTimestampField(name, core.SemanticKeyFileLinkTime, timestamp)
	return field, err == nil
}

// numbered は 2 番目以降の項目の名前に出現の番号を付ける。
func numbered(name string, index int) string {
	if index == 0 {
		return name
	}
	return name + "#" + strconv.Itoa(index+1)
}

// semanticTextField、normalizedField、derivedField は error を返さない。名前は非空、値は
// present か derived、semantic は語彙の項目か空であるため、検査が失敗しない。
func semanticTextField(name string, semantic core.SemanticKey, text string) core.RecordField {
	value, _ := core.NewRawValue(core.ValueStatePresent, text)
	field, _ := core.NewTextField(name, semantic, value)
	return field
}

func normalizedField(name string, semantic core.SemanticKey, raw, normalized, derivation string) core.RecordField {
	value, _ := core.NewNormalizedValue(core.ValueStatePresent, raw, normalized, derivation)
	field, _ := core.NewTextField(name, semantic, value)
	return field
}

func derivedField(name string, semantic core.SemanticKey, text string) core.RecordField {
	value, _ := core.NewDerivedValue(text, derivationTaskAction)
	field, _ := core.NewTextField(name, semantic, value)
	return field
}
