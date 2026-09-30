package winregistry

import (
	"regexp"
	"strconv"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// レコードの項目の名前。値の項目は名前の後ろに値の名前を持つ。
const (
	fieldKeyPath         = "KeyPath"
	fieldLastWrittenTime = "LastWrittenTime"
	fieldCellSegments    = "CellSegments"
	fieldValue           = "Value"
	// fieldValueType と fieldValueCell は `[<値の名前>]` を後ろに持つ。`.` で繋ぐと、値の名前の
	// 末尾で欄を探す検索が、値の項目と一緒に型と位置の項目を拾う。
	fieldValueType            = "ValueType"
	fieldValueCell            = "ValueCell"
	fieldUnreadableValueCount = "UnreadableValueCount"
	fieldAppliedLogEntry      = "AppliedLogEntry"
)

// 見出しの項目の名前。
const (
	headerHiveFileName      = "HiveFileName"
	headerPrimarySequence   = "PrimarySequenceNumber"
	headerSecondarySequence = "SecondarySequenceNumber"
	headerLastWritten       = "LastWrittenTime"
	headerFormatVersion     = "FormatVersion"
	headerRootCellOffset    = "RootCellOffset"
	headerHiveBinsDataSize  = "HiveBinsDataSize"
	headerBaseBlockChecksum = "BaseBlockChecksum"
	headerDirty             = "Dirty"
	headerRecovery          = "Recovery"
	headerAppliedSequence   = "AppliedSequenceRange"
	headerRecoveredBinsSize = "RecoveredHiveBinsDataSize"
	headerLogPrefix         = "Log."
	checksumValidText       = "valid"
	checksumInvalidText     = "invalid"
)

// headerFields は主 file の base block の値と、log の適用の結果を見出しの項目にする。
func headerFields(base baseBlock, h *hive, result recovery, members []Member, logs []*logFile) []core.RecordField {
	checksum := checksumInvalidText
	if base.checksumValid {
		checksum = checksumValidText
	}
	fields := []core.RecordField{
		textField(headerHiveFileName, base.fileName),
		textField(headerPrimarySequence, strconv.FormatUint(uint64(base.primarySequence), 10)),
		textField(headerSecondarySequence, strconv.FormatUint(uint64(base.secondarySequence), 10)),
	}
	// Windows 8.1 以降の書き手はこの欄を更新せず、0 を書くことがある。
	if base.lastWritten != 0 {
		if field, _, ok := fileTimeField(headerLastWritten, base.lastWritten); ok {
			fields = append(fields, field)
		}
	}
	fields = append(fields,
		textField(headerFormatVersion, strconv.FormatUint(uint64(base.majorVersion), 10)+"."+
			strconv.FormatUint(uint64(base.minorVersion), 10)),
		textField(headerRootCellOffset, strconv.FormatUint(uint64(base.rootCellOffset), 10)),
		textField(headerHiveBinsDataSize, strconv.FormatUint(uint64(base.hiveBinsDataSize), 10)),
		textField(headerBaseBlockChecksum, checksum),
		textField(headerDirty, strconv.FormatBool(base.dirty())),
		textField(headerRecovery, result.state),
	)
	if len(h.entries) > 0 {
		fields = append(fields,
			textField(headerAppliedSequence, sequenceRange(h.entries[0].sequence, h.entries[len(h.entries)-1].sequence)),
			textField(headerRecoveredBinsSize, strconv.Itoa(len(h.bins))),
		)
	}
	present := map[string]struct{}{}
	for _, log := range logs {
		present[log.name] = struct{}{}
		fields = append(fields, textField(headerLogPrefix+log.name, log.status))
	}
	if members[0].Name != "" {
		for _, suffix := range LogSuffixes() {
			if _, ok := present[members[0].Name+suffix]; !ok {
				fields = append(fields, textField(headerLogPrefix+members[0].Name+suffix, "absent"))
			}
		}
	}
	return fields
}

// currentVersionKey は、SOFTWARE の hive の中で Windows の導入先 (SystemRoot) を持つ key である。
var currentVersionKey = regexp.MustCompile(`(?i)^\\Microsoft\\Windows NT\\CurrentVersion$`)

// SystemRootOf は、SOFTWARE の hive の CurrentVersion の key のレコードの項目から、SystemRoot の
// 値 (`C:\Windows` の形) を返す。ok が偽になるのは、ほかの key のレコードと、値を持たない
// レコードである。
func SystemRootOf(fields []core.RecordField) (string, bool) {
	if len(fields) == 0 || fields[0].Name != fieldKeyPath || fields[0].Text == nil {
		return "", false
	}
	if path, ok := fields[0].Text.RawTextValue(); !ok || !currentVersionKey.MatchString(path) {
		return "", false
	}
	for _, field := range fields {
		if field.Name == valueFieldName("SystemRoot") && field.Text != nil {
			return field.Text.RawTextValue()
		}
	}
	return "", false
}

// textField は 1 つの文字列を項目にする。
//
// error を返さない。名前は非空、値は present の原資料の文字列であるため、検査が失敗しない。
func textField(name, text string) core.RecordField {
	value, _ := core.NewRawValue(core.ValueStatePresent, text)
	field, _ := core.NewTextField(name, "", value)
	return field
}

// fileTimeToUnixMicroseconds は、FILETIME の基準 (1601-01-01) から UNIX の基準 (1970-01-01)
// までのマイクロ秒である。
const fileTimeToUnixMicroseconds = 11644473600 * 1_000_000

// fileTimeTicksPerMicrosecond は、1 マイクロ秒に相当する FILETIME の 100 ナノ秒の数である。
const fileTimeTicksPerMicrosecond = 10

// fileTimeLayout はマイクロ秒 6 桁の RFC 3339 の layout である。
const fileTimeLayout = "2006-01-02T15:04:05.000000Z07:00"

// fileTimeField は FILETIME (1601 年からの 100 ナノ秒の数、UTC) を key と hive の property の
// 時刻の項目にする。原資料の文字列は 10 進の数で、精度はマイクロ秒へ切り捨てる。ok が偽になるのは、
// 時刻が RFC 3339 で書けない範囲 (西暦 10000 年以降) にあるときである。
func fileTimeField(name string, value uint64) (core.RecordField, core.Timestamp, bool) {
	rawText := strconv.FormatUint(value, 10)
	microseconds := int64(value/fileTimeTicksPerMicrosecond) - fileTimeToUnixMicroseconds // #nosec G115 -- uint64 を 10 で割った値は 2^61 未満であり、int64 に収まる。
	normalized := time.UnixMicro(microseconds).UTC().Format(fileTimeLayout)
	timestamp, err := core.NewTimestamp(core.Timestamp{
		RawText: &rawText, Normalized: &normalized,
		NormalizedForm: core.NormalizedFormRFC3339Absolute, Precision: core.PrecisionMicrosecond,
		OffsetState: core.OffsetStateEpoch, Clock: core.ClockFileProperty, Meaning: core.MeaningProperty,
		ValueState: core.ValueStatePresent,
	})
	if err != nil {
		return core.RecordField{}, core.Timestamp{}, false
	}
	field, _ := core.NewTimestampField(name, "", timestamp)
	return field, timestamp, true
}
