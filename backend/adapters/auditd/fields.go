package auditd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 欠測を表す原資料の文字列。auditd は値が無い欄を空にせず、欄ごとに決まった文字列で埋める。
const (
	// unsetIdText は auid と ses が未設定を表す文字列である。
	unsetIdText = "4294967295"
	// daemonUnsetIdText は DAEMON_END が持つ未設定の文字列である。
	daemonUnsetIdText = "-1"
	// unrecordedText は hostname と addr が記録の無いことを表す文字列である。
	unrecordedText = "?"
	// noRuleNameText は key が監査規則の名前を持たないことを表す文字列である。
	noRuleNameText = "(null)"
)

// absentTexts は欠測を表す文字列と、その文字列が出る key の組である。
//
// **文字列だけで欠測と決めない。** `uid=-1` は DAEMON_END の未設定であるが、`exit=-1` は
// システムコールの返り値である。key を見て判定する。
var absentTexts = map[string][]string{
	keyLoginUserId:   {unsetIdText, daemonUnsetIdText},
	keySession:       {unsetIdText, daemonUnsetIdText},
	keyProcessId:     {daemonUnsetIdText},
	"uid":            {daemonUnsetIdText},
	"old-auid":       {unsetIdText, daemonUnsetIdText},
	"old-ses":        {unsetIdText, daemonUnsetIdText},
	"hostname":       {unrecordedText},
	"addr":           {unrecordedText},
	"key":            {noRuleNameText},
	keyLoginUserName: {"unset"},
}

// 導出した正規化値の出どころを表す文字列。
const (
	derivationRemovedQuotes    = "引用符を外した文字列"
	derivationDecodedProctitle = "proctitle の 16 進表記を byte 列へ戻し、区切りの 0x00 を空白に置き換えた値"
	derivationExecveArguments  = "EXECVE の引数を並べたコマンド行。16 進表記の引数は byte 列へ戻した値"
)

// コマンド行を持つ項目の名前。原資料の key ではないため、導出であることが分かる名前にする。
const (
	commandLineFieldName          = "commandLine"
	proctitleCommandLineFieldName = "proctitleCommandLine"
)

// RecordFields は 1 事象の全項目を、原文の並び順で返す。
//
// **読めた欄をすべて返す。** 語彙の項目へ写さない欄も、原資料の key の文字列を name に
// 持つ項目として返す。
//
// **項目の名前に行の種別を付ける。** 1 事象が複数行に分かれ、同じ key が別の行に出る
// (2 つの PATH の行がどちらも `name` を持つ)。key だけを名前にすると、どの行の値かが
// 応答から消える。
func RecordFields(record Record) ([]core.RecordField, error) {
	fields := make([]core.RecordField, 0, 32)
	// 名前の重複を事象の全体で見る。同じ種別の行が 2 つ並ぶ事象があるためである。
	taken := make(map[string]int, 32)
	pathIndex := 0
	for index, line := range record.Lines() {
		prefix := line.Type()
		switch prefix {
		case LineTypePath:
			prefix = LineTypePath + "[" + strconv.Itoa(pathIndex) + "]"
			pathIndex++
		case "":
			// 種別を読めなかった行も、位置で指定できる形にする。
			prefix = "line[" + strconv.Itoa(index) + "]"
		}
		// 触れた path のうち、事象が記録した file は item=0 の行が持つ。残りの行は
		// 実行に伴って開かれた file であり、同じ語彙の項目を 2 度持たせない。
		lineFields, err := lineFields(line, prefix, prefix == firstPathPrefix, taken)
		if err != nil {
			return nil, err
		}
		fields = append(fields, lineFields...)
	}
	derived, err := derivedFields(record)
	if err != nil {
		return nil, err
	}
	return append(fields, derived...), nil
}

// firstPathPrefix は、事象が記録した file を持つ PATH の行の項目の名前の接頭辞である。
const firstPathPrefix = LineTypePath + "[0]"

// lineFields は 1 行の項目を返す。
func lineFields(
	line Line, prefix string, firstPath bool, taken map[string]int,
) ([]core.RecordField, error) {
	fields := make([]core.RecordField, 0, len(line.items)+4)
	failedSession := (line.Type() == lineTypeUserStart || line.Type() == lineTypeUserEnd) && !sessionSucceeded(line)
	for _, item := range line.Items() {
		semantic := semanticOfItem(line.Type(), item)
		if failedSession && item.Key() == keySession {
			semantic = ""
		}
		if firstPath && item.Key() == keyPathName && !item.Interpreted() {
			semantic = core.SemanticKeyFilePath
		}
		name := uniqueItemName(prefix, item, taken)
		field, err := itemField(name, item, semantic)
		if err != nil {
			return nil, err
		}
		fields = append(fields, field)
		// 内側の key=value は、外側の項目と別の項目として持つ。
		for _, inner := range item.Inner() {
			innerField, err := itemField(uniqueItemName(name, inner, taken), inner, "")
			if err != nil {
				return nil, err
			}
			fields = append(fields, innerField)
		}
	}
	return fields, nil
}

// uniqueItemName は 1 事象の中で重複しない項目の名前を返す。
//
// **同じ key が 1 行に 2 回出る。** `type=USER_START` は事象を指す `msg` と、もう 1 階層の
// key=value を持つ `msg` の両方を持つ。2 回目からは出現の番号を付け、どちらの値かを
// 応答に残す。
func uniqueItemName(prefix string, item Item, taken map[string]int) string {
	name := prefix + "." + item.Key()
	taken[name]++
	if occurrence := taken[name]; occurrence > 1 {
		return name + "#" + strconv.Itoa(occurrence)
	}
	return name
}

// itemField は 1 つの key=value を項目へ直す。
func itemField(name string, item Item, semantic core.SemanticKey) (core.RecordField, error) {
	value, err := itemValue(item)
	if err != nil {
		return core.RecordField{}, err
	}
	field, err := core.NewTextField(name, semantic, value)
	if err != nil {
		return core.RecordField{}, fmt.Errorf("building the field %q: %w", name, err)
	}
	return field, nil
}

// itemValue は 1 つの項目の値を、原資料の文字列と値の状態の組へ直す。
//
// **欠測を表す文字列を 0 や空値へまとめない。** 原資料の文字列をそのまま保ち、値の状態で欠測を表す。
func itemValue(item Item) (core.RawAndNormalized, error) {
	state := core.ValueStatePresent
	if isAbsentText(item.Key(), item.Value()) {
		state = core.ValueStateAbsent
	}
	value, err := core.NewRawValue(state, item.RawValue())
	if err != nil {
		return core.RawAndNormalized{}, fmt.Errorf("building the value of %q: %w", item.Key(), err)
	}
	if !item.Quoted() {
		return value, nil
	}
	// 引用符を外した値を正規化値として別に持つ。原資料の文字列は引用符を含む。
	value, err = core.NewNormalizedValue(state, item.RawValue(), item.Value(),
		derivationRemovedQuotes)
	if err != nil {
		return core.RawAndNormalized{}, fmt.Errorf("building the value of %q: %w", item.Key(), err)
	}
	return value, nil
}

// isAbsentText は、key に対してその文字列が値の不在を表すかを返す。
func isAbsentText(key, value string) bool {
	return containsText(absentTexts[key], value)
}

func containsText(texts []string, value string) bool {
	for _, text := range texts {
		if text == value {
			return true
		}
	}
	return false
}

// derivedFields は、行の組から導く項目を返す。
//
// **コマンド行の出どころを 2 つに分ける。** `type=EXECVE` を持つ事象は引数から組み、
// 持たない事象は `type=PROCTITLE` の復号値だけを持つ。`success=no` の execve は
// `type=SYSCALL` だけを残すため、引数は proctitle からしか読めない。
func derivedFields(record Record) ([]core.RecordField, error) {
	fields := make([]core.RecordField, 0, 4)
	commandLine, err := commandLineField(record)
	if err != nil {
		return nil, err
	}
	fields = append(fields, commandLine)
	proctitle, found, err := proctitleField(record)
	if err != nil {
		return nil, err
	}
	if found {
		fields = append(fields, proctitle)
	}
	return fields, nil
}

// commandLineField はコマンド行の項目を返す。
//
// `type=EXECVE` を持たない事象では、欄が出ない状態を値の状態で表す。
func commandLineField(record Record) (core.RecordField, error) {
	line, found := record.LineOfType(LineTypeExecve)
	if !found {
		return core.NewTextField(commandLineFieldName, core.SemanticKeyProcessCommandLine,
			core.NewAbsentItemValue())
	}
	commandLine, ok := commandLineOf(line)
	if !ok {
		return core.NewTextField(commandLineFieldName, core.SemanticKeyProcessCommandLine,
			core.NewAbsentItemValue())
	}
	value, err := core.NewNormalizedValue(core.ValueStatePresent,
		argumentsRawText(line), commandLine, derivationExecveArguments)
	if err != nil {
		return core.RecordField{}, fmt.Errorf("building the command line: %w", err)
	}
	return core.NewTextField(commandLineFieldName, core.SemanticKeyProcessCommandLine, value)
}

// argumentsRawText は EXECVE の引数の原資料の文字列を並びのまま連結して返す。
func argumentsRawText(line Line) string {
	return strings.Join(argumentsOf(line, Item.RawValue), " ")
}

// proctitleField は proctitle の復号値の項目を返す。
//
// found が偽になるのは、事象が `type=PROCTITLE` を持たないときである。
// 16 進として読めない値は、原資料の文字列を保ったまま定義の範囲の外として返す。
func proctitleField(record Record) (field core.RecordField, found bool, err error) {
	line, found := record.LineOfType(LineTypeProctitle)
	if !found {
		return core.RecordField{}, false, nil
	}
	item, found := line.Item(keyProctitle, false)
	if !found {
		return core.RecordField{}, false, nil
	}
	decoded, readable := decodeProctitle(item)
	if !readable {
		value, err := core.NewRawValue(core.ValueStateOutOfDefinition, item.RawValue())
		if err != nil {
			return core.RecordField{}, false, err
		}
		field, err := core.NewTextField(proctitleCommandLineFieldName, "", value)
		return field, err == nil, err
	}
	value, err := core.NewNormalizedValue(core.ValueStatePresent,
		item.RawValue(), decoded, derivationDecodedProctitle)
	if err != nil {
		return core.RecordField{}, false, fmt.Errorf("building the proctitle command line: %w", err)
	}
	field, err = core.NewTextField(proctitleCommandLineFieldName, "", value)
	return field, err == nil, err
}
