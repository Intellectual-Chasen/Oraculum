package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/output"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// 役割の管理の path。
const (
	membersPath   = "/api/v0/members"
	memberMePath  = "/api/v0/members/me"
	memberPattern = "/api/v0/members/{login}"
)

// Access は調査に参加する利用者の役割を保つ port である。本番は pipeline.AccessStore が実装する。
type Access interface {
	Role(login string) (pipeline.Role, bool)
	Member(login string) (pipeline.Member, bool)
	List() []pipeline.Member
	Grant(actor, login string, role pipeline.Role) (pipeline.Member, error)
	Revoke(actor, login string) error
}

// RequiredRole は、method と path の要求に要る役割を返す。
//
// **宣言の無い経路は、より強い役割を要る側に倒れる。** 読み取り (GET・HEAD) は閲覧者、それ以外は
// 編集者を要る。利用者の役割の管理は管理者を要る。自分の役割を読む操作は役割を要らない。
// ワークスペースは調査の内容を変えないので、閲覧者も保存できる。所有者の確認はワークスペースの
// handler が行う。基準の directory の一覧は、読み込みを始める編集者を要る。
func RequiredRole(method, path string) (pipeline.Role, bool) {
	switch {
	case path == memberMePath && (method == http.MethodGet || method == http.MethodHead):
		return "", false
	case path == strings.TrimPrefix(sourceFilesPattern, "GET "):
		return pipeline.RoleEditor, true
	case path == membersPath || strings.HasPrefix(path, membersPath+"/"):
		return pipeline.RoleAdmin, true
	case path == workspacesPath || strings.HasPrefix(path, workspacesPath+"/"):
		return pipeline.RoleViewer, true
	case method == http.MethodGet || method == http.MethodHead:
		return pipeline.RoleViewer, true
	default:
		return pipeline.RoleEditor, true
	}
}

// memberItem は役割を持つ利用者 1 人である。**本型が項目の定義元である。**
type memberItem struct {
	Login       string        `json:"login"`
	DisplayName string        `json:"displayName"`
	Role        pipeline.Role `json:"role"`
	GrantedBy   string        `json:"grantedBy"`
	GrantedAt   string        `json:"grantedAt"`
}

// membersResponse は役割を持つ利用者の一覧である。
type membersResponse struct {
	Members []memberItem `json:"members"`
}

// memberRequestBody は役割を与える要求の本文である。
type memberRequestBody struct {
	Role pipeline.Role `json:"role"`
}

// NewAccessHandler は、ログインした利用者の役割で `/api/` の要求を検査してから next へ渡し、
// 役割の管理の操作に答える。NewSessionHandler の内側に置く。
//
// 役割を持たない利用者には、調査の存在を返さない (investigation_not_found)。役割の足りない
// 要求は permission_denied で退ける。アカウントを持たない起動の要求 (利用者を持たない要求) は
// そのまま next へ渡す。
func NewAccessHandler(next http.Handler, access Access, accounts Accounts) http.Handler {
	handler := &accessHandler{next: next, access: access, accounts: accounts}
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+memberMePath, handler.me)
	mux.HandleFunc("GET "+membersPath, handler.list)
	mux.HandleFunc("PUT "+memberPattern, handler.grant)
	mux.HandleFunc("DELETE "+memberPattern, handler.revoke)
	mux.Handle("/", next)
	handler.mux = mux
	return handler
}

type accessHandler struct {
	next     http.Handler
	mux      *http.ServeMux
	access   Access
	accounts Accounts
}

func (h *accessHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	account, signedIn := requestAccount(r)
	if !signedIn || !strings.HasPrefix(r.URL.Path, "/api/") {
		h.next.ServeHTTP(w, r)
		return
	}
	if required, needed := RequiredRole(r.Method, r.URL.Path); needed {
		role, member := h.access.Role(account.Login)
		if !member {
			h.deny(w, r, account, http.StatusNotFound, core.ApiErrorCodeInvestigationNotFound,
				"the investigation is not available to the account")
			return
		}
		if !role.Includes(required) {
			h.deny(w, r, account, http.StatusForbidden, core.ApiErrorCodePermissionDenied,
				"the role of the account does not allow the operation")
			return
		}
	}
	h.mux.ServeHTTP(w, r)
}

func (h *accessHandler) deny(
	w http.ResponseWriter, r *http.Request, account Account, status int, code core.ApiErrorCode, message string,
) {
	// #nosec G706 -- output.Sanitize が制御文字と改行を除いてから log へ渡す。
	slog.Warn("refused a request over the role", "login", output.Sanitize(account.Login),
		"method", output.Sanitize(r.Method), "path", output.Sanitize(r.URL.Path), "code", string(code))
	writeError(w, status, core.ApiError{Code: code, Message: message})
}

// me はログインした利用者の役割を返す。役割を持たない利用者には investigation_not_found を返す。
func (h *accessHandler) me(w http.ResponseWriter, r *http.Request) {
	account, _ := requestAccount(r)
	member, ok := h.access.Member(account.Login)
	if !ok {
		h.deny(w, r, account, http.StatusNotFound, core.ApiErrorCodeInvestigationNotFound,
			"the investigation is not available to the account")
		return
	}
	writeJSON(w, http.StatusOK, h.item(r, member))
}

func (h *accessHandler) list(w http.ResponseWriter, r *http.Request) {
	members := h.access.List()
	response := membersResponse{Members: make([]memberItem, 0, len(members))}
	for _, member := range members {
		response.Members = append(response.Members, h.item(r, member))
	}
	writeJSON(w, http.StatusOK, response)
}

// item は役割に、アカウントの表示名を添える。無効にしたアカウントも役割は残るので、表示名が
// 読めなければログイン名を表示名にする。
func (h *accessHandler) item(r *http.Request, member pipeline.Member) memberItem {
	displayName := member.Login
	if account, err := h.accounts.Account(r.Context(), member.Login); err == nil {
		displayName = account.DisplayName
	}
	return memberItem{
		Login: member.Login, DisplayName: displayName, Role: member.Role, GrantedBy: member.GrantedBy,
		GrantedAt: member.GrantedAt.UTC().Format(time.RFC3339),
	}
}

func (h *accessHandler) grant(w http.ResponseWriter, r *http.Request) {
	actor, _ := requestAccount(r)
	decoder := json.NewDecoder(io.LimitReader(r.Body, requestBodyLimit))
	decoder.DisallowUnknownFields()
	var body memberRequestBody
	if err := decoder.Decode(&body); err != nil || !body.Role.IsKnown() {
		writeError(w, http.StatusBadRequest, core.ApiError{
			Code: core.ApiErrorCodeInvalidRequest, Message: "the request body is not a readable role",
		})
		return
	}
	login := r.PathValue("login")
	if _, err := h.accounts.Account(r.Context(), login); err != nil {
		h.writeAccountError(w, err)
		return
	}
	member, err := h.access.Grant(actor.Login, login, body.Role)
	if err != nil {
		h.writeAccessError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, h.item(r, member))
}

func (h *accessHandler) revoke(w http.ResponseWriter, r *http.Request) {
	actor, _ := requestAccount(r)
	if err := h.access.Revoke(actor.Login, r.PathValue("login")); err != nil {
		h.writeAccessError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *accessHandler) writeAccountError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrAccountNotFound) {
		writeError(w, http.StatusBadRequest, core.ApiError{
			Code: core.ApiErrorCodeInvalidRequest, Message: "no account has the login",
		})
		return
	}
	slog.Error("reading the account failed", "error", err)
	writeError(w, http.StatusInternalServerError, core.ApiError{
		Code: core.ApiErrorCodeInternalError, Message: "reading the account failed",
	})
}

func (h *accessHandler) writeAccessError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, pipeline.ErrMemberNotFound):
		writeError(w, http.StatusNotFound, core.ApiError{
			Code: core.ApiErrorCodeRecordNotFound, Message: "the login has no role in the investigation",
		})
	case errors.Is(err, pipeline.ErrLastAdmin):
		writeError(w, http.StatusBadRequest, core.ApiError{
			Code: core.ApiErrorCodeInvalidRequest, Message: "the investigation keeps at least one admin",
		})
	default:
		slog.Error("changing the role failed", "error", err)
		writeError(w, http.StatusInternalServerError, core.ApiError{
			Code: core.ApiErrorCodeInternalError, Message: "changing the role failed",
		})
	}
}
