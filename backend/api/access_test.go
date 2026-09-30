package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/api"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// accessFixture は、alice (admin)、bob (viewer)、carol (役割なし) のアカウントを持つ server を組む。
// next は届いた要求を 204 で答える。
type accessFixture struct {
	handler http.Handler
	access  *pipeline.AccessStore
	cookies map[string]*http.Cookie
}

func newAccessFixture(t *testing.T) accessFixture {
	t.Helper()
	accounts := newFakeAccounts()
	accounts.passwords["carol"] = "carol-password"
	access := pipeline.NewAccessStore([]pipeline.Member{
		{Login: "alice", Role: pipeline.RoleAdmin}, {Login: "bob", Role: pipeline.RoleViewer},
	})
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	handler := api.NewSessionHandler(api.NewAccessHandler(next, access, accounts), accounts)
	fixture := accessFixture{handler: handler, access: access, cookies: map[string]*http.Cookie{}}
	for _, name := range []string{"alice", "bob", "carol"} {
		fixture.cookies[name] = login(t, handler, name, name+"-password")
	}
	return fixture
}

func (f accessFixture) serve(as, method, path, body string) *httptest.ResponseRecorder {
	return serveSession(f.handler, method, path, body, f.cookies[as])
}

func TestAccessAllowsEachRoleItsOperations(t *testing.T) {
	fixture := newAccessFixture(t)
	for _, check := range []struct {
		as, method, path string
		status           int
		code             core.ApiErrorCode
	}{
		{"bob", http.MethodGet, "/api/v0/graph", http.StatusNoContent, ""},
		{"bob", http.MethodGet, "/api/v0/raw-texts", http.StatusNoContent, ""},
		{"bob", http.MethodPost, "/api/v0/assertions", http.StatusForbidden, core.ApiErrorCodePermissionDenied},
		{"bob", http.MethodPost, "/api/v0/stages/loading", http.StatusForbidden, core.ApiErrorCodePermissionDenied},
		{"bob", http.MethodGet, "/api/v0/members", http.StatusForbidden, core.ApiErrorCodePermissionDenied},
		{"alice", http.MethodPost, "/api/v0/assertions", http.StatusNoContent, ""},
		{"alice", http.MethodPost, "/api/v0/stages/processing", http.StatusNoContent, ""},
		// 役割を持たない利用者には、調査の存在を返さない。
		{"carol", http.MethodGet, "/api/v0/graph", http.StatusNotFound, core.ApiErrorCodeInvestigationNotFound},
		{"carol", http.MethodGet, "/api/v0/stages", http.StatusNotFound, core.ApiErrorCodeInvestigationNotFound},
		{"carol", http.MethodGet, "/api/v0/members/me", http.StatusNotFound, core.ApiErrorCodeInvestigationNotFound},
		// 自分の役割を読む path への書き込みは、役割の管理として管理者を要る。
		{"carol", http.MethodPut, "/api/v0/members/me", http.StatusNotFound, core.ApiErrorCodeInvestigationNotFound},
		{"carol", http.MethodDelete, "/api/v0/members/me", http.StatusNotFound, core.ApiErrorCodeInvestigationNotFound},
		{"bob", http.MethodPut, "/api/v0/members/me", http.StatusForbidden, core.ApiErrorCodePermissionDenied},
		{"bob", http.MethodHead, "/api/v0/graph", http.StatusNoContent, ""},
		// 画面の file は役割を要らない。
		{"carol", http.MethodGet, "/index.html", http.StatusNoContent, ""},
	} {
		recorder := fixture.serve(check.as, check.method, check.path, "")
		if recorder.Code != check.status {
			t.Errorf("%s %s %s status=%d want=%d body=%s", check.as, check.method, check.path, recorder.Code,
				check.status, recorder.Body)
			continue
		}
		if check.code != "" && decodeApiError(t, recorder).Code != check.code {
			t.Errorf("%s %s %s body=%s want code %s", check.as, check.method, check.path, recorder.Body, check.code)
		}
	}
}

func TestAccessManagesMembersAndAppliesTheChangeToTheNextRequest(t *testing.T) {
	fixture := newAccessFixture(t)
	if recorder := fixture.serve("alice", http.MethodPut, "/api/v0/members/carol", `{"role":"editor"}`); recorder.Code != http.StatusOK {
		t.Fatalf("grant status=%d body=%s", recorder.Code, recorder.Body)
	}
	if recorder := fixture.serve("carol", http.MethodPost, "/api/v0/assertions", ""); recorder.Code != http.StatusNoContent {
		t.Fatalf("after the grant status=%d", recorder.Code)
	}
	me := fixture.serve("carol", http.MethodGet, "/api/v0/members/me", "")
	var item struct {
		Login, DisplayName, Role, GrantedBy string
	}
	if err := json.Unmarshal(me.Body.Bytes(), &item); err != nil || item.Role != "editor" || item.GrantedBy != "alice" ||
		item.DisplayName != "CAROL" {
		t.Fatalf("me=%s err=%v", me.Body, err)
	}
	if recorder := fixture.serve("alice", http.MethodDelete, "/api/v0/members/carol", ""); recorder.Code != http.StatusNoContent {
		t.Fatalf("revoke status=%d body=%s", recorder.Code, recorder.Body)
	}
	if recorder := fixture.serve("carol", http.MethodGet, "/api/v0/graph", ""); recorder.Code != http.StatusNotFound {
		t.Fatalf("after the revocation status=%d", recorder.Code)
	}
	list := fixture.serve("alice", http.MethodGet, "/api/v0/members", "")
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"login":"bob"`) ||
		strings.Contains(list.Body.String(), `"login":"carol"`) {
		t.Fatalf("members=%s", list.Body)
	}
}

func TestAccessRejectsUnreadableMemberChanges(t *testing.T) {
	fixture := newAccessFixture(t)
	for _, check := range []struct {
		method, path, body string
		status             int
	}{
		{http.MethodPut, "/api/v0/members/carol", `{"role":"owner"}`, http.StatusBadRequest},
		{http.MethodPut, "/api/v0/members/carol", `{"role":"viewer","extra":1}`, http.StatusBadRequest},
		{http.MethodPut, "/api/v0/members/nobody", `{"role":"viewer"}`, http.StatusBadRequest},
		{http.MethodPut, "/api/v0/members/alice", `{"role":"viewer"}`, http.StatusBadRequest},
		{http.MethodDelete, "/api/v0/members/alice", "", http.StatusBadRequest},
		{http.MethodDelete, "/api/v0/members/carol", "", http.StatusNotFound},
	} {
		if recorder := fixture.serve("alice", check.method, check.path, check.body); recorder.Code != check.status {
			t.Errorf("%s %s %s status=%d want=%d body=%s", check.method, check.path, check.body, recorder.Code,
				check.status, recorder.Body)
		}
	}
}

// アカウントを持たない起動は、役割を検査しない。
func TestAccessWithoutAccountsPassesThrough(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	handler := api.NewSessionHandler(api.NewAccessHandler(next, pipeline.NewAccessStore(nil), newFakeAccounts()), nil)
	if recorder := serveSession(handler, http.MethodPost, "/api/v0/assertions", "", nil); recorder.Code != http.StatusNoContent {
		t.Fatalf("status=%d", recorder.Code)
	}
}

func TestRequiredRoleFallsToTheStrongerRole(t *testing.T) {
	for _, check := range []struct {
		method, path string
		role         pipeline.Role
		needed       bool
	}{
		{http.MethodGet, "/api/v0/anything-new", pipeline.RoleViewer, true},
		{http.MethodPatch, "/api/v0/anything-new", pipeline.RoleEditor, true},
		{http.MethodGet, "/api/v0/members", pipeline.RoleAdmin, true},
		{http.MethodGet, "/api/v0/members/me", "", false},
		{http.MethodPut, "/api/v0/members/me", pipeline.RoleAdmin, true},
		{http.MethodDelete, "/api/v0/members/me", pipeline.RoleAdmin, true},
		// 基準の directory の一覧は、読み込みを始める編集者だけが読む。
		{http.MethodGet, "/api/v0/stages/source-files", pipeline.RoleEditor, true},
		{http.MethodGet, "/api/v0/stages", pipeline.RoleViewer, true},
	} {
		role, needed := api.RequiredRole(check.method, check.path)
		if role != check.role || needed != check.needed {
			t.Errorf("RequiredRole(%s %s)=%s,%v want %s,%v", check.method, check.path, role, needed, check.role, check.needed)
		}
	}
}
