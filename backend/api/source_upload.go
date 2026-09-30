package api

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net/http"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

const sourceUploadPattern = "POST /api/v0/stages/source-files"

// uploadSource は 1 件の資料をストリームで受け取り、その資料と付属資料の形式判定を返す。
func (h *stagedHandler) uploadSource(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	if len(query["path"]) != 1 || query.Get("path") == "" || len(query["upload"]) > 1 || len(query) > 2 || (len(query) == 2 && !query.Has("upload")) {
		writeError(w, http.StatusBadRequest, core.ApiError{Code: core.ApiErrorCodeInvalidRequest, Message: "the upload requires a single relative path"})
		return
	}
	uploader, ok := h.stages.(interface {
		UploadRequestedFile(context.Context, string, string, io.Reader) (core.SourceUploadResult, error)
	})
	if !ok {
		writeError(w, http.StatusBadRequest, core.ApiError{Code: core.ApiErrorCodeInvalidRequest, Message: "uploads are unavailable"})
		return
	}
	listing, err := uploader.UploadRequestedFile(r.Context(), query.Get("upload"), query.Get("path"), r.Body)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			writeError(w, http.StatusConflict, core.ApiError{Code: core.ApiErrorCodeSourceUploadAlreadyExists, Message: "the uploaded source already exists"})
			return
		}
		writeStageError(w, err)
		return
	}
	if err := listing.Validate(); err != nil {
		writeError(w, http.StatusInternalServerError, core.ApiError{Code: core.ApiErrorCodeInternalError, Message: "reading uploaded source failed"})
		return
	}
	writeJSON(w, http.StatusCreated, listing)
}
