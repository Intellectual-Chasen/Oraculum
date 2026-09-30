package core_test

import (
	"encoding/json"
	"errors"
	"math"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 区分の重複、未知の区分、負の件数、4 個目の要素を構築で拒否する。
func TestNewImportCountSetRejectsBrokenSets(t *testing.T) {
	cases := map[string]struct {
		counts []core.ImportCount
		want   error
	}{
		"a repeated category": {
			counts: []core.ImportCount{
				{Category: core.ImportCategoryRead, Count: 10},
				{Category: core.ImportCategoryRead, Count: 12},
			},
			want: core.ErrDuplicateElement,
		},
		"a category outside the 3 values": {
			counts: []core.ImportCount{
				{Category: "partially_read", Count: 10},
			},
			want: core.ErrUnknownEnumValue,
		},
		"a negative count": {
			counts: []core.ImportCount{
				{Category: core.ImportCategoryRead, Count: -1},
			},
			want: core.ErrNegativeCount,
		},
		"a fourth element": {
			counts: []core.ImportCount{
				{Category: core.ImportCategoryRead, Count: 800},
				{Category: core.ImportCategorySucceeded, Count: 799},
				{Category: core.ImportCategoryFailed, Count: 1},
				{Category: core.ImportCategoryRead, Count: 800},
			},
			want: core.ErrTooManyElements,
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := core.NewImportCountSet(testCase.counts...); !errors.Is(err, testCase.want) {
				t.Errorf("error = %v, want one wrapping %v", err, testCase.want)
			}
		})
	}
}

// 3 区分を揃えて出す集合は、read の count が succeeded と failed の和に一致する。
// 一致しない値を構築で拒否する。
func TestImportCountSetRequiresReadToBeTheSumOfBothOutcomes(t *testing.T) {
	if _, err := core.NewImportCountSet(
		core.ImportCount{Category: core.ImportCategoryRead, Count: 800},
		core.ImportCount{Category: core.ImportCategorySucceeded, Count: 798},
		core.ImportCount{Category: core.ImportCategoryFailed, Count: 1},
	); !errors.Is(err, core.ErrInconsistentValue) {
		t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
	}

	// 3 区分が揃っていない集合には和の一致を求めない。
	if _, err := core.NewImportCountSet(
		core.ImportCount{Category: core.ImportCategoryRead, Count: 800},
		core.ImportCount{Category: core.ImportCategorySucceeded, Count: 798},
	); err != nil {
		t.Fatalf("building a set of two categories: %v", err)
	}

	// 位置を確定できない失敗がある応答でも、read は succeeded と failed の和のままである。
	if _, err := core.NewImportCountSet(
		core.ImportCount{Category: core.ImportCategoryRead, Count: 800},
		core.ImportCount{Category: core.ImportCategorySucceeded, Count: 800},
		core.ImportCount{Category: core.ImportCategoryFailed, Count: 0},
	); err != nil {
		t.Fatalf("building a set whose failures have no known position: %v", err)
	}
}

// 3 区分を揃えて出す応答では、read の count が succeeded と failed の和に一致する。
func TestNewImportCountSetAcceptsThreeCategories(t *testing.T) {
	counts, err := core.NewImportCountSet(
		core.ImportCount{Category: core.ImportCategoryRead, Count: 800},
		core.ImportCount{Category: core.ImportCategorySucceeded, Count: 799},
		core.ImportCount{Category: core.ImportCategoryFailed, Count: 1},
	)
	if err != nil {
		t.Fatalf("building the count set: %v", err)
	}
	if counts.Len() != 3 {
		t.Errorf("Len = %d, want 3", counts.Len())
	}
	if count, ok := counts.Count(core.ImportCategoryRead); !ok || count != 800 {
		t.Errorf("read = (%d, %v), want (800, true)", count, ok)
	}
	// 3 区分を固定値で確かめる。取得した値を足して期待値を作ると、succeeded と failed が
	// 入れ替わっても和が変わらず、入れ替わりを検出できない。
	succeeded, succeededOk := counts.Count(core.ImportCategorySucceeded)
	if !succeededOk || succeeded != 799 {
		t.Errorf("succeeded = (%d, %v), want (799, true)", succeeded, succeededOk)
	}
	failed, failedOk := counts.Count(core.ImportCategoryFailed)
	if !failedOk || failed != 1 {
		t.Errorf("failed = (%d, %v), want (1, true)", failed, failedOk)
	}
	// 3 つを揃えて出す応答では read が succeeded と failed の和に一致する。
	if read, _ := counts.Count(core.ImportCategoryRead); read != succeeded+failed {
		t.Errorf("read = %d, want %d", read, succeeded+failed)
	}
}

// 要素が無い区分と、件数が 0 の区分を別の結果にする。
func TestCountSeparatesAnUncountedCategoryFromZero(t *testing.T) {
	counts, err := core.NewImportCountSet(
		core.ImportCount{Category: core.ImportCategoryFailed, Count: 0},
	)
	if err != nil {
		t.Fatalf("building the count set: %v", err)
	}

	zeroCount, zeroOk := counts.Count(core.ImportCategoryFailed)
	if !zeroOk {
		t.Error("a category with a count of 0 must be reported as counted")
	}
	if zeroCount != 0 {
		t.Errorf("count = %d, want 0", zeroCount)
	}

	uncounted, uncountedOk := counts.Count(core.ImportCategoryRead)
	if uncountedOk {
		t.Error("a category without an element must be reported as not counted")
	}
	if uncounted != 0 {
		t.Errorf("count = %d, want 0", uncounted)
	}

	encoded, err := json.Marshal(counts)
	if err != nil {
		t.Fatalf("marshaling the count set: %v", err)
	}
	want := `[{"category":"failed","count":0}]`
	if string(encoded) != want {
		t.Errorf("json = %s, want %s", encoded, want)
	}

	emptySet, err := core.NewImportCountSet()
	if err != nil {
		t.Fatalf("building the empty count set: %v", err)
	}
	emptyEncoded, err := json.Marshal(emptySet)
	if err != nil {
		t.Fatalf("marshaling the empty count set: %v", err)
	}
	if string(emptyEncoded) != `[]` {
		t.Errorf("json = %s, want []", emptyEncoded)
	}
}

func TestImportCountSetJsonRoundTrip(t *testing.T) {
	encoded := `[{"category":"read","count":800},{"category":"succeeded","count":799}]`
	var counts core.ImportCountSet
	if err := json.Unmarshal([]byte(encoded), &counts); err != nil {
		t.Fatalf("decoding the count set: %v", err)
	}
	if count, ok := counts.Count(core.ImportCategorySucceeded); !ok || count != 799 {
		t.Errorf("succeeded = (%d, %v), want (799, true)", count, ok)
	}
	reEncoded, err := json.Marshal(counts)
	if err != nil {
		t.Fatalf("marshaling the count set: %v", err)
	}
	if string(reEncoded) != encoded {
		t.Errorf("json = %s, want %s", reEncoded, encoded)
	}
}

func TestImportCountSetUnmarshalRejectsBrokenSets(t *testing.T) {
	cases := map[string]string{
		"a repeated category":             `[{"category":"read","count":10},{"category":"read","count":12}]`,
		"a category outside the 3 values": `[{"category":"partially_read","count":10}]`,
		"an item outside the contract":    `[{"category":"read","count":10,"scope":"all"}]`,
		// count に 0 を置いて「数えていない」を表さないため、欠けた count を 0 へ
		// 復元しない。
		"a category without a count": `[{"category":"read"}]`,
		"a sum that does not match":  `[{"category":"read","count":10},{"category":"succeeded","count":8},{"category":"failed","count":1}]`,
	}
	for name, encoded := range cases {
		t.Run(name, func(t *testing.T) {
			var counts core.ImportCountSet
			if err := json.Unmarshal([]byte(encoded), &counts); err == nil {
				t.Error("decoding must fail")
			}
		})
	}
}

// 未計測の集合を持つ ImportStatus と、部分的に公開した状態を確かめる。
func TestImportStatusHoldsThePartialState(t *testing.T) {
	counts, err := core.NewImportCountSet(
		core.ImportCount{Category: core.ImportCategoryRead, Count: 800},
		core.ImportCount{Category: core.ImportCategorySucceeded, Count: 799},
		core.ImportCount{Category: core.ImportCategoryFailed, Count: 1},
	)
	if err != nil {
		t.Fatalf("building the count set: %v", err)
	}
	failure := importFailure(t)
	status := core.ImportStatus{
		SourceId: markIISourceId,
		Scope:    markIIRange(rangeFromSequenceNumber, rangeToSequenceNumber),
		Counts:   counts,
		DiagnosisCounts: []core.DiagnosisCount{
			{DiagnosisClass: core.DiagnosisClassUndetermined, Count: 1},
		},
		PublicationState: core.PublicationStatePublishedPartial,
		Failures:         []core.ImportFailure{failure},
		FailureCount:     1,
		AnalysisRunRef:   "analysis-run-1",
	}
	if err := status.Validate(); err != nil {
		t.Fatalf("validating the import status: %v", err)
	}
	if len(status.Failures) != 1 {
		t.Errorf("failures = %d, want 1", len(status.Failures))
	}
	if status.Counts.Len() != 3 {
		t.Errorf("counts = %d, want 3", status.Counts.Len())
	}
	if len(status.DiagnosisCounts) != 1 {
		t.Errorf("diagnosisCounts = %d, want 1", len(status.DiagnosisCounts))
	}
}

func TestImportStatusRejectsBrokenStates(t *testing.T) {
	base := func(t *testing.T) core.ImportStatus {
		t.Helper()
		return core.ImportStatus{
			SourceId:         markIISourceId,
			Scope:            markIIRange(rangeFromSequenceNumber, rangeToSequenceNumber),
			PublicationState: core.PublicationStatePublishedFull,
			Failures:         []core.ImportFailure{},
			FailureCount:     0,
			AnalysisRunRef:   "analysis-run-1",
		}
	}

	t.Run("a withheld state without a reason", func(t *testing.T) {
		status := base(t)
		status.PublicationState = core.PublicationStateWithheld
		if err := status.Validate(); !errors.Is(err, core.ErrUnknownEnumValue) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrUnknownEnumValue)
		}
	})

	t.Run("a reason without a withheld state", func(t *testing.T) {
		status := base(t)
		status.WithheldReason = core.WithheldReasonIdentifierCollision
		if err := status.Validate(); !errors.Is(err, core.ErrUnexpectedItem) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrUnexpectedItem)
		}
	})

	t.Run("a scope pointing at another source", func(t *testing.T) {
		status := base(t)
		status.Scope.SourceId = "source-squid"
		if err := status.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
		}
	})

	t.Run("a failure count below the number of failures", func(t *testing.T) {
		status := base(t)
		status.Failures = []core.ImportFailure{importFailure(t)}
		if err := status.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
		}
	})
}

// field_map / normalize / relate の失敗はレコードの位置を必ず持つ。
// read と tokenize の失敗は位置を確定できない場合がある。
func TestImportFailureRequiresTheRecordRefAfterTheRecordIsDelimited(t *testing.T) {
	requiring := []core.FailureStage{
		core.FailureStageFieldMap, core.FailureStageNormalize, core.FailureStageRelate,
	}
	for _, stage := range requiring {
		t.Run(string(stage)+" without a record position", func(t *testing.T) {
			failure := importFailure(t)
			failure.Stage = stage
			failure.RecordRef = nil
			failure.RawTextRef = ""
			if err := failure.Validate(); !errors.Is(err, core.ErrMissingRequiredItem) {
				t.Errorf("error = %v, want one wrapping %v", err, core.ErrMissingRequiredItem)
			}
		})
		t.Run(string(stage)+" with a record position", func(t *testing.T) {
			failure := importFailure(t)
			failure.Stage = stage
			if err := failure.Validate(); err != nil {
				t.Errorf("validating a %s failure: %v", stage, err)
			}
		})
	}

	allowing := []core.FailureStage{core.FailureStageRead, core.FailureStageTokenize}
	for _, stage := range allowing {
		t.Run(string(stage)+" without a record position", func(t *testing.T) {
			failure := importFailure(t)
			failure.Stage = stage
			failure.RecordRef = nil
			failure.RawTextRef = ""
			if err := failure.Validate(); err != nil {
				t.Errorf("validating a %s failure without a record position: %v", stage, err)
			}
		})
	}
}

// 失敗が指す収集元と行番号は、レコード位置の同名の項目と同じ値である。
func TestImportFailureMatchesItsRecordRef(t *testing.T) {
	t.Run("a record position in another source", func(t *testing.T) {
		failure := importFailure(t)
		recordRef := squidLocator(squidLineNumber)
		failure.RecordRef = &recordRef
		if err := failure.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
		}
	})

	t.Run("a line number only on the failure", func(t *testing.T) {
		failure := importFailure(t)
		lineNumber := int64(529)
		failure.LineNumber = &lineNumber
		if err := failure.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
		}
	})

	t.Run("the same line number on both items", func(t *testing.T) {
		failure := importFailure(t)
		lineNumber := int64(529)
		recordRef := markIILocator(failedSequenceNumber)
		recordRef.LineNumber = &lineNumber
		failure.RecordRef = &recordRef
		failure.LineNumber = &lineNumber
		if err := failure.Validate(); err != nil {
			t.Fatalf("validating a failure carrying a line number: %v", err)
		}
	})

	t.Run("two different line numbers", func(t *testing.T) {
		failure := importFailure(t)
		recordLine := int64(529)
		failureLine := int64(530)
		recordRef := markIILocator(failedSequenceNumber)
		recordRef.LineNumber = &recordLine
		failure.RecordRef = &recordRef
		failure.LineNumber = &failureLine
		if err := failure.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
		}
	})
}

// 返却した失敗は、分類ごとの件数と区分ごとの件数に数えられている。
func TestImportStatusCountsTheFailuresItReturns(t *testing.T) {
	counts, err := core.NewImportCountSet(
		core.ImportCount{Category: core.ImportCategoryRead, Count: 800},
		core.ImportCount{Category: core.ImportCategorySucceeded, Count: 799},
		core.ImportCount{Category: core.ImportCategoryFailed, Count: 1},
	)
	if err != nil {
		t.Fatalf("building the count set: %v", err)
	}
	status := core.ImportStatus{
		SourceId: markIISourceId,
		Scope:    markIIRange(rangeFromSequenceNumber, rangeToSequenceNumber),
		Counts:   counts,
		DiagnosisCounts: []core.DiagnosisCount{
			{DiagnosisClass: core.DiagnosisClassUndetermined, Count: 1},
		},
		PublicationState: core.PublicationStatePublishedPartial,
		Failures:         []core.ImportFailure{importFailure(t)},
		FailureCount:     1,
		AnalysisRunRef:   "analysis-run-1",
	}
	if err := status.Validate(); err != nil {
		t.Fatalf("validating the import status: %v", err)
	}

	t.Run("a class that the diagnosis counts do not carry", func(t *testing.T) {
		broken := status
		broken.DiagnosisCounts = []core.DiagnosisCount{
			{DiagnosisClass: core.DiagnosisClassUnsupportedFormat, Count: 1},
		}
		if err := broken.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
		}
	})

	// 位置を確定できた失敗は counts の failed に算入される。
	t.Run("a failed category of zero next to a positioned failure", func(t *testing.T) {
		broken := status
		zeroFailed, countErr := core.NewImportCountSet(
			core.ImportCount{Category: core.ImportCategoryRead, Count: 800},
			core.ImportCount{Category: core.ImportCategorySucceeded, Count: 800},
			core.ImportCount{Category: core.ImportCategoryFailed, Count: 0},
		)
		if countErr != nil {
			t.Fatalf("building the count set: %v", countErr)
		}
		broken.Counts = zeroFailed
		if err := broken.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
		}
	})

	// failureCount は failures の要素数と一致する。応答は失敗を全件持つ。
	t.Run("a failure count differing from the number of failures", func(t *testing.T) {
		broken := status
		broken.FailureCount = int64(len(status.Failures)) + 1
		if err := broken.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
		}
	})

	// failed が数えるのは位置を確定できた失敗であり、failureCount の部分集合である。
	t.Run("a failed category larger than the failure count", func(t *testing.T) {
		broken := status
		tooManyFailed, countErr := core.NewImportCountSet(
			core.ImportCount{Category: core.ImportCategoryRead, Count: 800},
			core.ImportCount{Category: core.ImportCategorySucceeded, Count: 798},
			core.ImportCount{Category: core.ImportCategoryFailed, Count: 2},
		)
		if countErr != nil {
			t.Fatalf("building the count set: %v", countErr)
		}
		broken.Counts = tooManyFailed
		if err := broken.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
		}
	})

	// 位置を確定できない失敗があると、failed は failureCount を下回る。この形は通る。
	t.Run("a failed category below the failure count", func(t *testing.T) {
		withUnpositioned := status
		withUnpositioned.FailureCount = 2
		// レコードの区切りを確定する前に止まった失敗は recordRef を持たない。
		// recordRef を持つのは field_map / normalize / relate の段階の失敗である。
		unpositioned := importFailure(t)
		unpositioned.Stage = core.FailureStageRead
		unpositioned.RecordRef = nil
		unpositioned.RawTextRef = ""
		withUnpositioned.Failures = append([]core.ImportFailure{status.Failures[0]}, unpositioned)
		withUnpositioned.DiagnosisCounts = []core.DiagnosisCount{
			{DiagnosisClass: core.DiagnosisClassUndetermined, Count: 2},
		}
		if err := withUnpositioned.Validate(); err != nil {
			t.Fatalf("validating a status whose failed count is below the failure count: %v", err)
		}
	})

	// 取り込めなかったレコードがある収集元は範囲の全体を公開していない。
	t.Run("a fully published source carrying failures", func(t *testing.T) {
		broken := status
		broken.PublicationState = core.PublicationStatePublishedFull
		if err := broken.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
		}
	})

	// 失敗が別の収集元を指す応答を落第させる。
	t.Run("a failure pointing at another source", func(t *testing.T) {
		broken := status
		failure := importFailure(t)
		failure.SourceId = squidSourceId
		failure.SourceContentSha256 = squidSha256
		recordRef := squidLocator(squidLineNumber)
		failure.RecordRef = &recordRef
		broken.Failures = []core.ImportFailure{failure}
		if err := broken.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
		}
	})
}

// 分類ごとの件数の和が int64 の範囲を超える応答を拒否する。桁あふれで和が回り込むと、
// failureCount との一致の検査が通ってしまう。
func TestImportStatusRejectsAnOverflowingDiagnosisSum(t *testing.T) {
	const nearMaxInt64 = int64(math.MaxInt64) - 1
	counts, err := core.NewImportCountSet(
		core.ImportCount{Category: core.ImportCategoryFailed, Count: 0},
	)
	if err != nil {
		t.Fatalf("building the count set: %v", err)
	}
	status := core.ImportStatus{
		SourceId: markIISourceId,
		Scope:    markIIRange(rangeFromSequenceNumber, rangeToSequenceNumber),
		Counts:   counts,
		DiagnosisCounts: []core.DiagnosisCount{
			{DiagnosisClass: core.DiagnosisClassUndetermined, Count: nearMaxInt64},
			{DiagnosisClass: core.DiagnosisClassUnsupportedFormat, Count: nearMaxInt64},
		},
		PublicationState: core.PublicationStatePublishedFull,
		Failures:         []core.ImportFailure{},
		FailureCount:     0,
		AnalysisRunRef:   "analysis-run-1",
	}
	if err := status.Validate(); !errors.Is(err, core.ErrCountOverflow) {
		t.Errorf("error = %v, want one wrapping %v", err, core.ErrCountOverflow)
	}
}

// 診断ログに出す表現に制御文字と改行が入らないことを確かめる。
func TestImportFailureRejectsAControlCharacterInTheSanitizedMessage(t *testing.T) {
	failure := importFailure(t)
	failure.SanitizedMessage = "unsupported subEvent\nread: 1"
	if err := failure.Validate(); !errors.Is(err, core.ErrControlCharacter) {
		t.Errorf("error = %v, want one wrapping %v", err, core.ErrControlCharacter)
	}
}

// 診断の分類が undetermined の失敗は、確定できなかった理由を必ず持つ。
func TestImportFailureRequiresAnUnresolvedReasonWhenUndetermined(t *testing.T) {
	failure := importFailure(t)
	failure.UnresolvedReason = ""
	if err := failure.Validate(); !errors.Is(err, core.ErrMissingRequiredItem) {
		t.Errorf("error = %v, want one wrapping %v", err, core.ErrMissingRequiredItem)
	}

	failure.DiagnosisClass = core.DiagnosisClassUnsupportedFormat
	if err := failure.Validate(); err != nil {
		t.Errorf("validating a failure of another class: %v", err)
	}
}

// failedSequenceNumber は取り込みに失敗したレコードの通番である。
//
// 収集元の範囲の両端の間に置く。収集元の scope が markii 形式の範囲を持つため、範囲の外の
// 通番を入れると、同じ fixture の中で 2 つの値が噛み合わなくなる。
const failedSequenceNumber = int64(900500)

// importFailure は失敗 1 件を返す。
//
// 位置を markii 形式の通番で表すのは、positionKind が sequence_number であり、
// sequenceNumber と lineNumber を併せて持てることを示すためである。
//
// 失敗の理由に、取り込みを続けられない文字列の不整合を置く。観測の種別の意味が確定しない
// 状態を失敗の理由にしない。2 つは別の状態であり、後者は取り込みに成功した
// レコードの ObservationKind の status が持つ。ImportFailure の diagnosisClass の
// undetermined と、ObservationKind の status の undetermined は別のものを指す。
func importFailure(t *testing.T) core.ImportFailure {
	t.Helper()
	recordRef := markIILocator(failedSequenceNumber)
	return core.ImportFailure{
		SourceId:            markIISourceId,
		SourceContentSha256: markIISha256,
		RecordRef:           &recordRef,
		DiagnosisClass:      core.DiagnosisClassUndetermined,
		Stage:               core.FailureStageFieldMap,
		Interpretation:      "UTF-8, LF, 03/14/2024 10:20:30.482 +0900",
		ExpectedMeaning:     "ヘッダーの時刻が 03/14/2024 10:20:30.482 の形である",
		ObservedResult:      "03/14/2024 10:20:30.4",
		UnresolvedReason:    "文字列が途中で切れており、桁の欠落と区切りの誤りを分けられない",
		ParserVersion:       "markii-parser-0",
		SanitizedMessage:    "truncated header timestamp",
		RawTextRef:          "/api/v0/records",
	}
}
