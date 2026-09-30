package pipeline

import "github.com/Intellectual-Chasen/Oraculum/backend/core"

// CandidateIndex は取り込み結果のレコードを、収集元と位置で探す索引である。
//
// **全対象の組合せを走査して関係を作らない。** 索引を使い、関連付けの条件で候補を限定する。
// 走査は NewCandidateIndex がレコード 1 件につき 1 回行い、探す操作は map の 1 回の探索と、
// 探した 1 件の複製だけである。
//
// 探せる値は 2 つである。収集元と位置は `/api/v0/records` のレコードを探し、収集元ごとの
// 走査した範囲は要求の位置が範囲の中にあるかの判定に使う。
//
// 公開を止めた収集元のレコードを索引に入れない。
type CandidateIndex struct {
	records      []RecordEntry
	byPosition   map[recordPositionKey]int
	sourceScopes map[string]core.RecordRange
}

// recordPositionKey は収集元 1 件の中の 1 レコードを指す位置である。
type recordPositionKey struct {
	sourceId     string
	positionKind core.PositionKind
	position     int64
}

// NewCandidateIndex は取り込み結果を 1 回走査して索引を組む。
func NewCandidateIndex(result ImportResult) CandidateIndex {
	index := CandidateIndex{
		byPosition:   make(map[recordPositionKey]int),
		sourceScopes: make(map[string]core.RecordRange),
	}
	for _, publication := range result.publications {
		if publication.status.PublicationState == core.PublicationStateWithheld {
			continue
		}
		index.sourceScopes[publication.status.SourceId] = cloneScope(publication.status.Scope)
		for _, record := range publication.records {
			index.add(record)
		}
	}
	return index
}

func cloneScope(scope core.RecordRange) core.RecordRange {
	scope.FromPosition = clonePointer(scope.FromPosition)
	scope.ToPosition = clonePointer(scope.ToPosition)
	return scope
}

func (index *CandidateIndex) add(record RecordEntry) {
	position := len(index.records)
	// **取り込み結果のレコードと値を共有する。** 取り込み結果は組んだ後に書き換えず、
	// RecordAt が返すときに複製する。
	index.records = append(index.records, record)
	for _, key := range positionKeysOf(record.Locator) {
		// 同じ位置を 2 件のレコードが名乗る取り込み結果は公開されない
		// (withhold.go の checkIdentifierCollision)。先に入れた要素を保つ。
		if _, taken := index.byPosition[key]; !taken {
			index.byPosition[key] = position
		}
	}
}

// positionKeysOf はレコードの位置を索引の鍵へ直す。
//
// **主の位置とは別の指し方を持つレコードは、その指し方でも探せる。** markii 形式のレコードは
// sequenceNumber と lineNumber の両方を持ち、1 レコードが複数行に分かれる形式の
// レコードは byteOffset と先頭の lineNumber の両方を持つ。`/api/v0/records` の要求は
// lineNumber だけでも位置を与えられる。
//
// 位置の値を読めなかったレコードでは要素数 0 を返す。
func positionKeysOf(locator core.RecordLocator) []recordPositionKey {
	if locator.SourceId == "" {
		return nil
	}
	keys := make([]recordPositionKey, 0, 2)
	appendKey := func(kind core.PositionKind, position *int64) {
		if position == nil {
			return
		}
		keys = append(keys, recordPositionKey{
			sourceId: locator.SourceId, positionKind: kind, position: *position,
		})
	}
	appendKey(locator.PositionKind, locatorPosition(&locator))
	if locator.PositionKind != core.PositionKindLineNumber {
		appendKey(core.PositionKindLineNumber, locator.LineNumber)
	}
	return keys
}

// SourceScope は収集元 1 件の走査した範囲を返す。
// ok が偽になるのは、その収集元が公開された取り込み結果に無いときである。
//
// **範囲の positionKind は、要求が与えた位置と範囲を比べてよいかを決める。** 単位が
// 同じ位置だけを範囲と比べ、単位が異なる位置は一致するレコードがあるかだけで判定する。
func (index CandidateIndex) SourceScope(sourceId string) (core.RecordRange, bool) {
	scope, found := index.sourceScopes[sourceId]
	if !found {
		return core.RecordRange{}, false
	}
	return cloneScope(scope), true
}

// RecordAt は収集元と位置で指した 1 レコードを返す。
//
// **通番を持つ収集元のレコードも行番号で探せる。** positionKind に line_number を
// 与えた要求は、markii 形式のレコードを行番号で探す。
//
// ok が偽になるのは、その位置のレコードが公開された取り込み結果に無いときである。
func (index CandidateIndex) RecordAt(
	sourceId string, positionKind core.PositionKind, position int64,
) (RecordEntry, bool) {
	at, found := index.byPosition[recordPositionKey{sourceId, positionKind, position}]
	if !found {
		return RecordEntry{}, false
	}
	return cloneRecords([]RecordEntry{index.records[at]})[0], true
}
