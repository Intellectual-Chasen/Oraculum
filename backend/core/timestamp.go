package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Precision は時刻の精度を持つ。
type Precision string

// Precision の値。字面は時刻精度の語彙 oc:TimestampPrecisionVocab と揃える。
const (
	// PrecisionYear は年までの精度である。
	PrecisionYear Precision = "year"
	// PrecisionMonth は月までの精度である。
	PrecisionMonth Precision = "month"
	// PrecisionDay は日までの精度である。
	PrecisionDay Precision = "day"
	// PrecisionHour は時までの精度である。
	PrecisionHour Precision = "hour"
	// PrecisionMinute は分までの精度である。
	PrecisionMinute Precision = "minute"
	// PrecisionSecond は秒までの精度である。
	PrecisionSecond Precision = "second"
	// PrecisionMillisecond はミリ秒までの精度である。
	PrecisionMillisecond Precision = "millisecond"
	// PrecisionMicrosecond はマイクロ秒までの精度である。
	PrecisionMicrosecond Precision = "microsecond"
)

// IsKnown は Precision が定義の中の値であるかを返す。
func (p Precision) IsKnown() bool {
	switch p {
	case PrecisionYear, PrecisionMonth, PrecisionDay, PrecisionHour,
		PrecisionMinute, PrecisionSecond, PrecisionMillisecond, PrecisionMicrosecond:
		return true
	default:
		return false
	}
}

// precisionRanks は精度の粗い順の位置である。値が大きいほど細かい。
//
// 文字列の定数どうしは大小を持たないため、2 つの精度を比べる判定はこの表を通す。
var precisionRanks = map[Precision]int{
	PrecisionYear: 0, PrecisionMonth: 1, PrecisionDay: 2, PrecisionHour: 3,
	PrecisionMinute: 4, PrecisionSecond: 5, PrecisionMillisecond: 6, PrecisionMicrosecond: 7,
}

// carriesSubSecond は精度が秒より細かい値を持つかを返す。
// 既知の値のいずれでもない精度は、秒未満を持たない扱いにする。
func (p Precision) carriesSubSecond() bool {
	rank, known := precisionRanks[p]
	if !known {
		return false
	}
	return rank > precisionRanks[PrecisionSecond]
}

// IsCoarserThan は、この精度が other より粗いかを返す。
// 既知の値のいずれでもない精度は、どの既知の精度よりも粗い扱いにする。
func (p Precision) IsCoarserThan(other Precision) bool {
	rank, known := precisionRanks[p]
	otherRank, otherKnown := precisionRanks[other]
	if !known {
		return otherKnown
	}
	if !otherKnown {
		return false
	}
	return rank < otherRank
}

// OffsetState は UTC からのずれの状態を持つ。
type OffsetState string

// OffsetState の値。
const (
	// OffsetStateInValue は UTC からのずれが値に入っている状態である。
	OffsetStateInValue OffsetState = "in_value"
	// OffsetStateItemAbsent は入力形式に UTC からのずれの欄が無い状態である。
	OffsetStateItemAbsent OffsetState = "item_absent"
	// OffsetStateEpoch は、原文字列が UTC の基準からの経過時間で絶対時刻を表す状態である。
	// UTC からのずれの文字列を持たないまま、時点が 1 つに定まる。
	OffsetStateEpoch OffsetState = "epoch"
	// OffsetStateFormatDefined は、入力形式の定義が UTC からのずれを定める状態である。
	// 原文字列はずれの文字列を持たないまま、時点が 1 つに定まる。
	OffsetStateFormatDefined OffsetState = "format_defined"
	// OffsetStateUndetermined は UTC からのずれが未確定の状態である。
	OffsetStateUndetermined OffsetState = "undetermined"
)

// IsKnown は OffsetState が定義の中の値であるかを返す。
func (o OffsetState) IsKnown() bool {
	switch o {
	case OffsetStateInValue, OffsetStateItemAbsent, OffsetStateUndetermined,
		OffsetStateEpoch, OffsetStateFormatDefined:
		return true
	default:
		return false
	}
}

// carriesInstant は、この状態の原文字列と入力形式の定義から時点が 1 つに定まるかを返す。
func (o OffsetState) carriesInstant() bool {
	return o == OffsetStateInValue || o == OffsetStateEpoch || o == OffsetStateFormatDefined
}

// describesSourceOnly は、この状態が原資料の文字列だけに該当し、利用者が与える時刻の文字列に
// 該当しないかを返す。
func (o OffsetState) describesSourceOnly() bool {
	return o == OffsetStateEpoch || o == OffsetStateFormatDefined
}

// Clock はどの時計が時刻を刻んだかを持つ。
//
// **収集元ごとの時計を区別する値である。** 2 つのレコードの時刻を比べる判定は、両方が
// 同じ時計の値であるかを本型で見る (clockDependencyOf)。
type Clock string

// Clock の値。
const (
	// ClockTerminalLocal は、レコードが記録した端末そのものの時計が刻んだ時刻である。
	// Windows の端末の時計と Linux の system clock が該当する。
	ClockTerminalLocal Clock = "terminal_local"
	// ClockObserverLocal は、端末を外から観測して記録したホストの時計が刻んだ時刻である。
	// 通信を中継するサーバの時計と、別のホストで動く収集ツールの時計が該当する。
	ClockObserverLocal Clock = "observer_local"
	// ClockFileProperty はファイルの属性が持つ時刻である。file system が記録した時刻が
	// 該当する。
	ClockFileProperty Clock = "file_property"
	// ClockUndetermined はどの時計が刻んだかを確定できない時刻である。
	ClockUndetermined Clock = "undetermined"
)

// IsKnown は Clock が定義の中の値であるかを返す。
func (c Clock) IsKnown() bool {
	switch c {
	case ClockTerminalLocal, ClockObserverLocal, ClockFileProperty, ClockUndetermined:
		return true
	default:
		return false
	}
}

// Meaning は何の時刻かを持つ。
type Meaning string

// Meaning の値。
const (
	// MeaningEvent は事象が起きた時刻である。
	MeaningEvent Meaning = "event"
	// MeaningOperationStart は操作を始めた時刻である。
	MeaningOperationStart Meaning = "operation_start"
	// MeaningRecordOutput はレコードを出力した時刻である。
	MeaningRecordOutput Meaning = "record_output"
	// MeaningProperty は file の property が持つ時刻である。
	MeaningProperty Meaning = "property"
)

// IsKnown は Meaning が定義の中の値であるかを返す。
func (m Meaning) IsKnown() bool {
	switch m {
	case MeaningEvent, MeaningOperationStart, MeaningRecordOutput, MeaningProperty:
		return true
	default:
		return false
	}
}

// NormalizedForm は正規化値の書式を持つ。
type NormalizedForm string

// NormalizedForm の値。
const (
	// NormalizedFormRFC3339Absolute は RFC 3339 の date-time である。
	NormalizedFormRFC3339Absolute NormalizedForm = "rfc3339_absolute"
	// NormalizedFormLocalWithoutOffset は RFC 3339 の date-time から
	// UTC からのずれを除いた形である。Z と +00:00 を付けない。
	NormalizedFormLocalWithoutOffset NormalizedForm = "local_without_offset"
	// NormalizedFormPartialDateTime は原資料が持つ桁までを書き、
	// 下位の桁を補わない形である。
	NormalizedFormPartialDateTime NormalizedForm = "partial_date_time"
)

// IsKnown は NormalizedForm が定義の中の値であるかを返す。
func (n NormalizedForm) IsKnown() bool {
	switch n {
	case NormalizedFormRFC3339Absolute, NormalizedFormLocalWithoutOffset,
		NormalizedFormPartialDateTime:
		return true
	default:
		return false
	}
}

// 正規化値の文字列を検査する layout。NormalizedForm ごとの形である。
// `local_without_offset` の layout は UTC からのずれを持たないため、`Z` と `+00:00` を
// 付けた文字列を拒否する。
// `partial_date_time` の layout は precision より下位の桁を持たないため、補った桁を拒否する。
const (
	localWithoutOffsetLayout = "2006-01-02T15:04:05"
	partialYearLayout        = "2006"
	partialMonthLayout       = "2006-01"
	partialDayLayout         = "2006-01-02"
	partialHourLayout        = "2006-01-02T15"
	partialMinuteLayout      = "2006-01-02T15:04"
)

// 正規化値の秒の小数部の桁数。Timestamp の Normalized は原資料の精度をそのまま保つ。
const (
	secondFractionDigits      = 0
	millisecondFractionDigits = 3
	microsecondFractionDigits = 6
)

// requiredFractionDigits は precision ごとの、正規化値の秒の小数部の桁数を返す。
// ok が偽になるのは、秒の小数部を持てない precision のときである。
func requiredFractionDigits(precision Precision) (int, bool) {
	switch precision {
	case PrecisionSecond:
		return secondFractionDigits, true
	case PrecisionMillisecond:
		return millisecondFractionDigits, true
	case PrecisionMicrosecond:
		return microsecondFractionDigits, true
	default:
		return 0, false
	}
}

// fractionDigits は正規化値の秒の小数部の桁数を返す。
//
// 小数点が現れるのは秒の後だけである。日付と時刻の区切りは `-` と `:` と `T`、
// UTC からのずれの文字列は `+09:00` と `Z` の形であり、どちらも小数点を持たない。
func fractionDigits(normalized string) int {
	position := strings.IndexByte(normalized, '.')
	if position < 0 {
		return 0
	}
	digits := 0
	for _, character := range normalized[position+1:] {
		if character < '0' || character > '9' {
			break
		}
		digits++
	}
	return digits
}

// sameOptionalString は 2 つの省略可の文字列が、出現と値の双方で同じであるかを返す。
func sameOptionalString(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

// optionalStringValue は省略可の文字列の値を返す。ok が偽になるのは項目が出ていない
// ときである。
func optionalStringValue(value *string) (string, bool) {
	if value == nil {
		return "", false
	}
	return *value, true
}

// truncateToSecond は時刻の秒未満の桁を切り捨てる。切り上げると境界の秒に並ぶレコードが
// 期間の外へ移る。
func truncateToSecond(at time.Time) time.Time {
	return time.Unix(at.Unix(), 0).UTC()
}

// normalizedLayout は normalizedForm と precision から、正規化値の文字列が従う layout を
// 返す。ok が偽になるのは、2 つの組に対応する layout が無いときである。
func normalizedLayout(form NormalizedForm, precision Precision) (string, bool) {
	switch form {
	case NormalizedFormRFC3339Absolute:
		return time.RFC3339, true
	case NormalizedFormLocalWithoutOffset:
		return localWithoutOffsetLayout, true
	case NormalizedFormPartialDateTime:
		return partialDateTimeLayout(precision)
	default:
		return "", false
	}
}

// partialDateTimeLayout は precision ごとの partial_date_time の layout を返す。
func partialDateTimeLayout(precision Precision) (string, bool) {
	switch precision {
	case PrecisionYear:
		return partialYearLayout, true
	case PrecisionMonth:
		return partialMonthLayout, true
	case PrecisionDay:
		return partialDayLayout, true
	case PrecisionHour:
		return partialHourLayout, true
	case PrecisionMinute:
		return partialMinuteLayout, true
	default:
		return "", false
	}
}

// expectedNormalizedForm は Timestamp の offsetState と precision から、
// 成立する NormalizedForm を返す。
//
// **本関数の判定を RequestedTime に適用しない。** RequestedTime の offsetState は
// requestText の状態を表す。
func expectedNormalizedForm(offsetState OffsetState, precision Precision) (NormalizedForm, bool) {
	switch precision {
	case PrecisionYear, PrecisionMonth, PrecisionDay, PrecisionHour, PrecisionMinute:
		return NormalizedFormPartialDateTime, true
	case PrecisionSecond, PrecisionMillisecond, PrecisionMicrosecond:
		// epoch と format_defined は原文字列にずれの文字列を持たないまま時点が定まるため、
		// 正規化値を UTC からのずれを持つ形で書く。
		if offsetState.carriesInstant() {
			return NormalizedFormRFC3339Absolute, true
		}
		return NormalizedFormLocalWithoutOffset, true
	default:
		return "", false
	}
}

// Timestamp は 1 つの時刻を、原資料の文字列と正規化値と精度の組で持つ。
//
// ValueState は derived と derivation_undetermined を取らない。時刻を別の収集元から
// 導く操作を行わないためである。
//
// 値は NewTimestamp または UnmarshalJSON で作る。2 つの経路は Normalized から内部の
// 時刻を導出し、Instant で取り出せる形にする。struct literal で作った値は内部の時刻を
// 持たないため、Normalized が日時として解釈できる場合は Validate が矛盾として拒否する。
// 原資料から読み取る 3 つの文字列を pointer で持つのは、原資料に実在する空文字列を項目の
// 不在の代用にしないためである。項目が出ていない状態を nil が表し、値が空文字列である
// 状態を空文字列を指す pointer が表す。
type Timestamp struct {
	// RawText はレコードの 1 項目の原資料の文字列である。
	// ValueState が item_absent でないとき必須で、item_absent のとき出ない。
	RawText *string `json:"rawText,omitempty"`
	// Normalized は比較に用いる値である。原資料の精度をそのまま保つ。
	// ValueState が item_absent でないとき必須で、item_absent のとき出ない。
	Normalized *string `json:"normalized,omitempty"`
	// NormalizedForm は Normalized の書式である。Normalized があるとき必須。
	NormalizedForm NormalizedForm `json:"normalizedForm,omitempty"`
	// Precision は時刻の精度である。
	Precision Precision `json:"precision"`
	// OffsetState は UTC からのずれの状態である。
	OffsetState OffsetState `json:"offsetState"`
	// OffsetText は UTC からのずれの原文字列である。
	// OffsetState が in_value のとき必須で、他の状態では省略可である。
	OffsetText *string `json:"offsetText,omitempty"`
	// Clock はどの時計が刻んだかである。
	Clock Clock `json:"clock"`
	// Meaning は何の時刻かである。
	Meaning Meaning `json:"meaning"`
	// ValueState は値の状態である。
	ValueState ValueState `json:"valueState"`
	// Interpretation は分析者が所見で与えた UTC からのずれである。原資料と入力形式の定義から
	// 時点が定まらない地方時の文字列 (AcceptsInterpretation) にだけ出る。出るとき、Instant は
	// 正規化値をこのずれで読んだ時点を返す。
	Interpretation *TimestampInterpretation `json:"interpretation,omitempty"`

	// instant は Normalized から導出した時刻である。値の出所を 1 つにするため
	// 非公開にし、hasInstant が真のときだけ意味を持つ。
	instant    time.Time
	hasInstant bool
}

// NewTimestamp は項目の整合を検査し、Normalized から内部の時刻を導出した Timestamp を
// 返す。引数の内部の時刻は捨て、Normalized から作り直す。
func NewTimestamp(items Timestamp) (Timestamp, error) {
	items.instant = time.Time{}
	items.hasInstant = false
	if err := items.validateItems(); err != nil {
		return Timestamp{}, fmt.Errorf("building Timestamp: %w", err)
	}
	if instant, ok := deriveInstant(items); ok {
		items.instant = instant
		items.hasInstant = true
	}
	return items, nil
}

// Instant は関連付けに使う時刻を返す。ok が真になるのは、ValueState が present であり、
// OffsetState から時点が定まって Normalized を RFC 3339 として解釈できるとき、または
// 分析者のずれ (Interpretation) で Normalized の地方時を読めるときである。
func (t Timestamp) Instant() (time.Time, bool) {
	if !t.hasInstant {
		return time.Time{}, false
	}
	derived, ok := deriveInstant(t)
	if !ok || !derived.Equal(t.instant) {
		return time.Time{}, false
	}
	return t.instant, true
}

// LocalClockTime は UTC からのずれを持たない時刻の、壁時計の日時を返す。ok が真になるのは、
// 正規化値が local_without_offset の形で、ValueState が present のときだけである。
//
// **返す値は時点ではない。** time.Time の zone は置き場として UTC を使う。同じ時計が
// 書いた時刻どうしの前後を比べるときだけに使い、Instant の時刻と比べない。
func (t Timestamp) LocalClockTime() (time.Time, bool) {
	if t.NormalizedForm != NormalizedFormLocalWithoutOffset || t.ValueState != ValueStatePresent ||
		t.Normalized == nil {
		return time.Time{}, false
	}
	at, err := time.Parse(localWithoutOffsetLayout, *t.Normalized)
	return at, err == nil
}

// RawTextValue はレコードの 1 項目の原資料の文字列を返す。ok が偽になるのは文字列が
// 出ていないときである。
func (t Timestamp) RawTextValue() (string, bool) {
	return optionalStringValue(t.RawText)
}

// NormalizedValue は比較に用いる値を返す。ok が偽になるのは正規化値が出ていない
// ときである。
func (t Timestamp) NormalizedValue() (string, bool) {
	return optionalStringValue(t.Normalized)
}

// OffsetTextValue は UTC からのずれの原文字列を返す。ok が偽になるのは文字列が出ていない
// ときである。
func (t Timestamp) OffsetTextValue() (string, bool) {
	return optionalStringValue(t.OffsetText)
}

// Equal は全項目と、Instant が返す時刻が同じであるかを返す。
// time.Time は同じ文字列から作っても location の pointer が別になるため、
// Timestamp の比較に `==` を使わない。
func (t Timestamp) Equal(other Timestamp) bool {
	sameItems := sameOptionalString(t.RawText, other.RawText) &&
		sameOptionalString(t.Normalized, other.Normalized) &&
		sameOptionalString(t.OffsetText, other.OffsetText) &&
		t.NormalizedForm == other.NormalizedForm &&
		t.Precision == other.Precision &&
		t.OffsetState == other.OffsetState &&
		t.Clock == other.Clock &&
		t.Meaning == other.Meaning &&
		t.ValueState == other.ValueState &&
		sameInterpretation(t.Interpretation, other.Interpretation)
	if !sameItems {
		return false
	}
	instant, ok := t.Instant()
	otherInstant, otherOk := other.Instant()
	if ok != otherOk {
		return false
	}
	return !ok || instant.Equal(otherInstant)
}

// Validate は項目の整合と、内部の時刻が Normalized と一致していることを確かめる。
func (t Timestamp) Validate() error {
	if problem := t.validateItems(); problem != nil {
		return problem
	}
	derived, ok := deriveInstant(t)
	if ok != t.hasInstant || (ok && !derived.Equal(t.instant)) {
		return itemError("Timestamp.normalized and the internal instant", ErrInconsistentValue)
	}
	return nil
}

// MarshalJSON は全項目を検査してから直列化する。
func (t Timestamp) MarshalJSON() ([]byte, error) {
	if err := t.Validate(); err != nil {
		return nil, fmt.Errorf("marshaling Timestamp: %w", err)
	}
	// items は Timestamp の method を持たないため、この Marshal は再帰しない。
	type items Timestamp
	encoded, err := json.Marshal(items(t))
	if err != nil {
		return nil, fmt.Errorf("marshaling Timestamp: %w", err)
	}
	return encoded, nil
}

// UnmarshalJSON は Timestamp の項目を復元し、Normalized から内部の時刻を作り直す。
// 項目どうしが整合しない場合と、本型が持たない項目が入っている場合は error を返す。
func (t *Timestamp) UnmarshalJSON(data []byte) error {
	type items Timestamp
	var decoded items
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return fmt.Errorf("decoding Timestamp: %w", err)
	}
	rebuilt, err := NewTimestamp(Timestamp(decoded))
	if err != nil {
		return fmt.Errorf("decoding Timestamp: %w", err)
	}
	*t = rebuilt
	return nil
}

// deriveInstant は Normalized から関連付けに使う時刻を導出する。
//
// 時点が 1 つに定まるのは、原文字列に UTC からのずれが入っている状態 (in_value)、
// 原文字列が UTC の基準からの経過時間で絶対時刻を表す状態 (epoch)、入力形式の定義が
// ずれを定める状態 (format_defined) である。どれでもない文字列は RFC 3339 として解釈できないため、既定のずれを埋めずに
// ok を偽で返す。分析者がずれを与えた地方時の文字列だけは、そのずれで読む (interpretedInstant)。
func deriveInstant(t Timestamp) (time.Time, bool) {
	if t.Interpretation != nil {
		return interpretedInstant(t)
	}
	if !t.OffsetState.carriesInstant() || t.ValueState != ValueStatePresent {
		return time.Time{}, false
	}
	normalized, ok := t.NormalizedValue()
	if !ok {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339, normalized)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

// validateItems は内部の時刻を見ずに項目だけを検査する。
func (t Timestamp) validateItems() error {
	if t.ValueState == ValueStateDerived || t.ValueState == ValueStateDerivationUndetermined {
		return itemError("Timestamp.valueState cannot be derived or derivation_undetermined: "+
			"Timestamp carries source timestamps and has no derivation field", ErrInconsistentValue)
	}
	problem := firstProblem(
		requireKnownEnum("Timestamp.precision", t.Precision),
		requireKnownEnum("Timestamp.offsetState", t.OffsetState),
		requireKnownEnum("Timestamp.clock", t.Clock),
		requireKnownEnum("Timestamp.meaning", t.Meaning),
		requireKnownEnum("Timestamp.valueState", t.ValueState),
	)
	if problem != nil {
		return problem
	}
	// 原文字列と正規化値は、欄が原資料に無い状態では出ない。
	if problem := t.validateAbsence(); problem != nil {
		return problem
	}
	// offsetText は offsetState が in_value のとき必須である。
	// 他の状態でも offsetText を持てるため、他の状態での出現を拒否しない。
	if t.OffsetState == OffsetStateInValue && t.OffsetText == nil {
		return itemError("Timestamp.offsetText", ErrMissingRequiredItem)
	}
	if problem := t.validateNormalizedForm(); problem != nil {
		return problem
	}
	return t.validateInterpretation()
}

// sameInterpretation は 2 つの省略可のずれが、出現と値の双方で同じであるかを返す。
func sameInterpretation(left, right *TimestampInterpretation) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

// validateAbsence は rawText と normalized の出現を valueState と突き合わせる。
//
// 2 項目は valueState が item_absent でないとき必須で、item_absent のとき出ない。
// 原資料に実在する空文字列と `-` は値であり、欄の不在の代用にしない。
func (t Timestamp) validateAbsence() error {
	if t.ValueState == ValueStateItemAbsent {
		if t.RawText != nil || t.Normalized != nil {
			return itemError("Timestamp.rawText and Timestamp.normalized on an item_absent value",
				ErrUnexpectedItem)
		}
		return nil
	}
	if t.RawText == nil {
		return itemError("Timestamp.rawText", ErrMissingRequiredItem)
	}
	if t.Normalized == nil {
		return itemError("Timestamp.normalized", ErrMissingRequiredItem)
	}
	return nil
}

// validateNormalizedForm は normalizedForm の出現と値を normalized と突き合わせる。
//
// normalizedForm は normalized があるとき必須である。値は offsetState と precision から
// 一意に決まる。
func (t Timestamp) validateNormalizedForm() error {
	if t.Normalized == nil {
		return requireAbsent("Timestamp.normalizedForm", string(t.NormalizedForm))
	}
	if problem := requireKnownEnum("Timestamp.normalizedForm", t.NormalizedForm); problem != nil {
		return problem
	}
	expected, ok := expectedNormalizedForm(t.OffsetState, t.Precision)
	if !ok {
		return itemError("Timestamp.normalizedForm for precision "+string(t.Precision),
			ErrInconsistentValue)
	}
	if t.NormalizedForm != expected {
		return itemError("Timestamp.normalizedForm is "+string(t.NormalizedForm)+
			" while the offsetState and precision require "+string(expected),
			ErrInconsistentValue)
	}
	return t.validateNormalizedText()
}

// validateNormalizedText は normalized の文字列が normalizedForm の書式に合い、
// 秒の小数部の桁数が precision と揃っていることを確かめる。
//
// local_without_offset の値に Z と +00:00 を付けず、partial_date_time の値に存在しない
// 下位の桁を補わない。
// time.Parse は layout が秒の小数部を持たなくても入力の小数部を受け取るため、
// 桁数は layout と別に確かめる。
func (t Timestamp) validateNormalizedText() error {
	normalized, ok := t.NormalizedValue()
	if !ok {
		return itemError("Timestamp.normalized", ErrMissingRequiredItem)
	}
	layout, hasLayout := normalizedLayout(t.NormalizedForm, t.Precision)
	if !hasLayout {
		return itemError("Timestamp.normalized has no layout for the form "+
			string(t.NormalizedForm)+" and the precision "+string(t.Precision),
			ErrInconsistentValue)
	}
	if _, err := time.Parse(layout, normalized); err != nil {
		return itemError("Timestamp.normalized does not follow the format of "+
			string(t.NormalizedForm), ErrInconsistentValue)
	}
	wanted, hasFraction := requiredFractionDigits(t.Precision)
	if !hasFraction {
		return nil
	}
	if got := fractionDigits(normalized); got != wanted {
		return itemError("Timestamp.normalized carries a second fraction that the precision "+
			string(t.Precision)+" does not have", ErrInconsistentValue)
	}
	return nil
}

// RequestedTime は要求が与えた時刻を持つ。観測された時刻は Timestamp が持つ。
//
// rawText を持たないのは、時刻の範囲の下端・上端・中心が利用者の与える値で、原資料に対応する文字列が
// 存在しないためである。clock と meaning を持たないのは、与えた時刻を刻んだ時計が無く、
// 原資料のどの時刻に該当するかも決まらないためである。
type RequestedTime struct {
	RequestText    string         `json:"requestText"`
	Precision      Precision      `json:"precision"`
	OffsetState    OffsetState    `json:"offsetState"`
	Normalized     string         `json:"normalized,omitempty"`
	NormalizedForm NormalizedForm `json:"normalizedForm,omitempty"`
	Derivation     string         `json:"derivation,omitempty"`
}

// Validate は項目の整合を確かめる。
//
// normalizedForm の値に expectedNormalizedForm の判定を適用しない。同判定は Timestamp の
// offsetState に対する条件である。RequestedTime の offsetState は requestText の状態を表し、
// normalized に補った桁とずれの出どころは derivation が表す。
func (r RequestedTime) Validate() error {
	problem := firstProblem(
		requirePresent("RequestedTime.requestText", r.RequestText),
		requireKnownEnum("RequestedTime.precision", r.Precision),
		requireKnownEnum("RequestedTime.offsetState", r.OffsetState),
	)
	if problem != nil {
		return problem
	}
	if r.OffsetState.describesSourceOnly() {
		return itemError("RequestedTime.offsetState cannot be "+string(r.OffsetState)+": "+
			"the state describes a source timestamp, not a requested one", ErrInconsistentValue)
	}
	if r.Normalized == "" {
		// 補った桁とずれの出どころを書かずに正規化値を出さない形の裏返しである。
		return firstProblem(
			requireAbsent("RequestedTime.normalizedForm", string(r.NormalizedForm)),
			requireAbsent("RequestedTime.derivation", r.Derivation),
		)
	}
	problem = firstProblem(
		requireKnownEnum("RequestedTime.normalizedForm", r.NormalizedForm),
		requirePresent("RequestedTime.derivation", r.Derivation),
	)
	if problem != nil {
		return problem
	}
	return r.validateNormalizedText()
}

// validateNormalizedText は normalized の文字列が、宣言した normalizedForm の書式に従うことを
// 確かめる。
//
// 書式は normalizedForm ごとの layout が定める。expectedNormalizedForm の判定は Timestamp の
// offsetState に対する条件であり、RequestedTime に適用しない。秒の小数部の桁数も確かめない。
// precision が表すのは requestText の精度であり、normalized は requestText に桁を補った
// 値である。
func (r RequestedTime) validateNormalizedText() error {
	layout, ok := normalizedLayout(r.NormalizedForm, r.Precision)
	if !ok {
		return itemError("RequestedTime.normalized has no layout for the form "+
			string(r.NormalizedForm)+" and the precision "+string(r.Precision),
			ErrInconsistentValue)
	}
	if _, err := time.Parse(layout, r.Normalized); err != nil {
		return itemError("RequestedTime.normalized does not follow the format of "+
			string(r.NormalizedForm), ErrInconsistentValue)
	}
	return nil
}

// UnmarshalJSON は RequestedTime を復元する。本型が持たない項目が入っている場合と、
// 項目どうしが整合しない場合は error を返す。
func (r *RequestedTime) UnmarshalJSON(data []byte) error {
	type items RequestedTime
	var decoded items
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return fmt.Errorf("decoding RequestedTime: %w", err)
	}
	value := RequestedTime(decoded)
	if err := value.Validate(); err != nil {
		return fmt.Errorf("decoding RequestedTime: %w", err)
	}
	*r = value
	return nil
}
