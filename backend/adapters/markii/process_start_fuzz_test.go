package markii_test

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/markii"
)

// FuzzParseProcessStart は任意の byte 列に対する 4 つの性質を確かめる。
//
//  1. panic しない
//  2. 失敗しなかった観測は、core の型が単独で Validate を通る
//  3. 失敗した観測は行番号を持ち、観測の中身を持たない
//  4. 失敗と成功が同時に立たない
//
// 入力は文字列の分割を通してから渡す。文字列の分割が失敗したレコードは解析へ渡さない。
func FuzzParseProcessStart(f *testing.F) {
	seeds := []string{
		processStartLine,
		processStartWithoutPathLine,
		processStartWithoutParentLine,
		processStartWithBadSequenceLine,
		processStartWithBadHeaderTimeLine,
		processStartWithRepeatedKeyLine,
		processStopLine,
		communicationLine,
		processStartWithoutEventLine,
		processStartWithEmptyGuidLine,
		processStartLine + "\r\n" + processStartWithoutParentLine + "\n",
		"02/01/2000 13:20:00.500 +0900 evt=ps subEvt=start",
		"02/01/2000 13:20:00.500 +0900 sn=0 evt=ps subEvt=start com=\"\" tmid= csid= psGUID= psPath=",
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

			observation, failure := markii.ParseProcessStart(record)

			if failure != nil {
				// 3. 失敗した観測は行番号を持ち、観測の中身を持たない。
				if observation.LineNumber != record.LineNumber() {
					t.Fatalf("the failed observation carries line %d, want %d",
						observation.LineNumber, record.LineNumber())
				}
				if observation.ProcessGuid != "" || observation.TerminalId != "" {
					t.Fatal("a failed observation must not carry the identifying values")
				}
				if len(observation.Fields) != 0 {
					t.Fatal("a failed observation must not carry fields")
				}
				if failure.LineNumber == nil || *failure.LineNumber != record.LineNumber() {
					t.Fatal("the failure must carry the line number of the record")
				}
				continue
			}

			// 2. 失敗しなかった観測は、core の型が単独で Validate を通る。
			if err := observation.StartTime.Validate(); err != nil {
				t.Fatalf("the header timestamp of a successful observation is invalid: %v", err)
			}
			if err := observation.ObservationKind.Validate(); err != nil {
				t.Fatalf("the observation kind of a successful observation is invalid: %v", err)
			}
			if err := observation.ParentGuid.Validate(); err != nil {
				t.Fatalf("the parent reference of a successful observation is invalid: %v", err)
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

			// 5. Fields はヘッダーの時刻 1 件の後に、原文の field を並び順のまま 1 件ずつ
			// 持つ。同じ key が 2 回出るレコードで 1 つ目の値を 2 回持たないことを、
			// 原文と突き合わせて確かめる。
			recordFields := record.Fields()
			if len(observation.Fields) != len(recordFields)+1 {
				t.Fatalf("the observation carries %d fields, want %d",
					len(observation.Fields), len(recordFields)+1)
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
		}
	})
}
