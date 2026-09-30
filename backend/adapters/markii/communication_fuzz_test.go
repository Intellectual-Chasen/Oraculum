package markii_test

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/markii"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// FuzzParseCommunication は任意の byte 列に対する 6 つの性質を確かめる。
//
//  1. panic しない
//  2. 失敗しなかった観測は、core の型が単独で Validate を通る
//  3. 失敗した観測は行番号を持ち、観測の中身を持たない
//  4. Fields はヘッダーの時刻 1 件の後に、原文の field を並び順のまま 1 件ずつ持つ
//  5. **EventTime の文字列が、同じレコードのヘッダーの文字列と 1 字も違わない**
//  6. 観測の種別の意味の状態は、subEvt の値だけで決まる
//
// 5 は、切断のレコードから接続の開始時刻を推定する経路が入っていないことを文字列の一致で
// 固定する。推定した値を入れると、ヘッダーと違う文字列が EventTime に現れる。
//
// 入力は文字列の分割を通してから渡す。文字列の分割が失敗したレコードは解析へ渡さない。
func FuzzParseCommunication(f *testing.F) {
	seeds := []string{
		communicationConnectLine,
		communicationAcceptLine,
		communicationCloseLine,
		communicationEstablishLine,
		communicationOpenUdpLine,
		communicationWithoutPathLine,
		communicationWithBadSequenceLine,
		communicationWithBadHeaderTimeLine,
		communicationWithRepeatedKeyLine,
		communicationWithUnknownSubEventLine,
		communicationWithoutSubEventLine,
		communicationWithQuotedSourceIpLine,
		processStartLine,
		communicationConnectLine + "\r\n" + communicationOpenUdpLine + "\n",
		"02/01/2000 13:30:00.500 +0900 evt=net subEvt=con",
		"02/01/2000 13:30:00.500 +0900 sn=0 evt=net subEvt=con com=\"\" tmid= csid= psGUID= psPath=",
		"",
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		var reader markii.Reader
		reader.Reset(strings.NewReader(input))
		for {
			record, tokenizeFailure, err := reader.Next()
			if errors.Is(err, io.EOF) {
				return
			}
			if err != nil {
				return
			}
			if tokenizeFailure != nil {
				continue
			}

			observation, failure := markii.ParseCommunication(record)

			if failure != nil {
				// 3. 失敗した観測は位置だけを持つ。
				//
				// 位置は行番号と、読めていれば通番の 2 つである。それ以外の欄が
				// 埋まっていないことを zero value との比較で確かめる。
				// 欄を足したときに検査から漏れないためである。
				positionOnly := markii.Communication{
					SequenceNumber: observation.SequenceNumber,
					LineNumber:     record.LineNumber(),
				}
				if !reflect.DeepEqual(observation, positionOnly) {
					t.Fatalf("a failed observation carries more than its position: %+v", observation)
				}
				if failure.LineNumber == nil || *failure.LineNumber != record.LineNumber() {
					t.Fatal("the failure must carry the line number of the record")
				}
				continue
			}

			// 2. 失敗しなかった観測は、core の型が単独で Validate を通る。
			if err := observation.EventTime.Validate(); err != nil {
				t.Fatalf("the header timestamp of a successful observation is invalid: %v", err)
			}
			if err := observation.ObservationKind.Validate(); err != nil {
				t.Fatalf("the observation kind of a successful observation is invalid: %v", err)
			}
			for name, value := range map[string]core.RawAndNormalized{
				"srcIP":   observation.SourceIp,
				"srcPort": observation.SourcePort,
				"dstIP":   observation.DestIp,
				"dstPort": observation.DestPort,
			} {
				if err := value.Validate(); err != nil {
					t.Fatalf("the %s of a successful observation is invalid: %v", name, err)
				}
			}
			for _, field := range observation.Fields {
				if err := field.Validate(); err != nil {
					t.Fatalf("the field %q of a successful observation is invalid: %v",
						field.Name, err)
				}
			}
			// 成功した観測は必ず通番と識別の値を持つ。
			if observation.SequenceNumber == nil {
				t.Fatal("a successful observation must carry the sequence number")
			}
			if observation.ProcessGuid == "" || observation.TerminalId == "" {
				t.Fatal("a successful observation must carry the identifying values")
			}

			// 5. EventTime の文字列がヘッダーの文字列と一致する。
			dateTime, hasDateTime := record.DateTimeText()
			zone, hasZone := record.ZoneText()
			if !hasDateTime || !hasZone {
				t.Fatal("a successful observation must come from a record with a header")
			}
			raw, ok := observation.EventTime.RawTextValue()
			if !ok || raw != dateTime+" "+zone {
				t.Fatalf("the event time carries %q, want the header text %q",
					raw, dateTime+" "+zone)
			}
			if observation.EventTime.Meaning != core.MeaningEvent {
				t.Fatalf("the event time carries the meaning %q, want %q",
					observation.EventTime.Meaning, core.MeaningEvent)
			}

			// 6. 意味の状態は subEvt の値だけで決まる。
			subEvent, hasSubEvent := record.Field("subEvt")
			if !hasSubEvent {
				t.Fatal("a successful observation must come from a record with a subEvt")
			}
			wantStatus := core.ObservationKindStatusUndetermined
			switch subEvent.Value() {
			case "con", "acpt", "dcon":
				wantStatus = core.ObservationKindStatusDetermined
			case "est", "openUDP":
				wantStatus = core.ObservationKindStatusInferred
			}
			if observation.ObservationKind.Status != wantStatus {
				t.Fatalf("the subEvt %q carries the status %q, want %q",
					subEvent.Value(), observation.ObservationKind.Status, wantStatus)
			}

			// 4. Fields はヘッダーの時刻 1 件の後に、原文の field を並び順のまま持ち、
			// 末尾に当該レコードに出ない 6 つの key を item_absent で持つ。
			recordFields := record.Fields()
			missing := make([]string, 0, len(communicationOptionalKeyNames))
			for _, key := range communicationOptionalKeyNames {
				if record.FieldCount(key) == 0 {
					missing = append(missing, key)
				}
			}
			want := len(recordFields) + 1 + len(missing)
			if len(observation.Fields) != want {
				t.Fatalf("the observation carries %d fields, want %d",
					len(observation.Fields), want)
			}
			for index, field := range recordFields {
				carried := observation.Fields[index+1]
				if carried.Name != field.Key() {
					t.Fatalf("the field at %d is named %q, want %q",
						index+1, carried.Name, field.Key())
				}
				raw, ok := carried.Text.RawTextValue()
				if !ok || raw != field.RawValue() {
					t.Fatalf("the field %q at %d carries %q, want %q",
						field.Key(), index+1, raw, field.RawValue())
				}
			}
			for offset, key := range missing {
				carried := observation.Fields[len(recordFields)+1+offset]
				if carried.Name != key {
					t.Fatalf("the completed field at %d is named %q, want %q",
						len(recordFields)+1+offset, carried.Name, key)
				}
				if carried.Text.ValueState != core.ValueStateItemAbsent {
					t.Fatalf("the completed field %q carries the state %q, want %q",
						key, carried.Text.ValueState, core.ValueStateItemAbsent)
				}
			}
		}
	})
}
