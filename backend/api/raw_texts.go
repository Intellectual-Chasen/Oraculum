package api

import (
	"errors"
	"net/http"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

const rawTextsPattern = "GET /api/v0/raw-texts"

// rawTextRefParam は、原文を返す操作への参照 (RecordLocator.recordRawTextRef と
// ImportFailure.rawTextRef) を与える項目の名前である。
const rawTextRefParam = "ref"

// rawTextsHandler は、参照が指すレコードの原文を返す。取り込めなかったレコードの原文も返す。
type rawTextsHandler struct {
	result pipeline.ImportResult
}

// rawTextResponse は原文の要求の応答である。**本型が項目の定義元である。**
//
// RawText は原資料の byte 列をそのまま持つ。制御文字を JSON escape で表すのは
// output.WriteJSON であり、値を書き換えない。
type rawTextResponse struct {
	RawTextRef string `json:"rawTextRef"`
	RawText    string `json:"rawText"`
}

func (h rawTextsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	for name, values := range query {
		if name != rawTextRefParam {
			writeError(w, http.StatusBadRequest, *invalidRequestError(
				errors.New(name+" is not a query parameter of a raw text request"), nil))
			return
		}
		if len(values) != 1 || values[0] == "" {
			writeError(w, http.StatusBadRequest, *invalidRequestError(
				errors.New(name+" must be given once with a value"), nil))
			return
		}
	}
	ref := query.Get(rawTextRefParam)
	if ref == "" {
		writeError(w, http.StatusBadRequest, *invalidRequestError(
			errors.New("required query parameters are missing"), []string{rawTextRefParam}))
		return
	}
	// 公開を止めた収集元の参照は、参照の無いときと同じく見つからない (ImportResult.RawText)。
	text, found := h.result.RawText(ref)
	if !found {
		writeError(w, httpStatusFor(core.ApiErrorCodeRecordNotFound), core.ApiError{
			Code: core.ApiErrorCodeRecordNotFound, Message: "no published record has the raw text reference",
		})
		return
	}
	writeJSON(w, http.StatusOK, rawTextResponse{RawTextRef: ref, RawText: text})
}
