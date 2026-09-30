package core

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"time"
)

// maxWindowRadiusSeconds は symmetric_seconds の半幅の上限である。
//
// 上限を超える半幅は ns 単位の 64 bit 整数への換算で桁あふれし、時刻の範囲の下端が上端を超える。
// その時刻の範囲は候補を 1 件も取らず、時刻の範囲を広げた要求が no_candidate_in_window を返す。
const maxWindowRadiusSeconds = int64(math.MaxInt64) / int64(time.Second)

// ComparisonUnit は時刻を比べた単位を持つ。
type ComparisonUnit string

// ComparisonUnit の値。
const (
	// ComparisonUnitSecond は秒単位で時刻を比べた状態である。
	ComparisonUnitSecond ComparisonUnit = "second"
	// ComparisonUnitNotCompared は時刻を比べていない状態である。
	ComparisonUnitNotCompared ComparisonUnit = "not_compared"
)

// IsKnown は ComparisonUnit が定義の中の値であるかを返す。
func (c ComparisonUnit) IsKnown() bool {
	switch c {
	case ComparisonUnitSecond, ComparisonUnitNotCompared:
		return true
	default:
		return false
	}
}

// WindowKind は関連付けに用いた時刻の範囲の種別を持つ。
type WindowKind string

// WindowKind の値。
const (
	// WindowKindNotCompared は時刻を比べていない時刻の範囲である。
	WindowKindNotCompared WindowKind = "not_compared"
	// WindowKindSameSecond は中心と同じ秒だけを取る時刻の範囲である。
	WindowKindSameSecond WindowKind = "same_second"
	// WindowKindSecondRange は下端と上端で秒の範囲を与える時刻の範囲である。
	WindowKindSecondRange WindowKind = "second_range"
	// WindowKindSymmetricSeconds は中心から前後に同じ秒数だけ広げた時刻の範囲である。
	WindowKindSymmetricSeconds WindowKind = "symmetric_seconds"
)

// IsKnown は WindowKind が定義の中の値であるかを返す。
func (w WindowKind) IsKnown() bool {
	switch w {
	case WindowKindNotCompared, WindowKindSameSecond, WindowKindSecondRange, WindowKindSymmetricSeconds:
		return true
	default:
		return false
	}
}

// TimeWindow は関連付けに用いた時刻の範囲を持つ。既定値を持たない。
// 時刻の範囲の境界と中心は要求が与えた時刻であり、持つのは RequestedTime である。
type TimeWindow struct {
	// WindowKind は時刻の範囲の種別である。
	WindowKind WindowKind `json:"windowKind"`
	// LowerBound は時刻の範囲の下端である。WindowKind が second_range のとき必須。
	LowerBound *RequestedTime `json:"lowerBound,omitempty"`
	// UpperBound は時刻の範囲の上端である。WindowKind が second_range のとき必須。
	UpperBound *RequestedTime `json:"upperBound,omitempty"`
	// CenterTime は時刻の範囲の中心である。
	// WindowKind が same_second または symmetric_seconds のとき必須。
	CenterTime *RequestedTime `json:"centerTime,omitempty"`
	// RadiusSeconds は時刻の範囲の半幅である。単位は秒。
	// WindowKind が symmetric_seconds のとき必須。
	//
	// 下限は 0、上限は maxWindowRadiusSeconds である。半幅 0 の時刻の範囲は中心の秒だけを取る。
	RadiusSeconds *int64 `json:"radiusSeconds,omitempty"`
}

// Validate は WindowKind と、WindowKind に対応する項目があることを確かめる。
func (w TimeWindow) Validate() error {
	if problem := requireKnownEnum("TimeWindow.windowKind", w.WindowKind); problem != nil {
		return problem
	}
	bounds := []struct {
		item  string
		value *RequestedTime
	}{
		{"TimeWindow.lowerBound", w.LowerBound},
		{"TimeWindow.upperBound", w.UpperBound},
		{"TimeWindow.centerTime", w.CenterTime},
	}
	for _, bound := range bounds {
		if bound.value == nil {
			continue
		}
		if err := bound.value.Validate(); err != nil {
			return itemError(bound.item, err)
		}
	}
	needsBounds := w.WindowKind == WindowKindSecondRange
	needsCenter := w.WindowKind == WindowKindSameSecond || w.WindowKind == WindowKindSymmetricSeconds
	needsRadius := w.WindowKind == WindowKindSymmetricSeconds
	problem := firstProblem(
		requireTimePresence("TimeWindow.lowerBound", w.LowerBound, needsBounds),
		requireTimePresence("TimeWindow.upperBound", w.UpperBound, needsBounds),
		requireTimePresence("TimeWindow.centerTime", w.CenterTime, needsCenter),
	)
	if problem != nil {
		return problem
	}
	if needsRadius {
		if w.RadiusSeconds == nil {
			return itemError("TimeWindow.radiusSeconds", ErrMissingRequiredItem)
		}
		if problem := requireNonNegative("TimeWindow.radiusSeconds", *w.RadiusSeconds); problem != nil {
			return problem
		}
		if *w.RadiusSeconds > maxWindowRadiusSeconds {
			return itemError("TimeWindow.radiusSeconds "+
				strconv.FormatInt(*w.RadiusSeconds, 10)+" exceeds the upper bound "+
				strconv.FormatInt(maxWindowRadiusSeconds, 10), ErrInvalid)
		}
		return nil
	}
	if w.RadiusSeconds != nil {
		return itemError("TimeWindow.radiusSeconds", ErrUnexpectedItem)
	}
	return nil
}

// timeItem は条件付きで必須になる時刻の項目が取る 2 つの型である。
// Timestamp は観測された時刻、RequestedTime は要求が与えた時刻を持つ。
type timeItem interface {
	Timestamp | RequestedTime
}

// requireTimePresence は条件付きで必須になる時刻の項目の有無を確かめる。
func requireTimePresence[T timeItem](item string, value *T, required bool) error {
	if required && value == nil {
		return itemError(item, ErrMissingRequiredItem)
	}
	if !required && value != nil {
		return itemError(item, ErrUnexpectedItem)
	}
	return nil
}

// TimeComparison は 2 つの入力の時刻を比べた結果を持つ。比較の結果を真偽 1 個で表さない。
type TimeComparison struct {
	// ComparisonUnit は比べた単位である。
	ComparisonUnit ComparisonUnit `json:"comparisonUnit"`
	// LeftTime は起点の側の時刻である。ComparisonUnit が second のとき必須。
	LeftTime *Timestamp `json:"leftTime,omitempty"`
	// RightTime は候補の側の時刻である。同じ候補の EventTime と等しい。
	RightTime *Timestamp `json:"rightTime,omitempty"`
	// Assumptions は比較が依拠する前提である。要素数 0 の場合も集合である。
	// ComparisonUnit が not_compared のとき要素数は 0 である。
	Assumptions []MatchAssumption `json:"assumptions"`
}

// Validate は項目の整合を確かめる。
func (c TimeComparison) Validate() error {
	if problem := requireKnownEnum("TimeComparison.comparisonUnit", c.ComparisonUnit); problem != nil {
		return problem
	}
	compared := c.ComparisonUnit == ComparisonUnitSecond
	problem := firstProblem(
		requireTimePresence("TimeComparison.leftTime", c.LeftTime, compared),
		requireTimePresence("TimeComparison.rightTime", c.RightTime, compared),
	)
	if problem != nil {
		return problem
	}
	// validateTimes が返す error は既に項目名を持つため、包み直さない。
	if problem := c.validateTimes(); problem != nil {
		return problem
	}
	// 時刻を比べていない比較には、比較が依拠する前提が無い。
	if !compared && len(c.Assumptions) > 0 {
		return itemError("TimeComparison.assumptions on a not_compared comparison",
			ErrUnexpectedItem)
	}
	return validateAssumptions("TimeComparison.assumptions", c.Assumptions)
}

// validateTimes は比べた 2 つの時刻の整合を確かめる。
//
// rfc3339_absolute 以外の値を絶対時刻として比較しない。2 つの時刻の normalizedForm は
// どちらも rfc3339_absolute である。分析者が UTC からのずれを与えた地方時の値
// (Timestamp.Interpretation) は、そのずれで読んだ時点を比べるため受け入れる。
func (c TimeComparison) validateTimes() error {
	sides := []struct {
		item  string
		value *Timestamp
	}{
		{"TimeComparison.leftTime", c.LeftTime},
		{"TimeComparison.rightTime", c.RightTime},
	}
	for _, side := range sides {
		if side.value == nil {
			continue
		}
		if err := side.value.Validate(); err != nil {
			return itemError(side.item, err)
		}
		if side.value.NormalizedForm != NormalizedFormRFC3339Absolute && side.value.Interpretation == nil {
			return itemError(side.item+" carries the normalizedForm "+
				string(side.value.NormalizedForm)+" while a comparison requires "+
				string(NormalizedFormRFC3339Absolute), ErrInconsistentValue)
		}
	}
	return nil
}

// MarshalJSON は Assumptions を要素数 0 の場合も集合として出す。
func (c TimeComparison) MarshalJSON() ([]byte, error) {
	// items は TimeComparison の method を持たないため、この Marshal は再帰しない。
	type items TimeComparison
	copied := items(c)
	copied.Assumptions = emptyIfNil(copied.Assumptions)
	encoded, err := json.Marshal(copied)
	if err != nil {
		return nil, fmt.Errorf("marshaling TimeComparison: %w", err)
	}
	return encoded, nil
}
