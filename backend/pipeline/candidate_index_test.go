// in-package test: 非公開の構築子で作った取り込み結果から索引を組んで検査する。
package pipeline

import (
	"strconv"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 索引の期待値は、下記 2 つの収集元の原文から読んで決める。
//
// squidIndexSource は行番号 1 と 2 の 2 行で、Squid combined は通番を持たない。
// markiiIndexSource は通番 101 から 103 の 3 行で、行番号でも探せる。101 と 102 は
// evt が net、103 は evt が ps のレコードである。
const (
	squidIndexSource = `192.0.2.1 - - [10/Oct/2000:13:55:36 +0000] "GET http://example.test/ HTTP/1.1" 200 12 "-" "test" TCP_MISS:DIRECT` + "\n" +
		`192.0.2.1 - - [10/Oct/2000:13:55:37 +0000] "GET http://198.51.100.42:8080/a HTTP/1.1" 200 12 "-" "test" TCP_MISS:DIRECT` + "\n"
	markiiIndexSource = "02/01/2000 10:00:01.100 +0900 sn=101 evt=net subEvt=con psGUID=p1 tmid=t com=TESTHOST csid=s psPath=app srcIP=192.0.2.10 srcPort=50002 dstIP=198.51.100.42 dstPort=8080\n" +
		"02/01/2000 10:00:02.100 +0900 sn=102 evt=net subEvt=con psGUID=p2 tmid=t com=TESTHOST csid=s psPath=app srcIP=192.0.2.10 srcPort=50003 dstIP=198.51.100.42 dstPort=8080\n" +
		"02/01/2000 10:00:03.100 +0900 sn=103 evt=ps subEvt=start psGUID=p3 tmid=t com=TESTHOST csid=s psPath=app\n"
)

func indexedResult(t *testing.T) ImportResult {
	t.Helper()
	squid := scanIndexSource(t, NewTestSquidParser(), "access.log", SquidFormatKey, squidIndexSource)
	markii := scanIndexSource(t, NewTestMarkIIParser(), "endpoint.log", MarkIIFormatKey, markiiIndexSource)
	sources := []scannedSource{squid, markii}
	statuses := []core.ImportStatus{settleStatus(t, squid, "squid"), settleStatus(t, markii, "markii")}
	result, err := newImportResult(sources, statuses, "run", indexRawTextRef)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func scanIndexSource(
	t *testing.T, parser SourceParser, fileName string, format core.FormatKey, input string,
) scannedSource {
	t.Helper()
	measurement, err := measureWholeSource(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	source, err := scanSource(parser, strings.NewReader(input), measurement,
		SourcePlan{FileName: fileName, FormatKey: format})
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func indexRawTextRef(locator core.RecordLocator, _ string) string {
	if locator.LineNumber == nil {
		return ""
	}
	return "raw:" + locator.SourceFileName + ":" + strconv.FormatInt(*locator.LineNumber, 10)
}

// requireRecordAt は索引が探せるはずの 1 レコードを返す。
func requireRecordAt(
	t *testing.T, index CandidateIndex,
	sourceId string, positionKind core.PositionKind, position int64,
) RecordEntry {
	t.Helper()
	record, found := index.RecordAt(sourceId, positionKind, position)
	if !found {
		t.Fatalf("the record at %s %d of %q is missing", positionKind, position, sourceId)
	}
	return record
}

func TestCandidateIndexRecordAt(t *testing.T) {
	index := NewCandidateIndex(indexedResult(t))
	record := requireRecordAt(t, index, "markii", core.PositionKindSequenceNumber, 103)
	if record.Semantics == nil || record.Semantics.ProcessRef == nil {
		t.Fatalf("the process start record lost its semantics: %+v", record.Semantics)
	}
	if record.Semantics.ProcessRef.SourceId != "markii" ||
		record.Semantics.ProcessRef.ProcessId != "p3" {
		t.Errorf("process reference = %+v", record.Semantics.ProcessRef)
	}
	if _, found := index.RecordAt("markii", core.PositionKindLineNumber, 103); found {
		t.Error("a line number that no record carries reached a record")
	}
	if _, found := index.RecordAt("squid", core.PositionKindLineNumber, 9); found {
		t.Error("an absent position reached a record")
	}
}

// 通番で指した収集元のレコードは行番号でも探せる。
func TestCandidateIndexRecordAtAcceptsBothUnits(t *testing.T) {
	index := NewCandidateIndex(indexedResult(t))
	cases := map[string]struct {
		sourceId     string
		positionKind core.PositionKind
		position     int64
		found        bool
		processId    string
	}{
		"markii 形式を通番で探す": {
			sourceId: "markii", positionKind: core.PositionKindSequenceNumber, position: 101,
			found: true, processId: "p1",
		},
		"markii 形式を行番号で探す": {
			sourceId: "markii", positionKind: core.PositionKindLineNumber, position: 1,
			found: true, processId: "p1",
		},
		"Squid を行番号で探す": {
			sourceId: "squid", positionKind: core.PositionKindLineNumber, position: 2, found: true,
		},
		"Squid を通番で探す": {
			sourceId: "squid", positionKind: core.PositionKindSequenceNumber, position: 2,
		},
		"範囲の外の行番号": {
			sourceId: "markii", positionKind: core.PositionKindLineNumber, position: 4,
		},
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			record, found := index.RecordAt(want.sourceId, want.positionKind, want.position)
			if found != want.found {
				t.Fatalf("RecordAt found = %t, want %t", found, want.found)
			}
			if !found || want.processId == "" {
				return
			}
			if record.Semantics == nil || record.Semantics.ProcessRef == nil {
				t.Fatalf("the record lost its semantics: %+v", record.Semantics)
			}
			if record.Semantics.ProcessRef.ProcessId != want.processId {
				t.Errorf("process id = %q, want %q",
					record.Semantics.ProcessRef.ProcessId, want.processId)
			}
		})
	}
}

// byte 位置で指すレコードは、byte 位置でも先頭の行番号でも探せる。
func TestPositionKeysOfAByteRangeLocator(t *testing.T) {
	offset, length, line := int64(7321), int64(300), int64(137)
	locator := core.RecordLocator{
		SourceId:     "auditd",
		PositionKind: core.PositionKindByteRange,
		ByteOffset:   &offset,
		ByteLength:   &length,
		LineNumber:   &line,
	}
	want := []recordPositionKey{
		{sourceId: "auditd", positionKind: core.PositionKindByteRange, position: offset},
		{sourceId: "auditd", positionKind: core.PositionKindLineNumber, position: line},
	}
	got := positionKeysOf(locator)
	if len(got) != len(want) {
		t.Fatalf("the locator keys as %+v, want %+v", got, want)
	}
	for index, key := range want {
		if got[index] != key {
			t.Errorf("key at %d is %+v, want %+v", index, got[index], key)
		}
	}

	// 先頭の行を数えていないレコードは byte 位置だけで探せる。
	locator.LineNumber = nil
	if got := positionKeysOf(locator); len(got) != 1 ||
		got[0].positionKind != core.PositionKindByteRange {
		t.Errorf("the locator without a line number keys as %+v, want the byte range alone", got)
	}
}

// 先頭の行が同じ 2 つの事象は、行番号の鍵が衝突する。
func TestLocatorPositionKeysSeparateByteRangesSharingTheFirstLine(t *testing.T) {
	line := int64(137)
	locatorAt := func(offset int64) core.RecordLocator {
		length := int64(300)
		return core.RecordLocator{
			PositionKind: core.PositionKindByteRange,
			ByteOffset:   &offset, ByteLength: &length, LineNumber: &line,
		}
	}
	first := locatorPositionKeys(locatorAt(7321))
	second := locatorPositionKeys(locatorAt(7621))
	if first[0] == second[0] {
		t.Errorf("both records take the byte key %q, want different keys", first[0])
	}
	if len(first) != 2 || len(second) != 2 || first[1] != second[1] {
		t.Errorf("the line keys are %v and %v, want the same key so the collision is caught",
			first, second)
	}
}

// 走査した範囲の positionKind は、要求の位置と範囲を比べてよいかを決める材料である。
func TestCandidateIndexSourceScope(t *testing.T) {
	index := NewCandidateIndex(indexedResult(t))
	cases := map[string]struct {
		sourceId     string
		positionKind core.PositionKind
		fromPosition int64
		toPosition   int64
	}{
		"Squid は行番号の範囲":   {"squid", core.PositionKindLineNumber, 1, 2},
		"markii 形式は通番の範囲": {"markii", core.PositionKindSequenceNumber, 101, 103},
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			scope, found := index.SourceScope(want.sourceId)
			if !found {
				t.Fatalf("the scope of %q is missing", want.sourceId)
			}
			if scope.RangeKind != core.RangeKindPositioned {
				t.Fatalf("rangeKind = %q, want %q", scope.RangeKind, core.RangeKindPositioned)
			}
			if scope.PositionKind != want.positionKind {
				t.Errorf("positionKind = %q, want %q", scope.PositionKind, want.positionKind)
			}
			if scope.FromPosition == nil || *scope.FromPosition != want.fromPosition {
				t.Errorf("fromPosition = %v, want %d", scope.FromPosition, want.fromPosition)
			}
			if scope.ToPosition == nil || *scope.ToPosition != want.toPosition {
				t.Errorf("toPosition = %v, want %d", scope.ToPosition, want.toPosition)
			}
		})
	}
	if _, found := index.SourceScope("absent"); found {
		t.Error("an absent source returned a scope")
	}
}

func TestCandidateIndexSkipsWithheldSource(t *testing.T) {
	result := indexedResult(t)
	result.publications[1].status.PublicationState = core.PublicationStateWithheld
	index := NewCandidateIndex(result)
	if _, found := index.RecordAt("markii", core.PositionKindSequenceNumber, 103); found {
		t.Error("a withheld record reached the index")
	}
	if _, found := index.SourceScope("markii"); found {
		t.Error("a withheld source returned a scope")
	}

	// 反対側。公開を止めていない収集元のレコードと範囲は索引に残る。
	if _, found := index.RecordAt("squid", core.PositionKindLineNumber, 2); !found {
		t.Error("a published record is absent from the index")
	}
	if _, found := index.SourceScope("squid"); !found {
		t.Error("a published source returned no scope")
	}
}

func TestCandidateIndexCopiesReturnedRecords(t *testing.T) {
	index := NewCandidateIndex(indexedResult(t))
	squid := requireRecordAt(t, index, "squid", core.PositionKindLineNumber, 2)
	markii := requireRecordAt(t, index, "markii", core.PositionKindSequenceNumber, 101)
	*squid.Semantics.Endpoint.Destination[0].Text.RawText = "changed"
	markii.Semantics.ProcessRef.ProcessId = "changed"

	again := requireRecordAt(t, index, "squid", core.PositionKindLineNumber, 2)
	destination := again.Semantics.Endpoint.Destination[0]
	if value, ok := destination.Text.RawTextValue(); !ok ||
		value != "http://198.51.100.42:8080/a" {
		t.Errorf("the destination original spelling mutated: %q %v", value, ok)
	}
	if value, ok := destination.Text.ComparableValue(); !ok || value != "198.51.100.42" {
		t.Errorf("the destination mutated: %q %v", value, ok)
	}
	if processId := requireRecordAt(t, index, "markii",
		core.PositionKindSequenceNumber, 101).Semantics.ProcessRef.ProcessId; processId != "p1" {
		t.Errorf("process id mutated: %q", processId)
	}
}
