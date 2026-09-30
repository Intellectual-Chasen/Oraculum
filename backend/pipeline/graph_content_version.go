package pipeline

import (
	"slices"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// contentVersionStartAt は、ファイルまたはレジストリの値のノード node の、時刻 at の内容の
// バージョンが始まった時刻を返す。replaced が偽のときは、at より後でない置き換えの記録が無く、
// バージョンは node の最初の書き込みから続いている。
//
// **置き換えの記録は、node へ向かう file_operation と registry_operation の根拠のレコードの
// うち、内容を置き換える観測の種別のレコードである** (graphRecord.replacesContent)。記録の
// タイムスタンプが次のバージョンを始める。追記と一部の書き換えの記録はバージョンを区切らない。
// 削除の記録も次のバージョンを始める。削除の後の作成までの間、内容は削除の前の書き込みを
// 含まない。
//
// タイムスタンプが at と等しい置き換えの記録は、at のバージョンを始める。返す時刻は根拠の
// レコードの最も遅いタイムスタンプであり、エッジと根拠の並びに依らない。
//
// 既知の制限: 置き換えと同じタイムスタンプの書き込みを、置き換えが始めたバージョンに含める,
// 記録はタイムスタンプの精度より細かい前後を持たず、前後を測る対象が無い,
// 同じタイムスタンプの記録の前後を示す欄を持つ入力形式を読むとき、その欄で前後を決める
//
// 既知の制限: 収集元の違うレコードのタイムスタンプを、時計の差を補わずに比べる,
// 同じ端末の同じ対象を 2 つの収集元が記録し、時計の差を測れる値が入力に無い, 収集元の時計の差を推定する処理ができたとき、差を補って比べる
func (g Graph) contentVersionStartAt(node int, at time.Time) (start time.Time, replaced bool) {
	return versionStartIn(g.contentReplacements(node), at)
}

// contentReplacements は、ノード node の置き換えの記録のタイムスタンプを昇順に返す
// (contentVersionStartAt)。同じタイムスタンプを重ねて持ちうる。
func (g Graph) contentReplacements(node int) []time.Time {
	records := g.contentReplacementRecords(node)
	instants := make([]time.Time, len(records))
	for index, at := range records {
		instants[index] = g.records[at].instant
	}
	return instants
}

// contentReplacementRecords は、ノード node の置き換えの記録の g.records での位置を、タイムスタンプの
// 昇順 (同じタイムスタンプは位置の昇順) に返す。
func (g Graph) contentReplacementRecords(node int) []int {
	var records []int
	for _, edgeAt := range g.adjacency[node].incoming {
		edge := g.edges[edgeAt]
		if edge.kind != core.EdgeKindFileOperation && edge.kind != core.EdgeKindRegistryOperation {
			continue
		}
		for _, recordAt := range edge.evidence {
			if record := g.records[recordAt]; record.replacesContent && record.hasInstant {
				records = append(records, recordAt)
			}
		}
	}
	slices.SortFunc(records, func(left, right int) int {
		if order := g.records[left].instant.Compare(g.records[right].instant); order != 0 {
			return order
		}
		return left - right
	})
	return records
}

// versionStartIn は、昇順の置き換えのタイムスタンプ replacements のうち、at より後でない最も遅い
// ものを返す。replaced が偽のときは、at より後でない置き換えが無い。
func versionStartIn(replacements []time.Time, at time.Time) (start time.Time, replaced bool) {
	after, _ := slices.BinarySearchFunc(replacements, at, func(instant, target time.Time) int {
		if instant.After(target) {
			return 1
		}
		return -1
	})
	if after == 0 {
		return time.Time{}, false
	}
	return replacements[after-1], true
}

// contentIncludesWrite は、ノード node の、レコード read のタイムスタンプの内容が、レコード
// write が記録した書き込みを含むかを返す。write が read より後のとき、どちらかがタイムスタンプを
// 持たないときは偽である。write と read は node へ向かう操作の根拠のレコードである。
// 読み込みと書き込みの分類は呼び出し元が決める。
//
// write が read の内容のバージョンの始まりより前のときは偽である (contentVersionStartAt)。
// 置き換えの記録そのものは、自分が始めたバージョンの書き込みである。
func (g Graph) contentIncludesWrite(node, write, read int) bool {
	written, readAt := g.records[write], g.records[read]
	if !written.hasInstant || !readAt.hasInstant || written.instant.After(readAt.instant) {
		return false
	}
	start, replaced := g.contentVersionStartAt(node, readAt.instant)
	return !replaced || !written.instant.Before(start)
}
