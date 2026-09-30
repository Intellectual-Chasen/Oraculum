package pipeline

import (
	"cmp"
	"slices"
	"strings"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// EventKindCount は事象の分類と動作の組 1 つと、その組を持つレコードの件数である。
type EventKindCount struct {
	// Category と Action は原資料の文字列である。Action が空の組は、動作の欄を持たない
	// レコードの組である。
	Category    string
	Action      string
	RecordCount int64
	// WindowsEvent は、組のレコードがすべて、プロバイダの名前を Category に、イベント ID を
	// Action に持つ Windows イベントログのレコードであるかである (eventKindOf)。
	WindowsEvent bool
}

// eventKindOf はレコードの事象の分類と動作の組を返す。分類が空の文字列の組は、分類を持たない
// レコードである。
//
// event.category の欄を持たないレコードでは、windows_event.provider を分類、windows_event.id を
// 動作にして、windowsEvent に真を返す。Windows イベントログはイベントの意味をプロバイダと
// イベント ID の組で定め、事象の分類の欄を持たない。**欄の語彙の項目は変えない。** 組は
// 事象の種別の選択肢と条件の値にだけ使う。
//
// **欄が在るかで分ける。** event.category の欄を持ち値を比べられないレコードを、プロバイダの
// 組に切り替えない。**イベント ID を比べられないイベントと空の文字列のイベントに組を作らない。**
// 動作が空の組は、動作で絞らない条件として同じプロバイダの全件を指し、組の件数と絞った件数が
// 合わない。
func eventKindOf(fields []core.RecordField) (category, action string, windowsEvent bool) {
	if len(fieldsWithSemantic(fields, core.SemanticKeyEventCategory)) > 0 {
		category, _ := comparableOfSemantic(fields, core.SemanticKeyEventCategory)
		action, _ := comparableOfSemantic(fields, core.SemanticKeyEventAction)
		return category, action, false
	}
	provider, _ := comparableOfSemantic(fields, core.SemanticKeyWindowsEventProvider)
	eventID, _ := comparableOfSemantic(fields, core.SemanticKeyWindowsEventId)
	if provider == "" || eventID == "" {
		return "", "", false
	}
	return provider, eventID, true
}

// EventKinds は、絞り込みを通るレコードが持つ事象の分類と動作の組と、分類を持たない
// レコードの件数を返す。組は件数の多い順に並び、同じ件数の組は分類、動作の文字列の順に並ぶ。
//
// **絞り込みの事象の分類と動作は使わない。** 返す組は、その 2 つの条件に与える値の
// 選択肢である。選んだ分類で動作の選択肢が消えないよう、2 つを外して数える。
func (g Graph) EventKinds(filter RecordFilter) ([]EventKindCount, int64) {
	return g.EventKindsInSource(filter, "")
}

// EventKindsInSource は EventKinds と同じ組を、収集元 sourceId のレコードだけから数える。
// sourceId が空の文字列のときは全収集元のレコードを数える。
//
// **グラフに入るのは公開した収集元の、取り込めたレコードだけである。** 取り込めなかった
// レコードは組にも分類を持たない件数にも入らない。
func (g Graph) EventKindsInSource(filter RecordFilter, sourceId string) ([]EventKindCount, int64) {
	filter.EventCategory, filter.EventAction = "", ""
	filter.Validate()
	counts := make(map[[2]string]*EventKindCount)
	var uncategorized int64
	for at, record := range g.records {
		if sourceId != "" && record.locator.SourceId != sourceId {
			continue
		}
		if !g.recordMatches(at, filter) {
			continue
		}
		if record.eventCategory == "" {
			uncategorized++
			continue
		}
		key := [2]string{record.eventCategory, record.eventAction}
		kind, seen := counts[key]
		if !seen {
			kind = &EventKindCount{Category: key[0], Action: key[1], WindowsEvent: true}
			counts[key] = kind
		}
		kind.RecordCount++
		kind.WindowsEvent = kind.WindowsEvent && record.windowsEventKind
	}
	kinds := make([]EventKindCount, 0, len(counts))
	for _, kind := range counts {
		kinds = append(kinds, *kind)
	}
	slices.SortFunc(kinds, func(left, right EventKindCount) int {
		if order := cmp.Compare(right.RecordCount, left.RecordCount); order != 0 {
			return order
		}
		if order := strings.Compare(left.Category, right.Category); order != 0 {
			return order
		}
		return strings.Compare(left.Action, right.Action)
	})
	return kinds, uncategorized
}
