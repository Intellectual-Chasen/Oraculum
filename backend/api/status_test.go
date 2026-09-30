package api

import (
	"net/http"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// httpStatusFor は ApiErrorCode のすべての値に status を割り当てる。
//
// 割り当てを忘れた値は default の 500 になる。500 は internal_error だけが取る値なので、
// 他の値が 500 を取った状態を割り当て漏れとして検出する。allowed は API の契約が定める
// status の一覧である。
func TestHttpStatusForCoversEveryCode(t *testing.T) {
	allowed := map[int]bool{
		http.StatusNotImplemented:      true,
		http.StatusBadRequest:          true,
		http.StatusConflict:            true,
		http.StatusNotFound:            true,
		http.StatusForbidden:           true,
		http.StatusUnauthorized:        true,
		http.StatusTooManyRequests:     true,
		http.StatusInternalServerError: true,
	}

	codes := core.ApiErrorCodes()
	if len(codes) == 0 {
		t.Fatal("core.ApiErrorCodes returned no value")
	}
	for _, code := range codes {
		status := httpStatusFor(code)
		if !allowed[status] {
			t.Errorf("httpStatusFor(%q) = %d, want one of the statuses the contract lists",
				code, status)
		}
		if status == http.StatusInternalServerError && code != core.ApiErrorCodeInternalError {
			t.Errorf("httpStatusFor(%q) = 500; only internal_error takes 500, so %q has no status",
				code, code)
		}
	}
	if got := httpStatusFor(core.ApiErrorCodeInternalError); got != http.StatusInternalServerError {
		t.Errorf("httpStatusFor(internal_error) = %d, want %d", got, http.StatusInternalServerError)
	}
}

// 定義の外にある code は 500 を取る。
func TestHttpStatusForUnknownCode(t *testing.T) {
	unknown := core.ApiErrorCode("no_such_code")
	if unknown.IsKnown() {
		t.Fatalf("%q must not be a known code", unknown)
	}
	if got := httpStatusFor(unknown); got != http.StatusInternalServerError {
		t.Errorf("httpStatusFor(%q) = %d, want %d", unknown, got, http.StatusInternalServerError)
	}
}
