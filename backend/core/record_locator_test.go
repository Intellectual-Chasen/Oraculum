package core_test

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 収集元を指すレコード位置は sourceId と sourceContentSha256 を揃えて持つ。
func TestRecordLocatorCarriesBothWaysOfPointingAtTheSource(t *testing.T) {
	locator := markIILocator(secondCandidateSn)
	if err := locator.Validate(); err != nil {
		t.Fatalf("validating the locator: %v", err)
	}
	if locator.SourceId == "" || locator.SourceContentSha256 == "" {
		t.Error("a locator must carry both the sourceId and the sourceContentSha256")
	}

	withoutSourceId := locator
	withoutSourceId.SourceId = ""
	if err := withoutSourceId.Validate(); !errors.Is(err, core.ErrMissingRequiredItem) {
		t.Errorf("error = %v, want one wrapping %v", err, core.ErrMissingRequiredItem)
	}

	withoutSha256 := locator
	withoutSha256.SourceContentSha256 = ""
	if err := withoutSha256.Validate(); !errors.Is(err, core.ErrInvalid) {
		t.Errorf("error = %v, want one wrapping %v", err, core.ErrInvalid)
	}
}

func TestRecordLocatorRequiresThePositionOfItsKind(t *testing.T) {
	t.Run("a line number kind without a line number", func(t *testing.T) {
		locator := squidLocator(squidLineNumber)
		locator.LineNumber = nil
		if err := locator.Validate(); !errors.Is(err, core.ErrMissingRequiredItem) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrMissingRequiredItem)
		}
	})

	t.Run("a sequence number kind without a sequence number", func(t *testing.T) {
		locator := markIILocator(secondCandidateSn)
		locator.SequenceNumber = nil
		if err := locator.Validate(); !errors.Is(err, core.ErrMissingRequiredItem) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrMissingRequiredItem)
		}
	})

	t.Run("a line number below the first line", func(t *testing.T) {
		locator := squidLocator(0)
		if err := locator.Validate(); !errors.Is(err, core.ErrInvalid) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrInvalid)
		}
	})

	t.Run("a byte range kind carrying its offset and length", func(t *testing.T) {
		locator := byteRangeLocator(byteRangeOffset, byteRangeLength)
		if err := locator.Validate(); err != nil {
			t.Fatalf("validating the locator: %v", err)
		}
	})

	t.Run("a byte range kind without a length", func(t *testing.T) {
		locator := byteRangeLocator(byteRangeOffset, byteRangeLength)
		locator.ByteLength = nil
		if err := locator.Validate(); !errors.Is(err, core.ErrMissingRequiredItem) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrMissingRequiredItem)
		}
	})

	t.Run("a byte range kind without an offset", func(t *testing.T) {
		locator := byteRangeLocator(byteRangeOffset, byteRangeLength)
		locator.ByteOffset = nil
		if err := locator.Validate(); !errors.Is(err, core.ErrMissingRequiredItem) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrMissingRequiredItem)
		}
	})

	t.Run("a byte range of no bytes", func(t *testing.T) {
		locator := byteRangeLocator(byteRangeOffset, 0)
		if err := locator.Validate(); !errors.Is(err, core.ErrInvalid) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrInvalid)
		}
	})

	t.Run("a byte range starting at the source head", func(t *testing.T) {
		locator := byteRangeLocator(0, byteRangeLength)
		if err := locator.Validate(); err != nil {
			t.Fatalf("validating the locator: %v", err)
		}
	})

	t.Run("a byte range spanning no lines", func(t *testing.T) {
		locator := byteRangeLocator(byteRangeOffset, byteRangeLength)
		noLines := int64(0)
		locator.LineCount = &noLines
		if err := locator.Validate(); !errors.Is(err, core.ErrInvalid) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrInvalid)
		}
	})

	t.Run("a sequence number kind holding a line number as well", func(t *testing.T) {
		locator := markIILocator(secondCandidateSn)
		lineNumber := int64(520)
		locator.LineNumber = &lineNumber
		if err := locator.Validate(); err != nil {
			t.Fatalf("validating the locator: %v", err)
		}
	})
}

// 位置を持たない項目は json から消える。
func TestRecordLocatorJsonOmitsTheAbsentPosition(t *testing.T) {
	encoded, err := json.Marshal(squidLocator(squidLineNumber))
	if err != nil {
		t.Fatalf("marshaling the locator: %v", err)
	}
	// 期待値は組み立てずに literal で置く。fixture の定数から作ると、定数の書き換えが
	// 期待値にも伝わり、値が変わったことを検出できない。
	want := `{"sourceId":"source-squid",` +
		`"sourceContentSha256":"bb22cc33dd44ee55ff6600778899aa11bb22cc33dd44ee55ff6600778899aa11",` +
		`"sourceFileName":"proxy-request.log","positionKind":"line_number","lineNumber":250,` +
		`"recordRawTextRef":"/api/v0/records"}`
	if string(encoded) != want {
		t.Errorf("json = %s, want %s", encoded, want)
	}
}

func TestRecordRangeRejectsAReversedRange(t *testing.T) {
	reversed := markIIRange(rangeToSequenceNumber, rangeFromSequenceNumber)
	if err := reversed.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
		t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
	}
	single := markIIRange(rangeFromSequenceNumber, rangeFromSequenceNumber)
	if err := single.Validate(); err != nil {
		t.Fatalf("validating a range of one record: %v", err)
	}
}

func TestProcessRefPairsTheProcessIdWithTheTerminal(t *testing.T) {
	processRef := core.ProcessRef{
		SourceId:            markIISourceId,
		SourceContentSha256: markIISha256,
		ProcessId:           markIIProcessId,
		TerminalId:          markIITerminalId,
	}
	if err := processRef.Validate(); err != nil {
		t.Fatalf("validating the process reference: %v", err)
	}
	processRef.TerminalId = ""
	if err := processRef.Validate(); !errors.Is(err, core.ErrMissingRequiredItem) {
		t.Errorf("error = %v, want one wrapping %v", err, core.ErrMissingRequiredItem)
	}
}

// DerivationTrail は 1 段階以上を順序として持ち、進めなかった段階はその中の 1 段階を指す。
func TestDerivationTrailHoldsItsStepsInOrder(t *testing.T) {
	steps := []core.TrailStep{
		{
			StepKey: "C1",
			InputRefs: []core.TrailInputRef{
				{
					Kind:   core.TrailInputKindRecord,
					Record: pointerToLocator(markIILocator(ancestorCandidateSn)),
				},
			},
			UsedIdentifiers: []string{"psGUID"},
			Output:          "候補のプロセス 1 個",
		},
		{
			StepKey:         "C5",
			UsedIdentifiers: []string{"parentGUID"},
			Output:          "同じ psGUID の ps / start が 0 件",
		},
	}
	trail := core.DerivationTrail{
		OriginRef: squidLocator(squidLineNumber),
		Steps:     steps,
		StoppedAt: &steps[1],
	}
	if err := trail.Validate(); err != nil {
		t.Fatalf("validating the trail: %v", err)
	}
	if trail.Steps[0].StepKey != "C1" || trail.Steps[1].StepKey != "C5" {
		t.Errorf("steps = %q and %q, want C1 and C5",
			trail.Steps[0].StepKey, trail.Steps[1].StepKey)
	}

	t.Run("without a step", func(t *testing.T) {
		empty := trail
		empty.Steps = []core.TrailStep{}
		empty.StoppedAt = nil
		if err := empty.Validate(); !errors.Is(err, core.ErrMissingRequiredItem) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrMissingRequiredItem)
		}
	})

	t.Run("stopping at a step outside the trail", func(t *testing.T) {
		outside := trail
		outside.StoppedAt = &core.TrailStep{
			StepKey:         "D3",
			UsedIdentifiers: []string{"dstIP"},
			Output:          "Proxy 側 300 件",
		}
		if err := outside.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
		}
	})

	t.Run("with a repeated step key", func(t *testing.T) {
		repeated := trail
		repeated.Steps = []core.TrailStep{steps[0], steps[0]}
		repeated.StoppedAt = nil
		if err := repeated.Validate(); !errors.Is(err, core.ErrDuplicateElement) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrDuplicateElement)
		}
	})
}

// pointerToLocator はレコード位置への pointer を返す。
func pointerToLocator(locator core.RecordLocator) *core.RecordLocator {
	return &locator
}

// 範囲は両端を確定できたかで 2 つの形を取る。位置を確定できない範囲に 0 を入れない。
func TestRecordRangeSeparatesPositionedFromWholeSource(t *testing.T) {
	positioned := markIIRange(rangeFromSequenceNumber, rangeToSequenceNumber)
	if err := positioned.Validate(); err != nil {
		t.Fatalf("validating a positioned range: %v", err)
	}
	if positioned.FromPosition == nil || *positioned.FromPosition != rangeFromSequenceNumber {
		t.Errorf("fromPosition = %v, want a pointer to %d",
			positioned.FromPosition, rangeFromSequenceNumber)
	}

	wholeSource := markIIWholeSourceRange()
	if err := wholeSource.Validate(); err != nil {
		t.Fatalf("validating a whole source range: %v", err)
	}
	if wholeSource.FromPosition != nil || wholeSource.ToPosition != nil {
		t.Error("a whole source range must not carry positions")
	}

	t.Run("a whole source range carrying a position", func(t *testing.T) {
		broken := markIIWholeSourceRange()
		from := rangeFromSequenceNumber
		broken.FromPosition = &from
		if err := broken.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
		}
	})

	t.Run("a whole source range carrying a position kind", func(t *testing.T) {
		broken := markIIWholeSourceRange()
		broken.PositionKind = core.PositionKindSequenceNumber
		if err := broken.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
		}
	})

	t.Run("a positioned range with one end", func(t *testing.T) {
		broken := markIIRange(rangeFromSequenceNumber, rangeToSequenceNumber)
		broken.ToPosition = nil
		if err := broken.Validate(); !errors.Is(err, core.ErrMissingRequiredItem) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrMissingRequiredItem)
		}
	})
}

// 段階の入力は収集元 2 件以上を持てる。複数の収集元を 1 要素にまとめない。
func TestTrailStepCarriesOneInputRefPerSource(t *testing.T) {
	fileNames := []string{
		"endpoint-01.log", "endpoint-02.log", "endpoint-03.log",
	}
	inputRefs := make([]core.TrailInputRef, 0, len(fileNames))
	for index, fileName := range fileNames {
		identity := markIIIdentity(t)
		identity.FileName = fileName
		identity.OriginPath = "examples/logs/" + fileName
		identity.SourceId = "source-mark-ii-" + fileName
		// 収集元ごとに別の内容の識別を持つ形を確かめる。値は書式例である。
		identity.ContentSha256 = strings.Repeat(string(rune('a'+index)), 64)
		inputRefs = append(inputRefs, core.TrailInputRef{
			Kind:   core.TrailInputKindSource,
			Source: &identity,
		})
	}
	step := core.TrailStep{
		StepKey:         "A3",
		InputRefs:       inputRefs,
		UsedIdentifiers: []string{"com", "ip"},
		Output:          "接続元の端末は " + markIITerminalName,
	}
	if err := step.Validate(); err != nil {
		t.Fatalf("validating the step: %v", err)
	}
	if len(step.InputRefs) != len(fileNames) {
		t.Errorf("inputRefs = %d, want %d", len(step.InputRefs), len(fileNames))
	}

	// 入力のすべてが原資料の外にある段階は inputRefs を出さない。
	t.Run("a step whose inputs are outside the source material", func(t *testing.T) {
		outside := step
		outside.StepKey = "A1"
		outside.InputRefs = nil
		if err := outside.Validate(); err != nil {
			t.Fatalf("validating a step without input refs: %v", err)
		}
	})

	t.Run("an empty set of input refs", func(t *testing.T) {
		empty := step
		empty.InputRefs = []core.TrailInputRef{}
		if err := empty.Validate(); !errors.Is(err, core.ErrMissingRequiredItem) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrMissingRequiredItem)
		}
	})

	t.Run("an input ref carrying both a record and a source", func(t *testing.T) {
		identity := markIIIdentity(t)
		both := core.TrailInputRef{
			Kind:   core.TrailInputKindSource,
			Record: pointerToLocator(markIILocator(firstCandidateSn)),
			Source: &identity,
		}
		if err := both.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
		}
	})

	t.Run("an input ref of kind source without a source", func(t *testing.T) {
		missing := core.TrailInputRef{Kind: core.TrailInputKindSource}
		if err := missing.Validate(); !errors.Is(err, core.ErrMissingRequiredItem) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrMissingRequiredItem)
		}
	})
}

// 段階の並び順と stepKey の順序が食い違う応答を作らない。
func TestDerivationTrailRejectsStepsOutOfKeyOrder(t *testing.T) {
	steps := []core.TrailStep{
		{StepKey: "A2", UsedIdentifiers: []string{"com"}, Output: "端末は " + markIITerminalName},
		{StepKey: "A1", UsedIdentifiers: []string{"問題文の記述"}, Output: "調査の起点"},
	}
	trail := core.DerivationTrail{OriginRef: squidLocator(squidLineNumber), Steps: steps}
	if err := trail.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
		t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
	}

	slices.Reverse(steps)
	inOrder := core.DerivationTrail{OriginRef: squidLocator(squidLineNumber), Steps: steps}
	if err := inOrder.Validate(); err != nil {
		t.Fatalf("validating a trail in key order: %v", err)
	}
}
