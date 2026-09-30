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

// ORACULUM_MARKII_SOURCE_DIR が指す入力の全レコードを種別に依らない意味付けに掛ける。
// opt-in の検査である。
//
// **件数を期待値に持たない。** 本 test が確かめるのは入力に依らない 4 つである。
//
//  1. 文字列の分割が成功したレコードが意味付けの失敗を返さない
//  2. 成功した観測の EventTime の文字列が、同じレコードのヘッダーの文字列と 1 字も違わない
//  3. 成功した観測の項目と観測の種別が単独で Validate を通る
//  4. 意味の状態が determined と undetermined のいずれかであり、observedEventKinds の表と
//     一致する。表に無い組が入力に出たら失敗させる
//
// 数えた値は t.Log が出し、assert しない。
func TestParseRecordObservationReadsTheConfiguredSource(t *testing.T) {
	logDir := sourceDir(t)
	entries, err := filepath.Glob(filepath.Join(logDir, "*.log"))
	if err != nil {
		t.Fatalf("listing the logs in %s: %v", logDir, err)
	}
	if len(entries) == 0 {
		t.Fatalf("no log file found in %s", logDir)
	}

	expected := map[string]core.ObservationKindStatus{}
	for _, kind := range observedEventKinds {
		expected[kind.event+"/"+kind.subEvent] = kind.status
	}
	observations := 0
	seen := map[string]int{}
	for _, path := range entries {
		readObservationsOfOneFile(t, path, expected, &observations, seen)
	}
	// 1 件も読めていない状態を成功にしない。
	if observations == 0 {
		t.Fatalf("no record read from %d file(s) in %s", len(entries), logDir)
	}
	t.Logf("files=%d observations=%d eventKinds=%d", len(entries), observations, len(seen))
	for kind, count := range seen {
		t.Logf("kind=%s count=%d", kind, count)
	}
}

// readObservationsOfOneFile は 1 file の全レコードを解析し、4 つの性質を確かめる。
func readObservationsOfOneFile(
	t *testing.T,
	path string,
	expected map[string]core.ObservationKindStatus,
	observations *int,
	seen map[string]int,
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
		if tokenizeFailure != nil {
			continue
		}

		observation, failure := markii.ParseRecordObservation(record)
		if failure != nil {
			// 1. 文字列の分割が成功したレコードが意味付けの失敗を返さない。
			t.Errorf("%s line %d: parsing failed at stage %s: expected %s, observed %s",
				path, record.LineNumber(), failure.Stage,
				failure.ExpectedMeaning, failure.ObservedResult)
			continue
		}
		*observations++

		if !checkObservationTimeMatchesHeader(t, path, record, observation) {
			continue
		}
		checkObservationValidates(t, path, record, observation)
		checkObservationStatus(t, path, record, observation, expected, seen)
	}
}

// checkObservationTimeMatchesHeader は EventTime の文字列がヘッダーの文字列と一致するかを
// 確かめる (性質 2)。一致しないレコードで偽を返す。
func checkObservationTimeMatchesHeader(
	t *testing.T, path string, record markii.Record, observation markii.RecordObservation,
) bool {
	t.Helper()
	dateTime, hasDateTime := record.DateTimeText()
	zone, hasZone := record.ZoneText()
	if !hasDateTime || !hasZone {
		t.Errorf("%s line %d: the record must carry its header", path, record.LineNumber())
		return false
	}
	raw, present := observation.EventTime.RawTextValue()
	if !present || raw != dateTime+" "+zone {
		t.Errorf("%s line %d: the event time carries %q, want the header text %q",
			path, record.LineNumber(), raw, dateTime+" "+zone)
		return false
	}
	return true
}

// checkObservationValidates は core の型が単独で Validate を通るかを確かめる (性質 3)。
func checkObservationValidates(
	t *testing.T, path string, record markii.Record, observation markii.RecordObservation,
) {
	t.Helper()
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
}

// checkObservationStatus は意味の状態が observedEventKinds の表と一致するかを確かめ、
// 見た組を数える (性質 4)。
func checkObservationStatus(
	t *testing.T,
	path string,
	record markii.Record,
	observation markii.RecordObservation,
	expected map[string]core.ObservationKindStatus,
	seen map[string]int,
) {
	t.Helper()
	event, _ := record.Field("evt")
	subEvent, _ := record.Field("subEvt")
	kind := event.Value() + "/" + subEvent.Value()
	seen[kind]++
	want, listed := expected[kind]
	if !listed {
		t.Errorf("%s line %d: the kind %s is absent from observedEventKinds",
			path, record.LineNumber(), kind)
		return
	}
	if observation.ObservationKind.Status != want {
		t.Errorf("%s line %d: the kind %s carries the status %q, want %q",
			path, record.LineNumber(), kind, observation.ObservationKind.Status, want)
	}
}
