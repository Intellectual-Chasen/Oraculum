package pipeline

import (
	"slices"
	"strconv"
	"time"
)

// RecordFilter は根拠のレコード 1 件を残すか外すかを決める条件である。
// 部分グラフの要求と時系列の要求が同じ条件で絞るため、両方が本型を埋め込む。
type RecordFilter struct {
	// EventCategory と EventAction は根拠のレコードの事象の分類の文字列で絞る。
	// 空の値はその文字列で絞らない。
	//
	// **文字列は完全一致で比べる。** 原資料の文字列と 1 文字でも違う値を与えた要求は 0 件を返す。
	//
	// Windows イベントログのレコードは、プロバイダの名前とイベント ID をこの 2 つの文字列として
	// 比べる (eventKindOf)。**比べるのは文字列だけである。** 事象の分類がプロバイダの名前と同じ
	// 文字列である別の入力形式のレコードは、同じ条件で両方が残る。
	EventCategory string
	EventAction   string
	// EventActionFrom と EventActionTo は事象の動作 (Windows イベントログではイベント ID) を
	// 10 進の数として比べる範囲の両端である。両端を含む。nil はその向きで絞らない。
	//
	// **動作を 10 進の数として読めないレコードは、範囲を与えた要求の結果に入らない。** 範囲の
	// 内か外かを決められない。EventAction の完全一致と重ねたときは、両方を満たすレコードが残る。
	EventActionFrom *uint64
	EventActionTo   *uint64
	// TimeFrom と TimeTo は根拠のレコードの時刻で絞る期間の両端である。
	// nil はその向きで絞らない。
	TimeFrom *time.Time
	TimeTo   *time.Time
	// TimeUnit は期間の判定に用いる比較の単位である。
	//
	// **zero value は秒として扱う。** time.Time.Truncate は 0 を渡されると切り捨てずに
	// 元の時刻を返すため、単位を入れ忘れた要求が nanosecond の精度で境界を比べる。
	// 期間を与える呼び出し元は Validate を通してから用いる。
	TimeUnit time.Duration
	// Case は根拠のレコードの収集元に付けた案件で絞る。空の値は案件で絞らない。
	Case string
	// Terminal は根拠のレコードを置いた端末のノードの識別子で絞る。空の値は端末で絞らない。
	//
	// **比べるのはレコードを置いた端末である (graphRecord.placedTerminalNodeId)。** 名前不明の
	// 端末と利用者が収集元に付けた割当の端末も含む。端末に置かないレコードは、端末を与えた
	// 要求の結果に入らない。**時系列の行の端末は、レコードが名乗った端末のまま変わらない。**
	Terminal string
	// Sources は根拠のレコードを、収集元の sourceId のどれかに一致するものに絞る。空の値は
	// 収集元で絞らない。
	Sources []string
}

// defaultRecordFilterUnit は比較の単位を与えなかった要求が用いる単位である。
const defaultRecordFilterUnit = time.Second

// Validate は比較の単位の zero value を既定の単位へ直す。
func (f *RecordFilter) Validate() {
	if f.TimeUnit <= 0 {
		f.TimeUnit = defaultRecordFilterUnit
	}
}

// filtersRecords は条件が根拠のレコードを絞るかを返す。
func (f RecordFilter) filtersRecords() bool {
	return f.EventCategory != "" || f.EventAction != "" || f.filtersEventActionRange() ||
		f.TimeFrom != nil || f.TimeTo != nil || f.Case != "" || f.Terminal != "" || len(f.Sources) > 0
}

// HasSource は、その sourceId の収集元をグラフが取り込んだかを返す。
func (g Graph) HasSource(sourceId string) bool {
	return slices.ContainsFunc(g.recordings, func(recording sourceRecording) bool {
		return recording.sourceId == sourceId
	})
}

// filtersEventActionRange は条件が動作の範囲を持つかを返す。
func (f RecordFilter) filtersEventActionRange() bool {
	return f.EventActionFrom != nil || f.EventActionTo != nil
}

// instantInPeriod は、時点が期間の内にあるかを TimeUnit の単位で比べて返す。期間を与えない
// 条件では真である。
func (f RecordFilter) instantInPeriod(instant time.Time) bool {
	observed := instant.Truncate(f.TimeUnit)
	if f.TimeFrom != nil && observed.Before(f.TimeFrom.Truncate(f.TimeUnit)) {
		return false
	}
	return f.TimeTo == nil || !observed.After(f.TimeTo.Truncate(f.TimeUnit))
}

// withoutPeriod は期間の両端を外した条件を返す。
func (f RecordFilter) withoutPeriod() RecordFilter {
	f.TimeFrom, f.TimeTo = nil, nil
	return f
}

// eventActionInRange は、動作の文字列が範囲の内にあるかを返す。10 進の数として読めない文字列は
// 範囲の外である。
func (f RecordFilter) eventActionInRange(action string) bool {
	number, err := strconv.ParseUint(action, 10, 64)
	if err != nil {
		return false
	}
	return (f.EventActionFrom == nil || number >= *f.EventActionFrom) &&
		(f.EventActionTo == nil || number <= *f.EventActionTo)
}
