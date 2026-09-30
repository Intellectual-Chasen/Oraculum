package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/output"
)

// セッションの操作の path と cookie の名前。
const (
	sessionPath       = "/api/v0/session"
	sessionMePath     = "/api/v0/session/me"
	sessionCookieName = "oraculum_session"
)

// ログインの失敗の上限。1 つのログイン名に loginFailureWindow の間で loginFailureLimit 回失敗すると、
// その期間を過ぎるまで同じログイン名のログインを退ける。
const (
	loginFailureLimit  = 5
	loginFailureWindow = 15 * time.Minute
	// loginFailureLogins を超えるログイン名の失敗を記録したら、loginFailureWindow の期間の外の記録を捨てる。
	loginFailureLogins = 10_000
	// maxPasswordBytes は、ログインで受け付けるパスワードの長さの上限である。
	maxPasswordBytes = 1024
)

// loginPattern はアカウントのログイン名の文字列の条件である。アカウントの file も同じ条件で検査する。
var loginPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

// Account は、server がセッションから特定した利用者である。
type Account struct {
	// Login は利用者の安定した識別子である。分析者の判断の著者に書く。
	Login string
	// DisplayName は画面に出す名前である。
	DisplayName string
}

// AccountSession はログインで作ったセッションである。
type AccountSession struct {
	// Token は cookie で browser に渡す secret である。
	Token     string
	Account   Account
	ExpiresAt time.Time
}

// Accounts は利用者のログインとセッションを確かめる port である。
type Accounts interface {
	// Login は、ログイン名とパスワードの組を受け付けないとき ErrLoginRejected を返す。
	Login(ctx context.Context, login, password string) (AccountSession, error)
	// SessionAccount は、token のセッションが無い、切れた、失効した、または利用者を無効にしたとき
	// ErrSessionInvalid を返す。
	SessionAccount(ctx context.Context, token string) (Account, error)
	// Logout は token のセッションを失効させる。
	Logout(ctx context.Context, token string) error
	// Account はログイン名のアカウントを返す。無いときは ErrAccountNotFound を返す。無効にした
	// アカウントも返す。
	Account(ctx context.Context, login string) (Account, error)
}

var (
	// ErrAccountNotFound は、ログイン名のアカウントが無いことを表す。
	ErrAccountNotFound = errors.New("no account has the login")
	// ErrLoginRejected はログイン名とパスワードの組を受け付けないことを表す。
	ErrLoginRejected = errors.New("the login and the password are rejected")
	// ErrSessionInvalid は、セッションが使えないことを表す。
	ErrSessionInvalid = errors.New("the session is not valid")
)

// 認証の方式。sessionResponse の authentication の値である。
const (
	// authenticationNone は、アカウントを持たない起動である。利用者は分析者の名前を自分で入力する。
	authenticationNone = "none"
	// authenticationAccount は、アカウントでログインした利用者の要求である。
	authenticationAccount = "account"
)

// sessionResponse はセッションの操作の応答である。**本型が項目の定義元である。**
type sessionResponse struct {
	// Authentication は authenticationNone か authenticationAccount である。
	Authentication string `json:"authentication"`
	// Login と DisplayName は、authentication が account のときだけ持つ。
	Login       string `json:"login,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
}

// loginRequestBody はログインの要求の本文である。
type loginRequestBody struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

// NewSessionHandler は、ログイン・ログアウト・ログイン中の利用者の操作に答え、`/api/` の要求に
// セッションを求めてから next へ渡す。
//
// accounts が nil の起動は、アカウントを持たない。`/api/` の要求をそのまま next へ渡し、
// ログイン中の利用者の操作は authentication が none の応答を返す。
//
// **セッションの操作は、調査の段階を終える前も答える。** 段階を始める操作もセッションを要る。
func NewSessionHandler(next http.Handler, accounts Accounts) http.Handler {
	return &sessionHandler{
		next: next, accounts: accounts, now: time.Now, failures: map[string][]time.Time{},
		inFlight: map[string]int{}, hashing: make(chan struct{}, runtime.GOMAXPROCS(0)),
	}
}

type sessionHandler struct {
	next     http.Handler
	accounts Accounts
	now      func() time.Time

	// ponytail: 失敗の記録は process のメモリだけに置く。再起動で数え直す。複数の server で
	// 同じアカウントを使うときに共有の記録へ移す。
	mu       sync.Mutex
	failures map[string][]time.Time
	// inFlight は、ログイン名ごとの実行中のログインの数である。上限の判定は失敗と合わせて数える。
	inFlight map[string]int
	// hashing は、パスワードの hash を同時に求めるログインの数を CPU の数までに抑える。
	hashing chan struct{}
}

func (h *sessionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == sessionPath && r.Method == http.MethodPost:
		h.login(w, r)
	case r.URL.Path == sessionPath && r.Method == http.MethodDelete:
		h.logout(w, r)
	case r.URL.Path == sessionMePath && r.Method == http.MethodGet:
		h.me(w, r)
	case r.URL.Path == sessionPath || r.URL.Path == sessionMePath:
		w.Header().Set("Allow", map[string]string{sessionPath: "POST, DELETE", sessionMePath: "GET"}[r.URL.Path])
		writeError(w, http.StatusMethodNotAllowed, core.ApiError{
			Code: core.ApiErrorCodeInvalidRequest, Message: "the method is not allowed for the session",
		})
	case h.accounts == nil || !strings.HasPrefix(r.URL.Path, "/api/"):
		h.next.ServeHTTP(w, r)
	default:
		account, ok := h.authenticate(w, r)
		if !ok {
			return
		}
		h.next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), accountKey{}, account)))
	}
}

// accountKey は、要求の context に置いた Account の鍵である。
type accountKey struct{}

// requestAccount は、セッションが特定した要求の利用者を返す。アカウントを持たない起動では ok が偽である。
func requestAccount(r *http.Request) (Account, bool) {
	account, ok := r.Context().Value(accountKey{}).(Account)
	return account, ok
}

// requestAuthor は、分析者の判断に書く著者を決める。
//
// **ログインした利用者の要求は、本文の著者を受け付けない。** 著者はセッションのログイン名である。
// アカウントを持たない起動は、本文の著者をそのまま使う。
func requestAuthor(r *http.Request, supplied string) (string, *core.ApiError) {
	account, ok := requestAccount(r)
	if !ok {
		return supplied, nil
	}
	if supplied != "" {
		return "", &core.ApiError{
			Code: core.ApiErrorCodeInvalidRequest, Message: "the author is the signed-in account; omit author",
		}
	}
	return account.Login, nil
}

// authenticate は cookie のセッションの利用者を返す。セッションが使えないときは 401 を書き、ok を偽にする。
func (h *sessionHandler) authenticate(w http.ResponseWriter, r *http.Request) (Account, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil {
		account, err := h.accounts.SessionAccount(r.Context(), cookie.Value)
		if err == nil {
			return account, true
		}
		if !errors.Is(err, ErrSessionInvalid) {
			slog.Error("reading the session failed", "error", err)
			writeError(w, http.StatusInternalServerError, core.ApiError{
				Code: core.ApiErrorCodeInternalError, Message: "reading the session failed",
			})
			return Account{}, false
		}
	}
	writeError(w, http.StatusUnauthorized, core.ApiError{
		Code: core.ApiErrorCodeAuthenticationRequired, Message: "the request needs a valid session",
	})
	return Account{}, false
}

func (h *sessionHandler) me(w http.ResponseWriter, r *http.Request) {
	if h.accounts == nil {
		writeJSON(w, http.StatusOK, sessionResponse{Authentication: authenticationNone})
		return
	}
	account, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, sessionResponse{
		Authentication: authenticationAccount, Login: account.Login, DisplayName: account.DisplayName,
	})
}

func (h *sessionHandler) login(w http.ResponseWriter, r *http.Request) {
	if h.accounts == nil {
		writeError(w, http.StatusBadRequest, core.ApiError{
			Code: core.ApiErrorCodeInvalidRequest, Message: "the server runs without accounts",
		})
		return
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, requestBodyLimit))
	decoder.DisallowUnknownFields()
	var body loginRequestBody
	if err := decoder.Decode(&body); err != nil || body.Login == "" || body.Password == "" {
		writeError(w, http.StatusBadRequest, core.ApiError{
			Code: core.ApiErrorCodeInvalidRequest, Message: "the request body is not a readable login",
		})
		return
	}
	rejected := core.ApiError{Code: core.ApiErrorCodeLoginRejected, Message: "the login and the password are rejected"}
	// アカウントのログイン名になり得ない文字列は、hash を求めず、失敗の記録にも入れずに退ける。
	// そのようなアカウントは無いので、応答の時間からアカウントの有無は分からない。
	if !loginPattern.MatchString(body.Login) || len(body.Password) > maxPasswordBytes {
		writeError(w, http.StatusUnauthorized, rejected)
		return
	}
	limited := func(message, retryAfter string) {
		// #nosec G706 -- output.Sanitize が制御文字と改行を除いてから log へ渡す。
		slog.Warn("refused a login", "reason", message, "login", output.Sanitize(body.Login))
		w.Header().Set("Retry-After", retryAfter)
		writeError(w, http.StatusTooManyRequests, core.ApiError{Code: core.ApiErrorCodeLoginRateLimited, Message: message})
	}
	select {
	case h.hashing <- struct{}{}:
		defer func() { <-h.hashing }()
	default:
		limited("too many logins are running", "1")
		return
	}
	release, ok := h.reserve(body.Login)
	if !ok {
		limited("too many failed logins for the login", "900")
		return
	}
	failed := false
	defer func() { release(failed) }()
	session, err := h.accounts.Login(r.Context(), body.Login, body.Password)
	failed = errors.Is(err, ErrLoginRejected)
	if errors.Is(err, ErrLoginRejected) {
		// #nosec G706 -- output.Sanitize が制御文字と改行を除いてから log へ渡す。
		slog.Warn("rejected a login", "login", output.Sanitize(body.Login))
		writeError(w, http.StatusUnauthorized, rejected)
		return
	}
	if err != nil {
		slog.Error("logging in failed", "error", err)
		writeError(w, http.StatusInternalServerError, core.ApiError{
			Code: core.ApiErrorCodeInternalError, Message: "logging in failed",
		})
		return
	}
	// 同じ browser が持っていた前のセッションは使えなくする。
	if previous, err := r.Cookie(sessionCookieName); err == nil {
		if err := h.accounts.Logout(r.Context(), previous.Value); err != nil {
			slog.Error("ending the previous session failed", "error", err)
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: session.Token, Path: "/", Expires: session.ExpiresAt,
		HttpOnly: true, SameSite: http.SameSiteStrictMode,
	})
	writeJSON(w, http.StatusOK, sessionResponse{
		Authentication: authenticationAccount, Login: session.Account.Login,
		DisplayName: session.Account.DisplayName,
	})
}

func (h *sessionHandler) logout(w http.ResponseWriter, r *http.Request) {
	if h.accounts == nil {
		writeError(w, http.StatusBadRequest, core.ApiError{
			Code: core.ApiErrorCodeInvalidRequest, Message: "the server runs without accounts",
		})
		return
	}
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		if err := h.accounts.Logout(r.Context(), cookie.Value); err != nil {
			slog.Error("logging out failed", "error", err)
			writeError(w, http.StatusInternalServerError, core.ApiError{
				Code: core.ApiErrorCodeInternalError, Message: "logging out failed",
			})
			return
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

// reserve は、ログイン名の直近の失敗と実行中のログインの数が上限に達していなければ、ログイン 1 回の
// 枠を確保する。ok が偽のときは上限に達している。
//
// **判定と確保を 1 回の lock で行う。** 同じログイン名へ同時に送った要求が、hash を求める間に
// 揃って判定を通らない。release は確保した枠を戻し、failed が真なら失敗として記録する。
func (h *sessionHandler) reserve(login string) (release func(failed bool), ok bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.recentFailures(login))+h.inFlight[login] >= loginFailureLimit {
		return nil, false
	}
	h.inFlight[login]++
	return func(failed bool) {
		h.mu.Lock()
		defer h.mu.Unlock()
		if h.inFlight[login]--; h.inFlight[login] == 0 {
			delete(h.inFlight, login)
		}
		if failed {
			h.recordFailure(login)
		}
	}, true
}

// recordFailure は失敗を 1 件記録する。呼び出し側が mu を持つ。
func (h *sessionHandler) recordFailure(login string) {
	h.failures[login] = append(h.recentFailures(login), h.now())
	// 存在しないログイン名を並べた要求で、記録が際限なく増えない。
	if len(h.failures) > loginFailureLogins {
		for other := range h.failures {
			h.recentFailures(other)
		}
	}
}

// recentFailures は loginFailureWindow の期間の外の失敗を捨てた記録を返す。呼び出し側が mu を持つ。
func (h *sessionHandler) recentFailures(login string) []time.Time {
	cutoff := h.now().Add(-loginFailureWindow)
	recent := h.failures[login][:0]
	for _, at := range h.failures[login] {
		if at.After(cutoff) {
			recent = append(recent, at)
		}
	}
	if len(recent) == 0 {
		delete(h.failures, login)
		return nil
	}
	h.failures[login] = recent
	return recent
}
