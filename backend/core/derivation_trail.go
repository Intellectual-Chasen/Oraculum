package core

import (
	"strconv"
	"strings"
)

// unlocatedStopOutput は、関連付けの無い組の止まった段階の出力である。
const unlocatedStopOutput = "選んだ条件では、起点のレコードからこのレコードへの関連付けが無い。" +
	"この組がどの段階で外れたかを、グラフから特定できない"

// DerivationTrailOfMatch は、関連付けの結果 1 件が起点のレコードから候補のレコードへ至った経路を組む。
//
// **段階ごとに 1 つの段階を置く。** 段階の名前は StageKey の値であり、段階の並び (clock_independent、
// second_time_matched) と文字列の昇順が一致する。時計に依存しない段階は起点のレコードを入力に取り、
// 時刻以外の条件の値を用いる。時刻を比べる段階は、時刻の範囲と起点と候補の時刻を用いる。最後の段階の入力に
// 候補のレコードを置く。段階の出力はその段階が挙げた候補の件数である。
//
// 用いた識別子と段階の出力は、分析者が画面で読む文であるため日本語で書く。
func DerivationTrailOfMatch(match EdgeMatch) (DerivationTrail, error) {
	steps := make([]TrailStep, 0, len(match.StageTallies))
	for index, tally := range match.StageTallies {
		step := TrailStep{
			StepKey: string(tally.StageKey),
			Output:  "この段階が挙げた候補 " + strconv.FormatInt(tally.MemberCount, 10) + " 件",
		}
		if index == 0 {
			step.InputRefs = append(step.InputRefs, recordInput(match.OriginRef))
		}
		if index == len(match.StageTallies)-1 {
			step.InputRefs = append(step.InputRefs, recordInput(match.CandidateRef))
		}
		if tally.StageKey == StageKeySecondTimeMatched {
			step.UsedIdentifiers = []string{timeComparisonText(match.TimeWindow, match.TimeComparison)}
		} else {
			step.UsedIdentifiers = clockIndependentConditionTexts(match.Conditions)
		}
		steps = append(steps, step)
	}
	trail := DerivationTrail{OriginRef: match.OriginRef, Steps: steps}
	if err := trail.Validate(); err != nil {
		return DerivationTrail{}, err
	}
	return trail, nil
}

// UnmatchedDerivationTrail は、起点のレコードと開いたレコードの間に関連付けが無いことを、
// 選択の最後の段階 stopped で止まった経路として組む。
//
// **止まった段階を、組が外れた段階と読ませない。** グラフは挙がった候補だけを持ち、候補に挙がらな
// かった組がどの段階で外れたかを持たない。止まった段階の出力にそのことを書く。
//
// conditions は要求が選んだ条件、timeTolerance は秒単位の時刻の一致に認めた幅である。時刻を
// 比べない選択では nil である。
func UnmatchedDerivationTrail(
	origin RecordLocator, stopped StageKey, conditions []ConditionKey, timeTolerance *int64,
) (DerivationTrail, error) {
	used := make([]string, 0, len(conditions))
	for _, key := range conditions {
		text := "選んだ条件: " + conditionDisplayName(key)
		if key == ConditionKeySecondOfTime && timeTolerance != nil {
			text += " (" + toleranceWindowText(*timeTolerance) + ")"
		}
		used = append(used, text)
	}
	step := TrailStep{
		StepKey:         string(stopped),
		InputRefs:       []TrailInputRef{recordInput(origin)},
		UsedIdentifiers: used,
		Output:          unlocatedStopOutput,
	}
	trail := DerivationTrail{OriginRef: origin, Steps: []TrailStep{step}, StoppedAt: &step}
	if err := trail.Validate(); err != nil {
		return DerivationTrail{}, err
	}
	return trail, nil
}

func recordInput(locator RecordLocator) TrailInputRef {
	return TrailInputRef{Kind: TrailInputKindRecord, Record: &locator}
}

// clockIndependentConditionTexts は、段階が用いた時刻以外の条件を、条件の呼び名と両側の値の文にする。
func clockIndependentConditionTexts(conditions []MatchCondition) []string {
	texts := make([]string, 0, len(conditions))
	for _, condition := range conditions {
		if condition.Use != ConditionUseUsed || condition.ConditionKey.isTimeCondition() {
			continue
		}
		text := conditionDisplayName(condition.ConditionKey) + ": 起点 " + fieldsText(condition.LeftValue)
		if len(condition.RightValue) > 0 {
			text += "、候補 " + fieldsText(condition.RightValue)
		}
		texts = append(texts, text)
	}
	if len(texts) == 0 {
		texts = append(texts, "時刻以外の条件を用いていない")
	}
	return texts
}

// timeComparisonText は、時刻の範囲と起点と候補の時刻の比較を、原資料の文字列で文にする。
func timeComparisonText(window TimeWindow, comparison TimeComparison) string {
	if comparison.ComparisonUnit != ComparisonUnitSecond ||
		comparison.LeftTime == nil || comparison.RightTime == nil {
		return "時刻を比べていない"
	}
	return conditionDisplayName(ConditionKeySecondOfTime) + " (" + timeWindowText(window) + "): 起点 " +
		timestampText(*comparison.LeftTime) + "、候補 " + timestampText(*comparison.RightTime)
}

// timeWindowText は関連付けに用いた時刻の範囲を文にする。
func timeWindowText(window TimeWindow) string {
	switch window.WindowKind {
	case WindowKindSameSecond:
		return toleranceWindowText(0)
	case WindowKindSymmetricSeconds:
		if window.RadiusSeconds != nil {
			return toleranceWindowText(*window.RadiusSeconds)
		}
	case WindowKindSecondRange:
		if window.LowerBound != nil && window.UpperBound != nil {
			return window.LowerBound.RequestText + " から " + window.UpperBound.RequestText + " までの範囲"
		}
	}
	return "範囲の幅を読めない"
}

// toleranceWindowText は、秒単位の時刻の一致に認めた幅を時刻の範囲の文にする。
func toleranceWindowText(tolerance int64) string {
	if tolerance == 0 {
		return "同じ秒の範囲"
	}
	return "前後 " + strconv.FormatInt(tolerance, 10) + " 秒の範囲"
}

func fieldsText(fields []RecordField) string {
	texts := make([]string, 0, len(fields))
	for _, field := range fields {
		if field.Timestamp != nil {
			texts = append(texts, timestampText(*field.Timestamp))
			continue
		}
		if value, ok := comparableValue(field); ok {
			texts = append(texts, value)
			continue
		}
		texts = append(texts, "値が無い")
	}
	return strings.Join(texts, ", ")
}

func timestampText(timestamp Timestamp) string {
	if raw, ok := timestamp.RawTextValue(); ok {
		return raw
	}
	if normalized, ok := timestamp.NormalizedValue(); ok {
		return normalized
	}
	return "値が無い"
}
