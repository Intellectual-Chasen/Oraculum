package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// ワークスペースの操作の path。
const (
	workspacesPath     = "/api/v0/workspaces"
	workspacePath      = "/api/v0/workspaces/{workspaceId}"
	workspaceSharePath = "/api/v0/workspaces/{workspaceId}/shares/{login}"
)

// localOwner は、アカウントを持たない起動のワークスペースの所有者である。アカウントのログイン名に
// 使えない予約した名前である。
const localOwner = "local"

// accessOwner は、要求の利用者がワークスペースの所有者であることを表す access の値である。
const accessOwner = "owner"

// Workspaces はワークスペースを保つ port である。本番は pipeline.WorkspaceStore が実装する。
type Workspaces interface {
	Get(id string) (pipeline.Workspace, bool)
	List(include func(pipeline.Workspace) bool) []pipeline.Workspace
	Create(owner, name string, state []byte) (pipeline.Workspace, error)
	Update(id, actor string, baseRevision int64, name string, state []byte) (pipeline.Workspace, pipeline.WorkspaceChange, error)
	Change(id, actor, clientChangeId string, baseRevision int64, patch map[string]json.RawMessage) (pipeline.WorkspaceChangeResult, error)
	Delete(id, actor string) error
	Share(id, actor, login string, access pipeline.WorkspaceAccess) (pipeline.WorkspaceShare, error)
	Unshare(id, actor, login string) error
}

// workspaceSummary は一覧の 1 件である。**本型が項目の定義元である。** Access は要求の利用者が行える
// 操作 (owner / edit / view) である。
type workspaceSummary struct {
	Id        string `json:"id"`
	Name      string `json:"name"`
	Owner     string `json:"owner"`
	Revision  int64  `json:"revision"`
	UpdatedAt string `json:"updatedAt"`
	UpdatedBy string `json:"updatedBy"`
	Access    string `json:"access"`
}

// workspaceShareItem は共有先の利用者 1 人である。
type workspaceShareItem struct {
	Login     string                   `json:"login"`
	Access    pipeline.WorkspaceAccess `json:"access"`
	GrantedBy string                   `json:"grantedBy"`
	GrantedAt string                   `json:"grantedAt"`
}

// workspaceItem はワークスペース 1 件である。state は画面が保存した JSON のままである。Shares は
// 所有者の要求だけが持つ。
type workspaceItem struct {
	workspaceSummary
	State  json.RawMessage       `json:"state"`
	Shares *[]workspaceShareItem `json:"shares,omitempty"`
}

type workspacesResponse struct {
	Workspaces []workspaceSummary `json:"workspaces"`
}

// workspaceRequestBody はワークスペースを作る要求と置き換える要求の本文である。BaseRevision は
// 置き換える要求だけが持つ。Name を持たない作成は、server が「ワークスペース N」の名前を付ける。
type workspaceRequestBody struct {
	Name         string          `json:"name"`
	State        json.RawMessage `json:"state"`
	BaseRevision *int64          `json:"baseRevision,omitempty"`
}

// workspaceShareRequestBody はワークスペースを共有する要求の本文である。
type workspaceShareRequestBody struct {
	Access pipeline.WorkspaceAccess `json:"access"`
}

// NewWorkspaceHandler は、ワークスペースの保存・一覧・読み出し・置き換え・削除・共有に答え、それ以外の
// 要求を next へ渡す。
//
// **ワークスペースの識別子を知るだけでは読めない。** 所有者でも共有先でもない利用者の要求には、
// ワークスペースの存在を返さない (record_not_found)。アカウントを持たない起動の所有者は localOwner
// であり、access と accounts は nil である。共有先の役割は外側の NewAccessHandler が確かめる。
//
// 変更・置き換え・削除・共有の取り消しは hub の順の中で書き、接続中の利用者へ配信する。
func NewWorkspaceHandler(next http.Handler, store Workspaces, access Access, accounts Accounts, hub *WorkspaceHub) http.Handler {
	handler := workspacesHandler{store: store, access: access, accounts: accounts, hub: hub}
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+workspacesPath, handler.list)
	mux.HandleFunc("POST "+workspacesPath, handler.create)
	mux.HandleFunc("GET "+workspacePath, handler.get)
	mux.HandleFunc("PUT "+workspacePath, handler.replace)
	mux.HandleFunc("DELETE "+workspacePath, handler.remove)
	mux.HandleFunc("PUT "+workspaceSharePath, handler.share)
	mux.HandleFunc("DELETE "+workspaceSharePath, handler.unshare)
	mux.HandleFunc("POST "+workspaceChangesPath, handler.change)
	mux.HandleFunc("GET "+workspaceEventsPath, handler.events)
	mux.HandleFunc("POST "+workspacePresencePath, handler.presence)
	mux.Handle("/", next)
	return mux
}

type workspacesHandler struct {
	store    Workspaces
	access   Access
	accounts Accounts
	hub      *WorkspaceHub
}

// requestOwner は要求の利用者の、ワークスペースの所有者としての名前を返す。
func requestOwner(r *http.Request) string {
	if account, ok := requestAccount(r); ok {
		return account.Login
	}
	return localOwner
}

// accessOf は login がワークスペースに行える操作を返す。所有者でも共有先でもなければ ok が偽である。
func accessOf(workspace pipeline.Workspace, login string) (string, bool) {
	if workspace.Owner == login {
		return accessOwner, true
	}
	share, ok := workspace.ShareOf(login)
	return string(share.Access), ok
}

func (h workspacesHandler) list(w http.ResponseWriter, r *http.Request) {
	login := requestOwner(r)
	listed := h.store.List(func(workspace pipeline.Workspace) bool {
		_, ok := accessOf(workspace, login)
		return ok
	})
	response := workspacesResponse{Workspaces: make([]workspaceSummary, 0, len(listed))}
	for _, workspace := range listed {
		response.Workspaces = append(response.Workspaces, summaryOf(workspace, login))
	}
	writeJSON(w, http.StatusOK, response)
}

func (h workspacesHandler) create(w http.ResponseWriter, r *http.Request) {
	body, apiError := decodeWorkspaceBody(r, false)
	if apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	workspace, err := h.store.Create(requestOwner(r), body.Name, body.State)
	if err != nil {
		writeWorkspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, itemOf(workspace, requestOwner(r)))
}

// readable は、要求の利用者が読めるワークスペースと、行える操作を返す。読めなければ 404 を書き、
// ok を偽にする。
func (h workspacesHandler) readable(w http.ResponseWriter, r *http.Request) (pipeline.Workspace, string, bool) {
	workspace, ok := h.store.Get(r.PathValue("workspaceId"))
	access, shared := accessOf(workspace, requestOwner(r))
	if !ok || !shared {
		writeError(w, http.StatusNotFound, core.ApiError{
			Code: core.ApiErrorCodeRecordNotFound, Message: "no workspace matches the identifier",
		})
		return pipeline.Workspace{}, "", false
	}
	return workspace, access, true
}

// permitted は access が allowed のどれかであるかを返す。違えば 403 を書く。
func permitted(w http.ResponseWriter, access string, allowed ...string) bool {
	for _, value := range allowed {
		if access == value {
			return true
		}
	}
	writeError(w, http.StatusForbidden, core.ApiError{
		Code: core.ApiErrorCodePermissionDenied, Message: "the access to the workspace does not allow the operation",
	})
	return false
}

func (h workspacesHandler) get(w http.ResponseWriter, r *http.Request) {
	if workspace, _, ok := h.readable(w, r); ok {
		writeJSON(w, http.StatusOK, itemOf(workspace, requestOwner(r)))
	}
}

func (h workspacesHandler) replace(w http.ResponseWriter, r *http.Request) {
	workspace, access, ok := h.readable(w, r)
	if !ok || !permitted(w, access, accessOwner, string(pipeline.WorkspaceEdit)) {
		return
	}
	body, apiError := decodeWorkspaceBody(r, true)
	if apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	var updated pipeline.Workspace
	err := h.hub.commit(workspace.Id, func() (*syncMessage, error) {
		if err := h.checkEditable(r, workspace.Id); err != nil {
			return nil, err
		}
		var change pipeline.WorkspaceChange
		var err error
		updated, change, err = h.store.Update(workspace.Id, requestOwner(r), *body.BaseRevision, body.Name, body.State)
		if err != nil {
			return nil, err
		}
		return replacedMessage(updated, change), nil
	})
	if err != nil {
		writeWorkspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, itemOf(updated, requestOwner(r)))
}

func (h workspacesHandler) remove(w http.ResponseWriter, r *http.Request) {
	workspace, access, ok := h.readable(w, r)
	if !ok || !permitted(w, access, accessOwner) {
		return
	}
	if err := h.hub.commit(workspace.Id, func() (*syncMessage, error) {
		return closedMessage(syncClosedDeleted), h.store.Delete(workspace.Id, requestOwner(r))
	}); err != nil {
		writeWorkspaceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// shareTarget は、共有の操作が指すワークスペースを、所有者の要求であることを確かめて返す。
func (h workspacesHandler) shareTarget(w http.ResponseWriter, r *http.Request) (pipeline.Workspace, bool) {
	if h.access == nil {
		writeError(w, http.StatusBadRequest, core.ApiError{
			Code: core.ApiErrorCodeInvalidRequest, Message: "sharing requires accounts",
		})
		return pipeline.Workspace{}, false
	}
	workspace, access, ok := h.readable(w, r)
	if !ok || !permitted(w, access, accessOwner) {
		return pipeline.Workspace{}, false
	}
	return workspace, true
}

func (h workspacesHandler) share(w http.ResponseWriter, r *http.Request) {
	workspace, ok := h.shareTarget(w, r)
	if !ok {
		return
	}
	invalid := func(message string) {
		writeError(w, http.StatusBadRequest, core.ApiError{Code: core.ApiErrorCodeInvalidRequest, Message: message})
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, requestBodyLimit))
	decoder.DisallowUnknownFields()
	var body workspaceShareRequestBody
	if err := decoder.Decode(&body); err != nil {
		invalid("the request body is not a readable share")
		return
	}
	login, owner := r.PathValue("login"), requestOwner(r)
	if login == owner || login == localOwner {
		invalid("the workspace cannot be shared with its owner or the local owner")
		return
	}
	if _, member := h.access.Role(login); !member {
		invalid("the login has no role in the investigation")
		return
	}
	share, err := h.store.Share(workspace.Id, owner, login, body.Access)
	if err != nil {
		writeWorkspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, shareItemOf(share))
}

func (h workspacesHandler) unshare(w http.ResponseWriter, r *http.Request) {
	workspace, ok := h.shareTarget(w, r)
	if !ok {
		return
	}
	// 取り消しの後の presence の配信で、各接続が読めるかを確かめ直し、共有を失った接続を閉じる。
	if err := h.hub.commit(workspace.Id, func() (*syncMessage, error) {
		if err := h.store.Unshare(workspace.Id, requestOwner(r), r.PathValue("login")); err != nil {
			return nil, err
		}
		presence := h.hub.presenceLocked(workspace.Id)
		return &presence, nil
	}); err != nil {
		writeWorkspaceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// decodeWorkspaceBody は要求の本文を、大きさを限らずに読む。state は JSON の object でなければならない。
// 名前を持たない作成は、名前を空の文字列として返す。
func decodeWorkspaceBody(r *http.Request, replacing bool) (workspaceRequestBody, *core.ApiError) {
	invalid := func(message string) *core.ApiError {
		return &core.ApiError{Code: core.ApiErrorCodeInvalidRequest, Message: message}
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var body workspaceRequestBody
	if err := decoder.Decode(&body); err != nil {
		return workspaceRequestBody{}, invalid("the request body is not a readable workspace")
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return workspaceRequestBody{}, invalid("the request body carries a value after its JSON object")
	}
	if !strings.HasPrefix(string(bytes.TrimSpace(body.State)), "{") {
		return workspaceRequestBody{}, invalid("state must be a JSON object")
	}
	if replacing != (body.BaseRevision != nil) {
		return workspaceRequestBody{}, invalid("baseRevision is required to replace a workspace and only then")
	}
	return body, nil
}

func writeWorkspaceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errWorkspaceNotEditable):
		writeError(w, http.StatusForbidden, core.ApiError{
			Code: core.ApiErrorCodePermissionDenied, Message: err.Error(),
		})
	case errors.Is(err, pipeline.ErrWorkspaceRevisionConflict):
		writeError(w, http.StatusConflict, core.ApiError{
			Code: core.ApiErrorCodeWorkspaceChanged, Message: "the workspace changed after the base revision",
		})
	case errors.Is(err, pipeline.ErrWorkspaceNotFound):
		writeError(w, http.StatusNotFound, core.ApiError{
			Code: core.ApiErrorCodeRecordNotFound, Message: "no workspace matches the identifier",
		})
	case errors.Is(err, pipeline.ErrWorkspaceShareNotFound):
		writeError(w, http.StatusNotFound, core.ApiError{
			Code: core.ApiErrorCodeRecordNotFound, Message: "the workspace is not shared with the login",
		})
	case errors.Is(err, pipeline.ErrWorkspaceInvalid):
		writeError(w, http.StatusBadRequest, core.ApiError{Code: core.ApiErrorCodeInvalidRequest, Message: err.Error()})
	default:
		slog.Error("storing the workspace failed", "error", err)
		writeError(w, http.StatusInternalServerError, core.ApiError{
			Code: core.ApiErrorCodeInternalError, Message: "storing the workspace failed",
		})
	}
}

func summaryOf(workspace pipeline.Workspace, login string) workspaceSummary {
	access, _ := accessOf(workspace, login)
	return workspaceSummary{
		Id: workspace.Id, Name: workspace.Name, Owner: workspace.Owner, Revision: workspace.Revision,
		UpdatedAt: workspace.UpdatedAt.UTC().Format(time.RFC3339Nano), UpdatedBy: workspace.UpdatedBy, Access: access,
	}
}

func itemOf(workspace pipeline.Workspace, login string) workspaceItem {
	item := workspaceItem{workspaceSummary: summaryOf(workspace, login), State: workspace.State}
	if item.Access == accessOwner {
		shares := make([]workspaceShareItem, len(workspace.Shares))
		for index, share := range workspace.Shares {
			shares[index] = shareItemOf(share)
		}
		item.Shares = &shares
	}
	return item
}

func shareItemOf(share pipeline.WorkspaceShare) workspaceShareItem {
	return workspaceShareItem{Login: share.Login, Access: share.Access, GrantedBy: share.GrantedBy,
		GrantedAt: share.GrantedAt.UTC().Format(time.RFC3339Nano)}
}
