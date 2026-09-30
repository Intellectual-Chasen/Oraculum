package winevent

import (
	"strings"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// Event は Windows イベントログの 1 件である。読み取りの形式 (XML、CSV、EVTX) に依らない。
//
// 値は原資料の文字列を復号した文字列である。XML の実体参照 (`&amp;` など) は復号し、
// それ以外の byte 列は書き換えない。
type Event struct {
	// System は `<System>` の項目である。
	System System
	// EventData は `<EventData>` の `<Data>` 要素を文書の順で持つ。名前の無い `<Data>` は
	// Name が空である。
	EventData []Value
	// Sections は System と EventData の `<Data>` のほかの値である。`<UserData>` の
	// 子要素、`<RenderingInfo>` の要素、System の中の未知の要素がここに入る。
	// Name は要素の path であり、要素を `.`、属性を `@` でつなぐ
	// (`UserData.LogFileCleared.SubjectUserName`)。
	Sections []Value
	// MessageUnrendered は、書き出した端末がイベントの定義を持たず、説明を組めなかったかで
	// ある。真の Event の EventData は欄名の無い値だけを持つ。
	MessageUnrendered bool
	// Source は原文と位置である。
	Source Source
}

// Value は名前と値の組 1 つである。
type Value struct {
	Name string
	Text string
}

// Source は 1 件の原文と、収集元の中の位置である。
type Source struct {
	// RawText は 1 件の原文である。XML では `<Event` から `</Event>` までの byte 列である。
	RawText string
	// ByteOffset は収集元の先頭からの開始位置である。0 起点。
	ByteOffset int64
	// ByteLength は RawText の byte 数である。
	ByteLength int64
	// LineNumber は開始位置の行番号である。1 起点。
	LineNumber int64
	// LineCount は 1 件が占める行数である。
	LineCount int64
}

// System は `<System>` の項目である。nil は要素または属性が無いことを表し、空文字列を
// 指す pointer は値が空であることを表す。
type System struct {
	ProviderName            *string
	ProviderGuid            *string
	ProviderEventSourceName *string
	EventID                 *string
	EventIDQualifiers       *string
	Version                 *string
	Level                   *string
	Task                    *string
	Opcode                  *string
	Keywords                *string
	SystemTime              *string
	EventRecordID           *string
	ActivityID              *string
	RelatedActivityID       *string
	ProcessID               *string
	ThreadID                *string
	Channel                 *string
	Computer                *string
	UserID                  *string
}

// System の項目の名前。応答の fields の名前であり、Event.Sections の path と同じ書き方をする。
const (
	nameProviderName            = "Provider@Name"
	nameProviderGuid            = "Provider@Guid"
	nameProviderEventSourceName = "Provider@EventSourceName"
	nameEventID                 = "EventID"
	nameEventIDQualifiers       = "EventID@Qualifiers"
	nameVersion                 = "Version"
	nameLevel                   = "Level"
	nameTask                    = "Task"
	nameOpcode                  = "Opcode"
	nameKeywords                = "Keywords"
	nameSystemTime              = "TimeCreated@SystemTime"
	nameEventRecordID           = "EventRecordID"
	nameActivityID              = "Correlation@ActivityID"
	nameRelatedActivityID       = "Correlation@RelatedActivityID"
	nameProcessID               = "Execution@ProcessID"
	nameThreadID                = "Execution@ThreadID"
	nameChannel                 = "Channel"
	nameComputer                = "Computer"
	nameUserID                  = "Security@UserID"
)

// systemFieldNames は System の項目の名前と、`@` を `_` にした名前から、項目の名前を求める。
var systemFieldNames = func() map[string]string {
	names := map[string]string{}
	for _, slot := range (&System{}).slots() {
		names[slot.name] = slot.name
		names[strings.Replace(slot.name, "@", "_", 1)] = slot.name
	}
	return names
}()

// FieldNameOf は、イベントの項目を XML の名前で指す文字列を、Observation.Fields の項目の名前へ
// 直す。
//
// name は System の子要素の名前 (`EventID`、`Channel` など)、属性を `_` でつないだ名前
// (`Provider_Name`。Sigma などの検出ルールの書き方である)、または `<EventData>` の `<Data>`
// の Name である。System の項目に該当しない name は `<Data>` の Name として扱う。
func FieldNameOf(name string) string {
	if field, found := systemFieldNames[name]; found {
		return field
	}
	return dataFieldPrefix + name
}

// NamedFields は、形式 format のレコード 1 件の項目 fields が、FieldNameOf が受け取る名前 name
// の項目を同じ名前で持ちうるかを返す関数を返す。
//
// System の項目はどの形式も同じ名前で持つ。XML と EVTX は `<Data>` を Name で持つ。
// イベントビューアーの CSV は、説明の項目名を `<Data>` の Name へ写した項目だけを持ち、写した
// 項目はレコードごとに違う。見出しの文字列が Windows のバージョンにより違い、写す表に無い文字列の項目は `見出し.項目名`
// の名前になるためである。**CSV では、レコードが実際にその名前の項目を持つときだけ真を返す。**
// 説明を組めなかったイベントは名前の無い値だけを持ち、System の項目だけに真を返す。
//
// Sysmon のイベントの説明は、イベントの `<Data>` をすべて `Name: 値` の行で書く。説明が持たない
// 項目はイベントが持たない項目であり、XML と同じくすべての名前に真を返す。偽を返すと、Sysmon の
// バージョンが記録しない項目 (OriginalFileName など) を `1 of` の一方に持つルールが、もう一方の検索で
// 一致できるレコードにも一致しない。`Name: 値` の行を 1 つも持たない Sysmon のレコードは説明を
// 組めていないため、System の項目だけに真を返す。
//
// **同じ名前の項目を 2 つ以上持つレコード (`名前#2` を持つレコード) では、その名前に偽を返す。**
// どれがイベントの項目かを決められないためである。CSV の説明では、利用者が書ける値 (コマンド行
// など) の中の改行の後ろに `名前: 値` を書くと、偽の値が先にその名前を取る。
func NamedFields(format core.FormatKey, fields []core.RecordField) func(name string) bool {
	present := make(map[string]struct{}, len(fields))
	ambiguous := map[string]struct{}{}
	sysmon, unrendered, hasData := false, false, false
	for _, field := range fields {
		switch {
		case field.Name == unnamedDataFieldName:
			// 説明を組めなかったイベントである (descriptionValues)。
			unrendered = true
		case field.Name == nameProviderName && field.Text != nil && field.Text.RawText != nil:
			sysmon = *field.Text.RawText == providerSysmon
		}
		if base, repeated := repeatedFieldBase(field.Name); repeated {
			ambiguous[base] = struct{}{}
		}
		hasData = hasData || strings.HasPrefix(field.Name, dataFieldPrefix)
		present[field.Name] = struct{}{}
	}
	csv := format == FormatKeyViewerCSV
	allData := !csv || (sysmon && hasData && !unrendered)
	return func(name string) bool {
		if _, system := systemFieldNames[name]; system {
			return true
		}
		field := FieldNameOf(name)
		if _, repeated := ambiguous[field]; repeated || (csv && unrendered) {
			return false
		}
		if allData {
			return true
		}
		_, carried := present[field]
		return carried
	}
}

// repeatedFieldBase は、fieldList.uniqueName が 2 回目からの出現に付けた `名前#<番号>` の
// 名前から、番号を除いた名前を返す。
func repeatedFieldBase(name string) (string, bool) {
	base, occurrence, found := strings.Cut(name, "#")
	if !found || occurrence == "" || strings.ContainsFunc(occurrence, func(char rune) bool {
		return char < '0' || char > '9'
	}) {
		return "", false
	}
	return base, true
}

// dataFieldPrefix は `<EventData>` の `<Data>` の項目の名前の前に置く文字列である。
const dataFieldPrefix = "EventData."

// unnamedDataFieldName は Name の無い `<Data>` の項目の名前である (Observe)。
const unnamedDataFieldName = "EventData.Data"

// systemSlot は System の項目 1 つの名前と置き場である。
type systemSlot struct {
	name  string
	value **string
}

// slots は System の項目を、応答の fields に並べる順で返す。読み取りと項目の組み立ての
// 両方が本表を探し、名前と置き場の対応を 1 か所に置く。
func (s *System) slots() []systemSlot {
	return []systemSlot{
		{nameProviderName, &s.ProviderName},
		{nameProviderGuid, &s.ProviderGuid},
		{nameProviderEventSourceName, &s.ProviderEventSourceName},
		{nameEventID, &s.EventID},
		{nameEventIDQualifiers, &s.EventIDQualifiers},
		{nameVersion, &s.Version},
		{nameLevel, &s.Level},
		{nameTask, &s.Task},
		{nameOpcode, &s.Opcode},
		{nameKeywords, &s.Keywords},
		{nameSystemTime, &s.SystemTime},
		{nameEventRecordID, &s.EventRecordID},
		{nameActivityID, &s.ActivityID},
		{nameRelatedActivityID, &s.RelatedActivityID},
		{nameProcessID, &s.ProcessID},
		{nameThreadID, &s.ThreadID},
		{nameChannel, &s.Channel},
		{nameComputer, &s.Computer},
		{nameUserID, &s.UserID},
	}
}
