package core_test

import (
	"errors"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// apiErrorCodes は code を判定の順序で返す。
func apiErrorCodes() []core.ApiErrorCode {
	return []core.ApiErrorCode{
		"not_implemented", "candidate_window_missing", "invalid_request",
		"source_not_found", "source_hash_mismatch", "import_withheld",
		"position_outside_source", "record_not_found", "internal_error",
	}
}

func TestApiErrorCodeKnownValues(t *testing.T) {
	known := apiErrorCodes()
	for _, code := range known {
		if !code.IsKnown() {
			t.Errorf("code %q must be one of the contract values", code)
		}
	}
	unknown := []core.ApiErrorCode{"bad_request", "unauthorized", "forbidden", ""}
	for _, code := range unknown {
		if code.IsKnown() {
			t.Errorf("code %q must not be accepted", code)
		}
	}
}

func TestApiErrorRequiresTheItemsOfItsCode(t *testing.T) {
	cases := map[string]struct {
		apiError core.ApiError
		want     error
	}{
		"candidate_window_missing without the missing parameters": {
			apiError: core.ApiError{
				Code:    core.ApiErrorCodeCandidateWindowMissing,
				Message: "windowKind is required",
			},
			want: core.ErrMissingRequiredItem,
		},
		// 収集元の 2 項目を揃え、originPath だけを欠いた要求にする。3 項目すべてを
		// 欠いた case は sourceId の検査だけで失敗するため、originPath の検査を
		// 削除しても通ってしまう。
		"source_hash_mismatch without the origin path": {
			apiError: core.ApiError{
				Code:                core.ApiErrorCodeSourceHashMismatch,
				Message:             "the content of the source changed",
				SourceId:            markIISourceId,
				SourceContentSha256: markIISha256,
			},
			want: core.ErrMissingRequiredItem,
		},
		// sourceId と sourceContentSha256 は揃って出る。片方だけを持つ応答は、
		// code に依らず 2 項目の食い違いとして失敗する。
		"source_hash_mismatch without the source id": {
			apiError: core.ApiError{
				Code:                core.ApiErrorCodeSourceHashMismatch,
				Message:             "the content of the source changed",
				SourceContentSha256: markIISha256,
				OriginPath:          markIIOriginPath,
			},
			want: core.ErrInconsistentValue,
		},
		"import_withheld without the import status reference": {
			apiError: core.ApiError{
				Code:    core.ApiErrorCodeImportWithheld,
				Message: "the result of the requested range is withheld",
			},
			want: core.ErrMissingRequiredItem,
		},
		"a message carrying a newline": {
			apiError: core.ApiError{
				Code:              core.ApiErrorCodeInvalidRequest,
				Message:           "depth is required\nGET /api/v0/graph",
				MissingParameters: []string{"depth"},
			},
			want: core.ErrControlCharacter,
		},
		"a source identifier without the content identifier": {
			apiError: core.ApiError{
				Code:              core.ApiErrorCodeInvalidRequest,
				Message:           "sourceContentSha256 is required",
				MissingParameters: []string{"sourceContentSha256"},
				SourceId:          "source-squid",
			},
			want: core.ErrInconsistentValue,
		},
		"a code outside the 10 values": {
			apiError: core.ApiError{
				Code:    "unauthorized",
				Message: "authentication is out of scope",
			},
			want: core.ErrUnknownEnumValue,
		},
		"a failure without a message": {
			apiError: core.ApiError{Code: core.ApiErrorCodeInternalError},
			want:     core.ErrMissingRequiredItem,
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			if err := testCase.apiError.Validate(); !errors.Is(err, testCase.want) {
				t.Errorf("error = %v, want one wrapping %v", err, testCase.want)
			}
		})
	}
}

// missingParameters は欠けている要求の項目があるときだけ出る。
//
// limit に 0 を与える要求は invalid_request で失敗するが、要求の項目は欠けていない。
// 同じ要求がその操作の必須の項目をすべて揃えているとき、応答は missingParameters を
// 出さない。
func TestInvalidRequestCarriesTheMissingParametersOnlyWhenItemsAreAbsent(t *testing.T) {
	withoutMissing := core.ApiError{
		Code:    core.ApiErrorCodeInvalidRequest,
		Message: "limit must be 1 or more",
	}
	if err := withoutMissing.Validate(); err != nil {
		t.Fatalf("validating a failure whose request omits no item: %v", err)
	}
	if len(withoutMissing.MissingParameters) != 0 {
		t.Errorf("missingParameters = %v, want none", withoutMissing.MissingParameters)
	}

	withMissing := core.ApiError{
		Code:              core.ApiErrorCodeInvalidRequest,
		Message:           "sortKey is required",
		MissingParameters: []string{"sortKey"},
	}
	if err := withMissing.Validate(); err != nil {
		t.Fatalf("validating a failure whose request omits an item: %v", err)
	}
	if len(withMissing.MissingParameters) != 1 || withMissing.MissingParameters[0] != "sortKey" {
		t.Errorf("missingParameters = %v, want [sortKey]", withMissing.MissingParameters)
	}

	withEmptyName := core.ApiError{
		Code:              core.ApiErrorCodeInvalidRequest,
		Message:           "sortKey is required",
		MissingParameters: []string{""},
	}
	if err := withEmptyName.Validate(); !errors.Is(err, core.ErrMissingRequiredItem) {
		t.Errorf("error = %v, want one wrapping %v", err, core.ErrMissingRequiredItem)
	}
}

// code の定数が、判定の順序に並べた文字列と 1 対 1 で対応する。
//
// 確かめるのは定数の値と、判定の順序に並べた文字列の対応である。宣言の並びを
// 実行時に読む手立ては Go に無く、本 test も読んでいない。
// 判定そのものは本 package が持たない。どの行の条件に該当したかを決めるには要求の項目の
// 一覧が要り、要求の型は backend/api/ が持つ。
func TestApiErrorCodesMatchTheDecisionOrderOfTheContract(t *testing.T) {
	want := []core.ApiErrorCode{
		core.ApiErrorCodeNotImplemented,
		core.ApiErrorCodeCandidateWindowMissing,
		core.ApiErrorCodeInvalidRequest,
		core.ApiErrorCodeSourceNotFound,
		core.ApiErrorCodeSourceHashMismatch,
		core.ApiErrorCodeImportWithheld,
		core.ApiErrorCodePositionOutsideSource,
		core.ApiErrorCodeRecordNotFound,
		core.ApiErrorCodeInternalError,
	}
	got := apiErrorCodes()
	if len(want) != len(got) {
		t.Fatalf("the constants hold %d codes over the %d written from the contract table",
			len(want), len(got))
	}
	for index := range want {
		if want[index] != got[index] {
			t.Errorf("code at %d = %q, want %q", index, want[index], got[index])
		}
	}
}

func TestApiErrorAcceptsAFullyFilledFailure(t *testing.T) {
	recordRef := squidLocator(squidLineNumber)
	apiError := core.ApiError{
		Code:                core.ApiErrorCodeSourceHashMismatch,
		Message:             "the content of the source changed since the import",
		SourceId:            squidSourceId,
		SourceContentSha256: squidSha256,
		OriginPath:          squidOriginPath,
		RecordRef:           &recordRef,
	}
	if err := apiError.Validate(); err != nil {
		t.Fatalf("validating the failure: %v", err)
	}

	withheld := core.ApiError{
		Code:            core.ApiErrorCodeImportWithheld,
		Message:         "the result of the requested range is withheld",
		ImportStatusRef: "/api/v0/sources",
	}
	if err := withheld.Validate(); err != nil {
		t.Fatalf("validating the withheld failure: %v", err)
	}
}
