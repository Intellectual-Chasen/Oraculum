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

type workspaceJSON struct {
	Id        string          `json:"id"`
	Name      string          `json:"name"`
	Owner     string          `json:"owner"`
	Revision  int64           `json:"revision"`
	UpdatedBy string          `json:"updatedBy"`
	Access    string          `json:"access"`
	State     json.RawMessage `json:"state"`
}

// workspaceFixture は、alice・bob・carol (どれも viewer) と、役割を持たない dave がログインした server
// を組む。
func workspaceFixture(t *testing.T) accessFixture {
	t.Helper()
	accounts := newFakeAccounts()
	accounts.passwords["carol"], accounts.passwords["dave"] = "carol-password", "dave-password"
	access := pipeline.NewAccessStore([]pipeline.Member{
		{Login: "alice", Role: pipeline.RoleViewer}, {Login: "bob", Role: pipeline.RoleViewer},
		{Login: "carol", Role: pipeline.RoleViewer},
	})
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	handler := api.NewSessionHandler(
		api.NewAccessHandler(api.NewWorkspaceHandler(next, pipeline.NewWorkspaceStore(nil), access, accounts,
			api.NewWorkspaceHub()), access, accounts),
		accounts)
	fixture := accessFixture{handler: handler, access: access, cookies: map[string]*http.Cookie{}}
	for _, name := range []string{"alice", "bob", "carol", "dave"} {
		fixture.cookies[name] = login(t, handler, name, name+"-password")
	}
	return fixture
}

func decodeWorkspace(t *testing.T, recorder *httptest.ResponseRecorder, status int) workspaceJSON {
	t.Helper()
	if recorder.Code != status {
		t.Fatalf("status=%d want=%d body=%s", recorder.Code, status, recorder.Body)
	}
	var workspace workspaceJSON
	if err := json.Unmarshal(recorder.Body.Bytes(), &workspace); err != nil {
		t.Fatal(err)
	}
	return workspace
}

// 閲覧者は自分のワークスペースを保存・一覧・読み出し・置き換え・削除でき、同じ状態を読み戻せる。
func TestWorkspacesAreSavedAndReopenedByTheOwner(t *testing.T) {
	fixture := workspaceFixture(t)
	state := `{"searchTerms":{"contains":["powershell"]},"bookmarks":[]}`
	created := decodeWorkspace(t, fixture.serve("alice", http.MethodPost, "/api/v0/workspaces",
		`{"name":"横移動の確認","state":`+state+`}`), http.StatusCreated)
	if created.Owner != "alice" || created.Revision != 1 || string(created.State) != state {
		t.Fatalf("created=%+v", created)
	}
	path := "/api/v0/workspaces/" + created.Id
	read := decodeWorkspace(t, fixture.serve("alice", http.MethodGet, path, ""), http.StatusOK)
	if string(read.State) != state {
		t.Fatalf("state=%s", read.State)
	}
	list := fixture.serve("alice", http.MethodGet, "/api/v0/workspaces", "")
	if !strings.Contains(list.Body.String(), created.Id) || strings.Contains(list.Body.String(), "powershell") {
		t.Fatalf("list=%s", list.Body)
	}
	replaced := decodeWorkspace(t, fixture.serve("alice", http.MethodPut, path,
		`{"name":"横移動の確認","state":{"bookmarks":[]},"baseRevision":1}`), http.StatusOK)
	if replaced.Revision != 2 {
		t.Fatalf("replaced=%+v", replaced)
	}
	stale := fixture.serve("alice", http.MethodPut, path, `{"name":"x","state":{},"baseRevision":1}`)
	if stale.Code != http.StatusConflict || decodeApiError(t, stale).Code != core.ApiErrorCodeWorkspaceChanged {
		t.Fatalf("stale status=%d body=%s", stale.Code, stale.Body)
	}
	if recorder := fixture.serve("alice", http.MethodDelete, path, ""); recorder.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d", recorder.Code)
	}
	if recorder := fixture.serve("alice", http.MethodGet, path, ""); recorder.Code != http.StatusNotFound {
		t.Fatalf("after delete status=%d", recorder.Code)
	}
}

// 識別子を知るだけでは、別の利用者のワークスペースを読めない。
func TestWorkspacesOfAnotherOwnerAreNotReadable(t *testing.T) {
	fixture := workspaceFixture(t)
	created := decodeWorkspace(t, fixture.serve("alice", http.MethodPost, "/api/v0/workspaces",
		`{"name":"alice の作業","state":{"secret":true}}`), http.StatusCreated)
	path := "/api/v0/workspaces/" + created.Id
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		body := ""
		if method == http.MethodPut {
			body = `{"name":"x","state":{},"baseRevision":1}`
		}
		recorder := fixture.serve("bob", method, path, body)
		if recorder.Code != http.StatusNotFound || strings.Contains(recorder.Body.String(), "secret") {
			t.Fatalf("%s status=%d body=%s", method, recorder.Code, recorder.Body)
		}
	}
	if list := fixture.serve("bob", http.MethodGet, "/api/v0/workspaces", ""); strings.Contains(list.Body.String(), created.Id) {
		t.Fatalf("bob lists alice's workspace: %s", list.Body)
	}
}

func TestWorkspacesRejectUnreadableBodies(t *testing.T) {
	fixture := workspaceFixture(t)
	for _, body := range []string{
		`{"name":"x"}`, `{"name":"x","state":[1]}`, `{"name":"x","state":"s"}`, `{"name":" ","state":{}}`,
		`{"name":"x","state":{},"baseRevision":1}`, `{"name":"x","state":{},"extra":1}`,
	} {
		if recorder := fixture.serve("alice", http.MethodPost, "/api/v0/workspaces", body); recorder.Code != http.StatusBadRequest {
			t.Errorf("body=%s status=%d", body, recorder.Code)
		}
	}
	created := decodeWorkspace(t, fixture.serve("alice", http.MethodPost, "/api/v0/workspaces",
		`{"name":"x","state":{}}`), http.StatusCreated)
	for _, body := range []string{`{"name":"x","state":{}}`, `{"state":{},"baseRevision":1}`} {
		if recorder := fixture.serve("alice", http.MethodPut, "/api/v0/workspaces/"+created.Id,
			body); recorder.Code != http.StatusBadRequest {
			t.Fatalf("replace body=%s status=%d", body, recorder.Code)
		}
	}
}

// 1 MiB を超える状態を、作成・置き換え・変更のどれでも記録する。
func TestWorkspacesStoreStatesLargerThanOneMebibyte(t *testing.T) {
	fixture := workspaceFixture(t)
	large := `{"a":"` + strings.Repeat("a", 2<<20) + `"}`
	created := decodeWorkspace(t, fixture.serve("alice", http.MethodPost, "/api/v0/workspaces",
		`{"name":"x","state":`+large+`}`), http.StatusCreated)
	if string(created.State) != large {
		t.Fatalf("created state bytes=%d", len(created.State))
	}
	path := "/api/v0/workspaces/" + created.Id
	replaced := decodeWorkspace(t, fixture.serve("alice", http.MethodPut, path,
		`{"name":"x","state":`+large+`,"baseRevision":1}`), http.StatusOK)
	if replaced.Revision != 2 {
		t.Fatalf("replaced=%d", replaced.Revision)
	}
	second := strings.Repeat("b", 2<<20)
	change := fixture.serve("alice", http.MethodPost, path+"/changes",
		`{"clientChangeId":"c-1","baseRevision":2,"patch":{"b":"`+second+`"}}`)
	if change.Code != http.StatusOK {
		t.Fatalf("change status=%d body=%.200s", change.Code, change.Body)
	}
	want := `{"a":"` + strings.Repeat("a", 2<<20) + `","b":"` + second + `"}`
	if read := decodeWorkspace(t, fixture.serve("alice", http.MethodGet, path, ""), http.StatusOK); string(read.State) != want {
		t.Fatalf("read state bytes=%d want=%d", len(read.State), len(want))
	}
}

// 名前を持たない作成は、利用者の「ワークスペース N」の最大の N に 1 を足した名前を付ける。
func TestWorkspacesWithoutANameAreNumbered(t *testing.T) {
	fixture := workspaceFixture(t)
	decodeWorkspace(t, fixture.serve("alice", http.MethodPost, "/api/v0/workspaces",
		`{"name":"ワークスペース 3","state":{}}`), http.StatusCreated)
	created := decodeWorkspace(t, fixture.serve("alice", http.MethodPost, "/api/v0/workspaces", `{"state":{}}`),
		http.StatusCreated)
	if created.Name != "ワークスペース 4" {
		t.Fatalf("name=%q", created.Name)
	}
}

// アカウントを持たない起動のワークスペースは、予約した所有者のものとして保存する。
func TestWorkspacesWithoutAccountsBelongToTheLocalOwner(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	handler := api.NewSessionHandler(api.NewWorkspaceHandler(next, pipeline.NewWorkspaceStore(nil), nil, nil,
		api.NewWorkspaceHub()), nil)
	created := decodeWorkspace(t, serveSession(handler, http.MethodPost, "/api/v0/workspaces",
		`{"name":"x","state":{}}`, nil), http.StatusCreated)
	if created.Owner != "local" || created.Access != "owner" {
		t.Fatalf("created=%+v", created)
	}
	sharePath := "/api/v0/workspaces/" + created.Id + "/shares/bob"
	for _, method := range []string{http.MethodPut, http.MethodDelete} {
		if recorder := serveSession(handler, method, sharePath, `{"access":"view"}`, nil); recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s share without accounts status=%d", method, recorder.Code)
		}
	}
}

// 所有者は役割を持つ利用者へ共有でき、共有先は access の範囲で読み・置き換え、取り消しと役割の
// 取り外しの後は読めない。
func TestWorkspacesAreSharedWithinTheAccess(t *testing.T) {
	fixture := workspaceFixture(t)
	created := decodeWorkspace(t, fixture.serve("alice", http.MethodPost, "/api/v0/workspaces",
		`{"name":"共有","state":{}}`), http.StatusCreated)
	path := "/api/v0/workspaces/" + created.Id
	put := `{"name":"共有","state":{"a":1},"baseRevision":1}`

	if recorder := fixture.serve("bob", http.MethodGet, path, ""); recorder.Code != http.StatusNotFound {
		t.Fatalf("unshared status=%d", recorder.Code)
	}
	for _, target := range []string{"dave", "alice", "local"} {
		if recorder := fixture.serve("alice", http.MethodPut, path+"/shares/"+target, `{"access":"view"}`); recorder.Code != http.StatusBadRequest {
			t.Fatalf("share with %s status=%d", target, recorder.Code)
		}
	}
	if recorder := fixture.serve("alice", http.MethodPut, path+"/shares/bob", `{"access":"own"}`); recorder.Code != http.StatusBadRequest {
		t.Fatalf("unknown access status=%d", recorder.Code)
	}
	shared := fixture.serve("alice", http.MethodPut, path+"/shares/bob", `{"access":"view"}`)
	var share struct{ Login, Access, GrantedBy, GrantedAt string }
	if err := json.Unmarshal(shared.Body.Bytes(), &share); err != nil || shared.Code != http.StatusOK ||
		share.Login != "bob" || share.Access != "view" || share.GrantedBy != "alice" || share.GrantedAt == "" {
		t.Fatalf("share status=%d body=%s", shared.Code, shared.Body)
	}
	if recorder := fixture.serve("alice", http.MethodPut, path+"/shares/carol", `{"access":"edit"}`); recorder.Code != http.StatusOK {
		t.Fatalf("share with carol status=%d", recorder.Code)
	}

	read := fixture.serve("bob", http.MethodGet, path, "")
	if read.Code != http.StatusOK || strings.Contains(read.Body.String(), `"shares"`) ||
		!strings.Contains(read.Body.String(), `"access":"view"`) {
		t.Fatalf("viewer read status=%d body=%s", read.Code, read.Body)
	}
	owned := fixture.serve("alice", http.MethodGet, path, "")
	if !strings.Contains(owned.Body.String(), `"shares":[{"login":"bob"`) || !strings.Contains(owned.Body.String(), `"login":"carol"`) {
		t.Fatalf("owner read body=%s", owned.Body)
	}
	for _, as := range []string{"bob", "carol"} {
		for _, request := range [][2]string{{http.MethodDelete, path}, {http.MethodPut, path + "/shares/dave"}, {http.MethodDelete, path + "/shares/bob"}} {
			recorder := fixture.serve(as, request[0], request[1], `{"access":"view"}`)
			if recorder.Code != http.StatusForbidden || decodeApiError(t, recorder).Code != core.ApiErrorCodePermissionDenied {
				t.Fatalf("%s %s %s status=%d", as, request[0], request[1], recorder.Code)
			}
		}
	}
	if recorder := fixture.serve("bob", http.MethodPut, path, put); recorder.Code != http.StatusForbidden {
		t.Fatalf("viewer replace status=%d", recorder.Code)
	}
	if replaced := decodeWorkspace(t, fixture.serve("carol", http.MethodPut, path, put), http.StatusOK); replaced.UpdatedBy != "carol" {
		t.Fatalf("replaced=%+v", replaced)
	}
	var list struct{ Workspaces []workspaceJSON }
	if err := json.Unmarshal(fixture.serve("carol", http.MethodGet, "/api/v0/workspaces", "").Body.Bytes(), &list); err != nil ||
		len(list.Workspaces) != 1 || list.Workspaces[0].Access != "edit" {
		t.Fatalf("carol list=%+v err=%v", list, err)
	}

	if recorder := fixture.serve("alice", http.MethodDelete, path+"/shares/bob", ""); recorder.Code != http.StatusNoContent {
		t.Fatalf("unshare status=%d", recorder.Code)
	}
	if recorder := fixture.serve("alice", http.MethodDelete, path+"/shares/bob", ""); recorder.Code != http.StatusNotFound {
		t.Fatalf("second unshare status=%d", recorder.Code)
	}
	if recorder := fixture.serve("bob", http.MethodGet, path, ""); recorder.Code != http.StatusNotFound {
		t.Fatalf("after unshare status=%d", recorder.Code)
	}
	if err := fixture.access.Revoke("alice", "carol"); err != nil {
		t.Fatal(err)
	}
	if recorder := fixture.serve("carol", http.MethodGet, path, ""); recorder.Code != http.StatusNotFound {
		t.Fatalf("after role removal status=%d", recorder.Code)
	}
}
