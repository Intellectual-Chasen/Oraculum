// in-package test: 要求が与えた位置を、位置の指し方ごとに読めることを確かめる。
package api

import (
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 要求が与えた位置は、core.PositionKind のすべての指し方で読める。
//
// **要求を通す条件と、位置を読む表を突き合わせる。** 片方だけが指し方を知っていると、
// 必須条件を満たした要求が位置を 1 つも持たないまま record_not_found になる。
func TestRequestedPositionsCoverEveryPositionKind(t *testing.T) {
	const value = int64(42)
	for _, testCase := range []struct {
		kind     core.PositionKind
		position requestedPosition
	}{
		{core.PositionKindSequenceNumber, requestedPosition{sequenceNumber: positionValue(value)}},
		{core.PositionKindLineNumber, requestedPosition{lineNumber: positionValue(value)}},
		{core.PositionKindByteRange, requestedPosition{byteOffset: positionValue(value)}},
	} {
		t.Run(string(testCase.kind), func(t *testing.T) {
			positions := requestedPositionsOf(testCase.position)
			if len(positions) != 1 {
				t.Fatalf("the request reaches %d positions, want 1", len(positions))
			}
			if positions[0].kind != testCase.kind {
				t.Errorf("the position is %q, want %q", positions[0].kind, testCase.kind)
			}
			if positions[0].value != value {
				t.Errorf("the position value is %d, want %d", positions[0].value, value)
			}
		})
	}
}

// 取り込みに失敗したレコードの位置を、すべての指し方で探せる。
func TestFailurePositionCoversEveryPositionKind(t *testing.T) {
	const at = int64(7)
	failure := core.ImportFailure{
		LineNumber: positionValue(at),
		RecordRef: &core.RecordLocator{
			SequenceNumber: positionValue(at),
			ByteOffset:     positionValue(at),
		},
	}
	for _, kind := range []core.PositionKind{
		core.PositionKindSequenceNumber,
		core.PositionKindLineNumber,
		core.PositionKindByteRange,
	} {
		if got := failurePositionOf(failure, kind); got != at {
			t.Errorf("the failure at %q is %d, want %d", kind, got, at)
		}
	}
	// 位置を確定できていない失敗は、要求の位置と一致しない値を返す。
	empty := core.ImportFailure{}
	for _, kind := range []core.PositionKind{
		core.PositionKindSequenceNumber,
		core.PositionKindLineNumber,
		core.PositionKindByteRange,
	} {
		if got := failurePositionOf(empty, kind); got >= 0 {
			t.Errorf("a failure without a position returns %d at %q, want a negative value",
				got, kind)
		}
	}
}

func positionValue(value int64) *int64 { return &value }
