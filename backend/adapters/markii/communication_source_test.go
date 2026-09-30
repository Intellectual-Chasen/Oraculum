package markii_test

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/markii"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// ORACULUM_MARKII_SOURCE_DIR が指す入力の通信のレコードを端まで解析する。opt-in の検査である。
//
// **件数や subEvt の種類数を期待値に持たない。** 本 test が確かめるのは入力に依らない 4 つである。
//
//  1. 通信として選んだレコードが解析の失敗を返さない
//  2. 成功した観測の EventTime の文字列が、同じレコードのヘッダーの文字列と 1 字も違わない
//  3. 成功した観測が持つ core の型が単独で Validate を通る
//  4. 接続の相手の 4 つが present か item_absent のいずれかである
//
// 数えた値は t.Log が出し、assert しない。
func TestParseCommunicationReadsTheConfiguredSource(t *testing.T) {
	logDir := sourceDir(t)
	entries, err := filepath.Glob(filepath.Join(logDir, "*.log"))
	if err != nil {
		t.Fatalf("listing the logs in %s: %v", logDir, err)
	}
	if len(entries) == 0 {
		t.Fatalf("no log file found in %s", logDir)
	}

	communications := 0
	subEvents := map[string]int{}
	withoutEndpoints := 0
	for _, path := range entries {
		readCommunicationsOfOneFile(t, path, &communications, subEvents, &withoutEndpoints)
	}
	// 1 件も読めていない状態を成功にしない。
	if communications == 0 {
		t.Fatalf("no communication record read from %d file(s) in %s", len(entries), logDir)
	}
	t.Logf("files=%d communications=%d subEventKinds=%d withoutEndpoints=%d",
		len(entries), communications, len(subEvents), withoutEndpoints)
	for subEvent, count := range subEvents {
		t.Logf("subEvt=%s count=%d", subEvent, count)
	}
}

// readCommunicationsOfOneFile は 1 file の通信のレコードを解析し、4 つの性質を確かめる。
func readCommunicationsOfOneFile(
	t *testing.T,
	path string,
	communications *int,
	subEvents map[string]int,
	withoutEndpoints *int,
) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening %s: %v", path, err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			t.Errorf("closing %s: %v", path, err)
		}
	}()

	var reader markii.Reader
	reader.Reset(file)
	for {
		record, tokenizeFailure, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			t.Fatalf("%s line %d: reading stopped: %v", path, record.LineNumber(), err)
		}
		if tokenizeFailure != nil || !markii.IsCommunication(record) {
			continue
		}
		*communications++
		if subEvent, found := record.Field("subEvt"); found {
			subEvents[subEvent.Value()]++
		}

		observation, failure := markii.ParseCommunication(record)
		if failure != nil {
			// 1. 通信として選んだレコードが解析の失敗を返さない。
			t.Errorf("%s line %d: parsing failed at stage %s: expected %s, observed %s",
				path, record.LineNumber(), failure.Stage,
				failure.ExpectedMeaning, failure.ObservedResult)
			continue
		}

		// 2. EventTime の文字列がヘッダーの文字列と一致する。
		dateTime, hasDateTime := record.DateTimeText()
		zone, hasZone := record.ZoneText()
		if !hasDateTime || !hasZone {
			t.Errorf("%s line %d: the record must carry its header", path, record.LineNumber())
			continue
		}
		raw, present := observation.EventTime.RawTextValue()
		if !present || raw != dateTime+" "+zone {
			t.Errorf("%s line %d: the event time carries %q, want the header text %q",
				path, record.LineNumber(), raw, dateTime+" "+zone)
			continue
		}

		// 3. core の型が単独で Validate を通る。
		if err := observation.EventTime.Validate(); err != nil {
			t.Errorf("%s line %d: the event time is invalid: %v", path, record.LineNumber(), err)
		}
		if err := observation.ObservationKind.Validate(); err != nil {
			t.Errorf("%s line %d: the observation kind is invalid: %v",
				path, record.LineNumber(), err)
		}
		for _, field := range observation.Fields {
			if err := field.Validate(); err != nil {
				t.Errorf("%s line %d: the field %q is invalid: %v",
					path, record.LineNumber(), field.Name, err)
			}
		}

		// 4. 接続の相手の 4 つが present か item_absent のいずれかである。
		absent := 0
		for name, value := range map[string]core.RawAndNormalized{
			"srcIP":   observation.SourceIp,
			"srcPort": observation.SourcePort,
			"dstIP":   observation.DestIp,
			"dstPort": observation.DestPort,
		} {
			switch value.ValueState {
			case core.ValueStatePresent:
			case core.ValueStateItemAbsent:
				absent++
			default:
				t.Errorf("%s line %d: %s carries the value state %q",
					path, record.LineNumber(), name, value.ValueState)
			}
		}
		if absent == 4 {
			*withoutEndpoints++
		}
	}
}
