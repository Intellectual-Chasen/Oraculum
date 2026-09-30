package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/api"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// fakeAccounts は、パスワードと token を固定の表で確かめる。
type fakeAccounts struct {
	passwords map[string]string
	sessions  map[string]api.Account
	loggedOut []string
	logins    int
	// failure は、SessionAccount と Logout が返す保存先の失敗である。
	failure error
}

func newFakeAccounts() *fakeAccounts {
	return &fakeAccounts{
		passwords: map[string]string{"alice": "alice-password", "bob": "bob-password"},
		sessions:  map[string]api.Account{},
	}
}

func (f *fakeAccounts) Login(_ context.Context, login, password string) (api.AccountSession, error) {
	f.logins++
	if f.passwords[login] == "" || f.passwords[login] != password {
		return api.AccountSession{}, api.ErrLoginRejected
	}
	token := "token-of-" + login
	account := api.Account{Login: login, DisplayName: strings.ToUpper(login)}
	f.sessions[token] = account
	return api.AccountSession{Token: token, Account: account, ExpiresAt: time.Now().Add(time.Hour)}, nil
}

func (f *fakeAccounts) SessionAccount(_ context.Context, token string) (api.Account, error) {
	if f.failure != nil {
		return api.Account{}, f.failure
	}
	account, ok := f.sessions[token]
	if !ok {
		return api.Account{}, api.ErrSessionInvalid
	}
	return account, nil
}

func (f *fakeAccounts) Logout(_ context.Context, token string) error {
	if f.failure != nil {
		return f.failure
	}
	delete(f.sessions, token)
	f.loggedOut = append(f.loggedOut, token)
	return nil
}

func (f *fakeAccounts) Account(_ context.Context, login string) (api.Account, error) {
	if _, ok := f.passwords[login]; !ok {
		return api.Account{}, api.ErrAccountNotFound
	}
	return api.Account{Login: login, DisplayName: strings.ToUpper(login)}, nil
}

// sessionFixture は、届いた要求の利用者を答える next を持つセッションの wrapper である。
func sessionFixture(accounts api.Accounts) (http.Handler, *[]string) {
	var reached []string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = append(reached, r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	})
	if accounts == nil {
		return api.NewSessionHandler(next, nil), &reached
	}
	return api.NewSessionHandler(next, accounts), &reached
}

func serveSession(handler http.Handler, method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if cookie != nil {
		request.AddCookie(cookie)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func decodeApiError(t *testing.T, recorder *httptest.ResponseRecorder) core.ApiError {
	t.Helper()
	var apiError core.ApiError
	if err := json.Unmarshal(recorder.Body.Bytes(), &apiError); err != nil {
		t.Fatalf("body=%s err=%v", recorder.Body, err)
	}
	return apiError
}

func login(t *testing.T, handler http.Handler, loginName, password string) *http.Cookie {
	t.Helper()
	recorder := serveSession(handler, http.MethodPost, "/api/v0/session",
		`{"login":"`+loginName+`","password":"`+password+`"}`, nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", recorder.Code, recorder.Body)
	}
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies=%v", cookies)
	}
	cookie := cookies[0]
	if cookie.Name != "oraculum_session" || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode ||
		cookie.Path != "/" {
		t.Fatalf("cookie=%+v", cookie)
	}
	return cookie
}

func TestSessionLoginIdentifiesEachAccount(t *testing.T) {
	handler, _ := sessionFixture(newFakeAccounts())
	for _, name := range []string{"alice", "bob"} {
		cookie := login(t, handler, name, name+"-password")
		recorder := serveSession(handler, http.MethodGet, "/api/v0/session/me", "", cookie)
		want := `{"authentication":"account","login":"` + name + `","displayName":"` + strings.ToUpper(name) + `"}`
		if recorder.Code != http.StatusOK || strings.TrimSpace(recorder.Body.String()) != want {
			t.Fatalf("status=%d body=%s want=%s", recorder.Code, recorder.Body, want)
		}
	}
}

func TestSessionRequiresAValidSessionForTheApi(t *testing.T) {
	accounts := newFakeAccounts()
	handler, reached := sessionFixture(accounts)
	for name, cookie := range map[string]*http.Cookie{
		"no cookie":      nil,
		"unknown token":  {Name: "oraculum_session", Value: "forged"},
		"another cookie": {Name: "session", Value: "token-of-alice"},
	} {
		t.Run(name, func(t *testing.T) {
			for _, path := range []string{"/api/v0/stages", "/api/v0/graph", "/api/v0/session/me"} {
				recorder := serveSession(handler, http.MethodGet, path, "", cookie)
				if recorder.Code != http.StatusUnauthorized ||
					decodeApiError(t, recorder).Code != core.ApiErrorCodeAuthenticationRequired {
					t.Fatalf("%s status=%d body=%s", path, recorder.Code, recorder.Body)
				}
			}
		})
	}
	if len(*reached) != 0 {
		t.Fatalf("requests without a session reached the api: %v", *reached)
	}
	// 画面の file は、ログインの前に読める。
	if recorder := serveSession(handler, http.MethodGet, "/index.html", "", nil); recorder.Code != http.StatusNoContent {
		t.Fatalf("screen status=%d", recorder.Code)
	}
	cookie := login(t, handler, "alice", "alice-password")
	if recorder := serveSession(handler, http.MethodGet, "/api/v0/stages", "", cookie); recorder.Code != http.StatusNoContent {
		t.Fatalf("signed-in status=%d", recorder.Code)
	}
}

func TestSessionLogoutEndsTheSession(t *testing.T) {
	accounts := newFakeAccounts()
	handler, _ := sessionFixture(accounts)
	cookie := login(t, handler, "alice", "alice-password")
	recorder := serveSession(handler, http.MethodDelete, "/api/v0/session", "", cookie)
	if recorder.Code != http.StatusNoContent || len(accounts.loggedOut) != 1 {
		t.Fatalf("status=%d loggedOut=%v", recorder.Code, accounts.loggedOut)
	}
	if cleared := recorder.Result().Cookies(); len(cleared) != 1 || cleared[0].MaxAge >= 0 {
		t.Fatalf("cookies=%v", cleared)
	}
	if recorder := serveSession(handler, http.MethodGet, "/api/v0/graph", "", cookie); recorder.Code != http.StatusUnauthorized {
		t.Fatalf("after logout status=%d", recorder.Code)
	}
}

func TestSessionLoginRejectsWrongPasswordsAndLimitsFailures(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	handler, _ := sessionFixture(newFakeAccounts())
	for range 5 {
		recorder := serveSession(handler, http.MethodPost, "/api/v0/session",
			`{"login":"alice","password":"secret-guess"}`, nil)
		if recorder.Code != http.StatusUnauthorized || decodeApiError(t, recorder).Code != core.ApiErrorCodeLoginRejected {
			t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body)
		}
	}
	// 上限に達した後は、正しいパスワードも失敗を数える期間を過ぎるまで退ける。
	recorder := serveSession(handler, http.MethodPost, "/api/v0/session",
		`{"login":"alice","password":"alice-password"}`, nil)
	if recorder.Code != http.StatusTooManyRequests || decodeApiError(t, recorder).Code != core.ApiErrorCodeLoginRateLimited ||
		recorder.Header().Get("Retry-After") == "" {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body)
	}
	login(t, handler, "bob", "bob-password")
	if strings.Contains(logs.String(), "secret-guess") || strings.Contains(logs.String(), "token-of-") {
		t.Fatalf("the log carries a credential: %s", logs.String())
	}
	if !strings.Contains(logs.String(), "login=alice") {
		t.Fatalf("the log lacks the rejected login: %s", logs.String())
	}
}

func TestSessionRejectsUnreadableLoginsAndWrongMethods(t *testing.T) {
	handler, _ := sessionFixture(newFakeAccounts())
	for _, body := range []string{``, `{}`, `{"login":"alice"}`, `{"login":"alice","password":"x","extra":1}`} {
		if recorder := serveSession(handler, http.MethodPost, "/api/v0/session", body, nil); recorder.Code != http.StatusBadRequest {
			t.Fatalf("body=%q status=%d", body, recorder.Code)
		}
	}
	if recorder := serveSession(handler, http.MethodPut, "/api/v0/session", "", nil); recorder.Code != http.StatusMethodNotAllowed ||
		recorder.Header().Get("Allow") != "POST, DELETE" {
		t.Fatalf("status=%d allow=%q", recorder.Code, recorder.Header().Get("Allow"))
	}
}

// アカウントのログイン名になり得ない文字列と長すぎるパスワードは、hash を求めずに退け、失敗に数えない。
func TestSessionRejectsImpossibleLoginsWithoutHashing(t *testing.T) {
	accounts := newFakeAccounts()
	handler, _ := sessionFixture(accounts)
	for _, body := range []string{
		`{"login":"` + strings.Repeat("a", 65) + `","password":"x"}`,
		`{"login":"Alice","password":"x"}`,
		`{"login":"alice","password":"` + strings.Repeat("p", 1025) + `"}`,
	} {
		for range 10 {
			recorder := serveSession(handler, http.MethodPost, "/api/v0/session", body, nil)
			if recorder.Code != http.StatusUnauthorized || decodeApiError(t, recorder).Code != core.ApiErrorCodeLoginRejected {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body)
			}
		}
	}
	if accounts.logins != 0 {
		t.Fatalf("hashed %d impossible logins", accounts.logins)
	}
	login(t, handler, "alice", "alice-password")
}

// ログインは、同じ browser が持っていた前のセッションを失効させる。
func TestSessionLoginEndsThePreviousSession(t *testing.T) {
	accounts := newFakeAccounts()
	handler, _ := sessionFixture(accounts)
	previous := login(t, handler, "alice", "alice-password")
	recorder := serveSession(handler, http.MethodPost, "/api/v0/session",
		`{"login":"bob","password":"bob-password"}`, previous)
	if recorder.Code != http.StatusOK || len(accounts.loggedOut) != 1 || accounts.loggedOut[0] != previous.Value {
		t.Fatalf("status=%d loggedOut=%v", recorder.Code, accounts.loggedOut)
	}
}

// 保存先の失敗は、セッションの不備と分けて 500 で返す。
func TestSessionReportsStoreFailuresAsInternal(t *testing.T) {
	accounts := newFakeAccounts()
	handler, _ := sessionFixture(accounts)
	cookie := login(t, handler, "alice", "alice-password")
	accounts.failure = errors.New("disk failure")
	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		path := "/api/v0/graph"
		if method == http.MethodDelete {
			path = "/api/v0/session"
		}
		recorder := serveSession(handler, method, path, "", cookie)
		if recorder.Code != http.StatusInternalServerError || strings.Contains(recorder.Body.String(), "disk") {
			t.Fatalf("%s status=%d body=%s", method, recorder.Code, recorder.Body)
		}
	}
}

// アカウントを持たない起動は、要求をそのまま渡し、ログインの操作を受け付けない。
func TestSessionWithoutAccountsPassesTheApiThrough(t *testing.T) {
	handler, reached := sessionFixture(nil)
	if recorder := serveSession(handler, http.MethodGet, "/api/v0/graph", "", nil); recorder.Code != http.StatusNoContent {
		t.Fatalf("status=%d", recorder.Code)
	}
	recorder := serveSession(handler, http.MethodGet, "/api/v0/session/me", "", nil)
	if strings.TrimSpace(recorder.Body.String()) != `{"authentication":"none"}` {
		t.Fatalf("me=%s", recorder.Body)
	}
	if recorder := serveSession(handler, http.MethodPost, "/api/v0/session", `{"login":"a","password":"b"}`, nil); recorder.Code != http.StatusBadRequest {
		t.Fatalf("login status=%d", recorder.Code)
	}
	if len(*reached) != 1 {
		t.Fatalf("reached=%v", *reached)
	}
}
