package prefetch

import (
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 項目の名前。
const (
	fieldExecutableName = "ExecutableName"
	fieldExecutablePath = "ExecutablePath"
	fieldLastRunTime    = "LastRunTime"
	fieldRunCount       = "RunCount"
	fieldPathHash       = "PathHash"
	fieldFormatVersion  = "FormatVersion"
	fieldCompressed     = "Compressed"
	fieldReferencedFile = "ReferencedFile"
	fieldVolumePath     = "Volume.DevicePath"
	fieldVolumeSerial   = "Volume.Serial"
	fieldVolumeCreated  = "Volume.CreatedTime"
)

// runCountUndeterminedDerivation は、形式 30 と 31 の metrics の位置が既知の 2 つの配置の
// どちらでもなく、実行回数の位置を決められなかった導き方である。
const runCountUndeterminedDerivation = "run count read at the offset that the metrics offset selects in format 30 and 31"

// ItemSemantics は本 package のレコードが持ちうる語彙の項目を返す。
func ItemSemantics() []core.SemanticKey {
	return []core.SemanticKey{core.SemanticKeyEventTime, core.SemanticKeyFilePath}
}

// Fields は 1 つの file を core.RecordField の集合へ直す。最新の実行時刻を event.time とし、
// 実行ファイルの path と参照した file の path を file.path とする。
//
// 2 番目以降の実行時刻に語彙を付けない。レコードの時刻は 1 つであり、最新の実行を指す。
// 2 番目以降の実行時刻は AdditionalRunTimes が返す。
//
// path はボリュームのデバイスの path で始まる。ドライブ文字へ直すのは、同じ端末の registry の
// 値を読む取り込みの実行である (MapVolume)。
func Fields(file File) []core.RecordField {
	fields := []core.RecordField{
		textField(fieldExecutableName, "", file.ExecutableName),
	}
	if executable, unique := executablePathOf(file); executable != "" {
		semantic := core.SemanticKey("")
		if unique {
			semantic = core.SemanticKeyFilePath
		}
		fields = append(fields, textField(fieldExecutablePath, semantic, executable))
	}
	for index, at := range file.LastRuns {
		semantic := core.SemanticKey("")
		if index == 0 {
			semantic = core.SemanticKeyEventTime
		}
		fields = append(fields, fileTimeField(numbered(fieldLastRunTime, index), semantic,
			at, core.MeaningEvent, core.ClockTerminalLocal))
	}
	fields = append(fields,
		runCountField(file),
		textField(fieldPathHash, "", fmt.Sprintf("%08X", file.PathHash)),
		textField(fieldFormatVersion, "", strconv.FormatUint(uint64(file.Version), 10)),
		textField(fieldCompressed, "", strconv.FormatBool(file.Compressed)),
	)
	for index, referenced := range file.ReferencedFiles {
		fields = append(fields, textField(numbered(fieldReferencedFile, index), core.SemanticKeyFilePath, referenced))
	}
	for index, volume := range file.Volumes {
		fields = append(fields,
			textField(numbered(fieldVolumePath, index), "", volume.DevicePath),
			textField(numbered(fieldVolumeSerial, index), "", fmt.Sprintf("%08X", volume.Serial)),
			fileTimeField(numbered(fieldVolumeCreated, index), "",
				volume.Created, core.MeaningProperty, core.ClockFileProperty))
	}
	return fields
}

// derivationVolumeDrive は、path の先頭のボリュームのデバイスの path をドライブ文字へ直した導き方である。
const derivationVolumeDrive = "volume device path replaced with the drive letter of the system root in the registry"

// VolumesUnder は、Fields が組んだ項目のボリュームのデバイスの path のうち、directory の下の
// file を参照したボリュームのデバイスの path を返す。directory は `\WINDOWS\SYSTEM32\` の形で
// 渡し、大文字と小文字を区別せずに比べる。
func VolumesUnder(fields []core.RecordField, directory string) []string {
	var devices []string
	for _, field := range fields {
		if field.Text == nil || field.Name != fieldVolumePath && !strings.HasPrefix(field.Name, fieldVolumePath+"#") {
			continue
		}
		device, readable := field.Text.RawTextValue()
		if !readable || device == "" {
			continue
		}
		prefix := device + directory
		for _, referenced := range fields {
			if referenced.Semantic != core.SemanticKeyFilePath || referenced.Text == nil {
				continue
			}
			if text, ok := referenced.Text.RawTextValue(); ok && len(text) >= len(prefix) && strings.EqualFold(text[:len(prefix)], prefix) {
				devices = append(devices, device)
				break
			}
		}
	}
	return devices
}

// MapVolume は、file.path の項目のうち device のボリュームの下の path に、device を drive
// (`C:` の形) へ替えた正規化値を持たせる。device は大文字と小文字を区別せずに比べる。原資料の
// 文字列は変えない。
func MapVolume(fields []core.RecordField, device, drive string) {
	for index := range fields {
		field := &fields[index]
		if field.Semantic != core.SemanticKeyFilePath || field.Text == nil {
			continue
		}
		text, ok := field.Text.RawTextValue()
		if !ok || len(text) <= len(device) || text[len(device)] != '\\' || !strings.EqualFold(text[:len(device)], device) {
			continue
		}
		value, err := core.NewNormalizedValue(core.ValueStatePresent, text, drive+text[len(device):], derivationVolumeDrive)
		if err == nil {
			field.Text = &value
		}
	}
}

// AdditionalRunTimes は、Fields が組んだ項目のうち、2 番目以降の実行時刻の項目を返す。
// 時刻として書けない値の項目は入らない。
func AdditionalRunTimes(fields []core.RecordField) []core.RecordField {
	var times []core.RecordField
	for _, field := range fields {
		if field.Timestamp != nil && strings.HasPrefix(field.Name, fieldLastRunTime+"#") {
			times = append(times, field)
		}
	}
	return times
}

// LastRunTime は最新の実行時刻を返す。実行時刻の枠がすべて 0 の file と、時刻として
// 書けない値の file では nil である。
func LastRunTime(file File) *core.Timestamp {
	if len(file.LastRuns) == 0 {
		return nil
	}
	timestamp, ok := fileTime(file.LastRuns[0], core.MeaningEvent, core.ClockTerminalLocal)
	if !ok {
		return nil
	}
	return &timestamp
}

// executablePathOf は、参照した path のうち、最後の要素が実行ファイルの名前で始まる path を
// 返す。unique は候補が 1 つだけであるかである。
//
// **欄を使い切った名前だけを前方一致で比べる。** 実行ファイルの名前の欄は 29 文字までを
// 持ち、長い名前は途中で切れる。
//
// 既知の制限: 候補が 2 つ以上のとき、最初の候補を原資料の文字列として残し、語彙を付けない。同じ
// 名前の実行ファイルを別のディレクトリから読んだ file では、どちらが実行ファイルかを
// 名前から決められない, 候補が 2 つ以上になる file の数は測っていない, path の hash から
// 実行ファイルを決める要求が出たとき
func executablePathOf(file File) (executable string, unique bool) {
	name := strings.ToUpper(file.ExecutableName)
	if name == "" {
		return "", false
	}
	// 欄を使い切らない名前は切れておらず、完全に一致する要素だけを候補にする。前方一致に
	// すると、`X.EXE` が読んだ `X.EXE.MUI` も候補になる。
	truncated := len(utf16.Encode([]rune(file.ExecutableName))) >= executableNameUnits
	var candidates []string
	for _, referenced := range file.ReferencedFiles {
		last := strings.ToUpper(path.Base(strings.ReplaceAll(referenced, `\`, "/")))
		if last == name || truncated && strings.HasPrefix(last, name) {
			candidates = append(candidates, referenced)
		}
	}
	if len(candidates) == 0 {
		return "", false
	}
	return candidates[0], len(candidates) == 1
}

// runCountField は実行回数の項目を組む。位置を決められなかった file では、導けなかった値にする。
func runCountField(file File) core.RecordField {
	if !file.RunCountKnown {
		value, _ := core.NewDerivationUndeterminedValue(runCountUndeterminedDerivation)
		field, _ := core.NewTextField(fieldRunCount, "", value)
		return field
	}
	return textField(fieldRunCount, "", strconv.FormatUint(uint64(file.RunCount), 10))
}

// numbered は 2 番目以降の項目の名前に出現の番号を付ける。
func numbered(name string, index int) string {
	if index == 0 {
		return name
	}
	return name + "#" + strconv.Itoa(index+1)
}

// fileTimeToUnixMicroseconds は、FILETIME の基準 (1601-01-01) から UNIX の基準 (1970-01-01)
// までのマイクロ秒である。
const fileTimeToUnixMicroseconds = 11644473600 * 1_000_000

// fileTimeTicksPerMicrosecond は、1 マイクロ秒に相当する FILETIME の 100 ナノ秒の数である。
const fileTimeTicksPerMicrosecond = 10

// fileTimeLayout はマイクロ秒 6 桁の RFC 3339 の layout である。
const fileTimeLayout = "2006-01-02T15:04:05.000000Z07:00"

// fileTime は FILETIME (1601 年からの 100 ナノ秒の数) を時刻にする。原資料の文字列は 10 進の数で
// あり、精度はマイクロ秒へ切り捨てる。ok が偽になるのは、時刻が RFC 3339 で書けない
// 範囲 (西暦 10000 年以降) にあるときである。
func fileTime(value uint64, meaning core.Meaning, clock core.Clock) (core.Timestamp, bool) {
	rawText := strconv.FormatUint(value, 10)
	microseconds := int64(value/fileTimeTicksPerMicrosecond) - fileTimeToUnixMicroseconds // #nosec G115 -- uint64 を 10 で割った値は 2^61 未満であり、int64 に収まる。
	normalized := time.UnixMicro(microseconds).UTC().Format(fileTimeLayout)
	timestamp, err := core.NewTimestamp(core.Timestamp{
		RawText: &rawText, Normalized: &normalized,
		NormalizedForm: core.NormalizedFormRFC3339Absolute, Precision: core.PrecisionMicrosecond,
		OffsetState: core.OffsetStateEpoch, Clock: clock, Meaning: meaning,
		ValueState: core.ValueStatePresent,
	})
	return timestamp, err == nil
}

// fileTimeField は FILETIME の項目を組む。時刻として書けない値は、語彙を外した原資料の文字列の項目にする。
func fileTimeField(name string, semantic core.SemanticKey, value uint64, meaning core.Meaning, clock core.Clock) core.RecordField {
	timestamp, ok := fileTime(value, meaning, clock)
	if !ok {
		return textField(name, "", strconv.FormatUint(value, 10))
	}
	field, _ := core.NewTimestampField(name, semantic, timestamp)
	return field
}

// textField は 1 つの文字列を項目にする。
//
// error を返さない。名前は非空の定数、値は present の原資料の文字列、semantic は語彙の項目か空で
// あるため、検査が失敗しない。
func textField(name string, semantic core.SemanticKey, text string) core.RecordField {
	value, _ := core.NewRawValue(core.ValueStatePresent, text)
	field, _ := core.NewTextField(name, semantic, value)
	return field
}
