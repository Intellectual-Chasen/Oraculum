package core_test

import (
	"errors"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 検索式の誤りは invalid_request の失敗だけが持ち、理由は定義の中の値で、範囲は負にならない。
func TestApiErrorCarriesTheSearchExpressionErrorOnlyForAnInvalidRequest(t *testing.T) {
	valid := core.SearchExpressionError{
		Reason: core.SearchExpressionErrorReasonMissingValue, Offset: 4, Length: 2,
	}
	for _, testCase := range []struct {
		name   string
		code   core.ApiErrorCode
		detail core.SearchExpressionError
		valid  bool
	}{
		{"an invalid request", core.ApiErrorCodeInvalidRequest, valid, true},
		{"an empty range at the end", core.ApiErrorCodeInvalidRequest, core.SearchExpressionError{
			Reason: core.SearchExpressionErrorReasonMissingOperand, Offset: 9,
		}, true},
		{"another code", core.ApiErrorCodeInternalError, valid, false},
		{"an unknown reason", core.ApiErrorCodeInvalidRequest, core.SearchExpressionError{Reason: "typo"}, false},
		{"a negative offset", core.ApiErrorCodeInvalidRequest, core.SearchExpressionError{
			Reason: core.SearchExpressionErrorReasonMissingValue, Offset: -1,
		}, false},
		{"a negative length", core.ApiErrorCodeInvalidRequest, core.SearchExpressionError{
			Reason: core.SearchExpressionErrorReasonMissingValue, Length: -1,
		}, false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			detail := testCase.detail
			err := core.ApiError{
				Code: testCase.code, Message: "searchExpression has a syntax error", SearchExpressionError: &detail,
			}.Validate()
			if testCase.valid && err != nil {
				t.Errorf("Validate returned %v, want nil", err)
			}
			if !testCase.valid && !errors.Is(err, core.ErrInvalid) {
				t.Errorf("Validate returned %v, want ErrInvalid", err)
			}
		})
	}
}

// 理由の一覧のどの値も定義の中にあり、一覧の外の値は定義の外にある。
func TestSearchExpressionErrorReasonsAreKnown(t *testing.T) {
	for _, reason := range core.SearchExpressionErrorReasons() {
		if !reason.IsKnown() {
			t.Errorf("%q is not known", reason)
		}
	}
	for _, reason := range []core.SearchExpressionErrorReason{"", "syntax_error", "EMPTY_EXPRESSION"} {
		if reason.IsKnown() {
			t.Errorf("%q is known, want outside the contract", reason)
		}
	}
}
