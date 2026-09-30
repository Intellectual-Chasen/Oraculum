package api_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/api"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// lockedAccounts は、SSE の handler が別の goroutine から読む fakeAccounts を lock で守る。
type lockedAccounts struct {
	mu sync.Mutex
	*fakeAccounts
	disabled map[string]bool
}

func (a *lockedAccounts) Login(ctx context.Context, login, password string) (api.AccountSession, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.fakeAccounts.Login(ctx, login, password)
}

// SessionAccount は、無効にしたアカウントのセッションを使えないものとして扱う。
func (a *lockedAccounts) SessionAccount(ctx context.Context, token string) (api.Account, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	account, err := a.fakeAccounts.SessionAccount(ctx, token)
	if err == nil && a.disabled[account.Login] {
		return api.Account{}, api.ErrSessionInvalid
	}
	return account, err
}

func (a *lockedAccounts) disable(login string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.disabled[login] = true
}

type syncFixture struct {
	accessFixture
	accounts *lockedAccounts
	server   *httptest.Server
}

// newSyncFixture は、alice・bob・carol・erin (どれも viewer) がログインした server を、boundary と
// 短い WriteTimeout を持つ http.Server で起動する。
func newSyncFixture(t *testing.T) syncFixture {
	t.Helper()
	accounts := &lockedAccounts{fakeAccounts: newFakeAccounts(), disabled: map[string]bool{}}
	accounts.passwords["carol"], accounts.passwords["erin"] = "carol-password", "erin-password"
	access := pipeline.NewAccessStore([]pipeline.Member{
		{Login: "alice", Role: pipeline.RoleViewer}, {Login: "bob", Role: pipeline.RoleViewer},
		{Login: "carol", Role: pipeline.RoleViewer}, {Login: "erin", Role: pipeline.RoleViewer},
	})
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	handler := api.NewSessionHandler(api.NewAccessHandler(
		api.NewWorkspaceHandler(next, pipeline.NewWorkspaceStore(nil), access, accounts, api.NewWorkspaceHub()),
		access, accounts), accounts)
	fixture := syncFixture{accessFixture: accessFixture{handler: handler, access: access, cookies: map[string]*http.Cookie{}},
		accounts: accounts}
	for _, name := range []string{"alice", "bob", "carol", "erin"} {
		fixture.cookies[name] = login(t, handler, name, name+"-password")
	}
	fixture.server = startSyncServer(t, handler)
	return fixture
}

// startSyncServer は handler を boundary の内側に置いて起動する。停止は t.Cleanup で行う。
func startSyncServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewUnstartedServer(nil)
	bounded, err := api.NewBoundaryHandler(handler, []string{server.Listener.Addr().String()})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	server.Config.Handler = bounded
	server.Config.BaseContext = func(_ net.Listener) context.Context { return ctx }
	server.Config.WriteTimeout = 200 * time.Millisecond
	server.Start()
	t.Cleanup(func() {
		cancel()
		server.Close()
	})
	return server
}

// eventStream は SSE の接続 1 本から読んだイベントである。
type eventStream struct {
	events chan string
	cancel context.CancelFunc
}

func openStream(t *testing.T, server *httptest.Server, cookie *http.Cookie, workspaceId, connectionId string) *eventStream {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	request, err := http.NewRequestWithContext(ctx, http.MethodGet,
		server.URL+"/api/v0/workspaces/"+workspaceId+"/events?connectionId="+connectionId, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("status=%d type=%s", response.StatusCode, response.Header.Get("Content-Type"))
	}
	stream := &eventStream{events: make(chan string, 256), cancel: cancel}
	go func() {
		defer close(stream.events)
		defer response.Body.Close()
		scanner := bufio.NewScanner(response.Body)
		for scanner.Scan() {
			if data, ok := strings.CutPrefix(scanner.Text(), "data: "); ok {
				stream.events <- data
			}
		}
	}()
	t.Cleanup(cancel)
	return stream
}

// next は次のイベントを返す。接続が閉じていれば空の map を返す。
func (s *eventStream) next(t *testing.T) map[string]any {
	t.Helper()
	select {
	case data, open := <-s.events:
		if !open {
			return map[string]any{}
		}
		if strings.Contains(data, "shares") || strings.Contains(data, "grantedBy") {
			t.Fatalf("the event carries the shares: %s", data)
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			t.Fatal(err)
		}
		return event
	case <-time.After(5 * time.Second):
		t.Fatal("no event arrived")
		return nil
	}
}

// nextOf は type が kind の次のイベントを返す。
func (s *eventStream) nextOf(t *testing.T, kind string) map[string]any {
	t.Helper()
	for {
		event := s.next(t)
		if event["type"] == kind || len(event) == 0 {
			return event
		}
	}
}

func (s *eventStream) closedAfter(t *testing.T) {
	t.Helper()
	if event := s.next(t); len(event) != 0 {
		t.Fatalf("the stream continues with %v", event)
	}
}

func presenceIds(event map[string]any) []string {
	var ids []string
	for _, connection := range event["connections"].([]any) {
		ids = append(ids, connection.(map[string]any)["connectionId"].(string))
	}
	return ids
}

func (f syncFixture) createShared(t *testing.T, state string, shares map[string]string) string {
	t.Helper()
	created := decodeWorkspace(t, f.serve("alice", http.MethodPost, "/api/v0/workspaces",
		`{"name":"共同の確認","state":`+state+`}`), http.StatusCreated)
	for login, access := range shares {
		if recorder := f.serve("alice", http.MethodPut, "/api/v0/workspaces/"+created.Id+"/shares/"+login,
			`{"access":"`+access+`"}`); recorder.Code != http.StatusOK {
			t.Fatalf("share status=%d body=%s", recorder.Code, recorder.Body)
		}
	}
	return created.Id
}

// 接続した利用者は snapshot から始め、ほかの利用者の変更と presence を受け取る。
func TestWorkspaceSyncDeliversSnapshotsChangesAndPresence(t *testing.T) {
	f := newSyncFixture(t)
	id := f.createShared(t, `{"a":1,"b":1}`, map[string]string{"bob": "edit", "carol": "view"})
	path := "/api/v0/workspaces/" + id
	alice := openStream(t, f.server, f.cookies["alice"], id, "conn-alice")
	if snapshot := alice.next(t); snapshot["type"] != "snapshot" || snapshot["revision"] != 1.0 ||
		snapshot["state"].(map[string]any)["a"] != 1.0 || snapshot["name"] != "共同の確認" {
		t.Fatalf("snapshot=%v", snapshot)
	}
	bob := openStream(t, f.server, f.cookies["bob"], id, "conn-bob")
	bob.nextOf(t, "snapshot")
	if ids := presenceIds(alice.nextOf(t, "presence")); len(ids) != 1 || ids[0] != "conn-alice" {
		t.Fatalf("ids=%v", ids)
	}
	if ids := presenceIds(alice.nextOf(t, "presence")); len(ids) != 2 || ids[0] != "conn-alice" || ids[1] != "conn-bob" {
		t.Fatalf("ids=%v", ids)
	}
	// server の WriteTimeout を過ぎても、接続は切れない。
	time.Sleep(400 * time.Millisecond)

	changed := f.serve("bob", http.MethodPost, path+"/changes", `{"clientChangeId":"c-1","baseRevision":1,"patch":{"b":2}}`)
	if changed.Code != http.StatusOK || !strings.Contains(changed.Body.String(), `"revision":2`) ||
		!strings.Contains(changed.Body.String(), `"updatedBy":"bob"`) {
		t.Fatalf("status=%d body=%s", changed.Code, changed.Body)
	}
	change := alice.nextOf(t, "change")
	if change["revision"] != 2.0 || change["actor"] != "bob" || change["clientChangeId"] != "c-1" ||
		change["patch"].(map[string]any)["b"] != 2.0 || change["fields"].([]any)[0] != "b" {
		t.Fatalf("change=%v", change)
	}
	// 再送は前の結果を返し、二重に適用も配信もしない。
	again := f.serve("bob", http.MethodPost, path+"/changes", `{"clientChangeId":"c-1","baseRevision":1,"patch":{"b":2}}`)
	if again.Code != http.StatusOK || !strings.Contains(again.Body.String(), `"revision":2`) {
		t.Fatalf("again status=%d body=%s", again.Code, again.Body)
	}
	// 同じ欄の同時の変更は競合し、別の欄は適用する。
	conflict := f.serve("alice", http.MethodPost, path+"/changes", `{"clientChangeId":"c-2","baseRevision":1,"patch":{"b":3}}`)
	var conflictBody struct {
		Code     string `json:"code"`
		Conflict struct {
			Fields    []string        `json:"fields"`
			Revision  int64           `json:"revision"`
			State     json.RawMessage `json:"state"`
			UpdatedBy string          `json:"updatedBy"`
		} `json:"conflict"`
	}
	if err := json.Unmarshal(conflict.Body.Bytes(), &conflictBody); err != nil || conflict.Code != http.StatusConflict ||
		conflictBody.Code != "workspace_changed" || len(conflictBody.Conflict.Fields) != 1 ||
		conflictBody.Conflict.Fields[0] != "b" || conflictBody.Conflict.Revision != 2 ||
		string(conflictBody.Conflict.State) != `{"a":1,"b":2}` || conflictBody.Conflict.UpdatedBy != "bob" {
		t.Fatalf("status=%d body=%s", conflict.Code, conflict.Body)
	}
	if recorder := f.serve("alice", http.MethodPost, path+"/changes",
		`{"clientChangeId":"c-3","baseRevision":1,"patch":{"a":5}}`); recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body)
	}
	if change := bob.nextOf(t, "change"); change["revision"] != 2.0 {
		t.Fatalf("change=%v", change)
	}
	if change := bob.nextOf(t, "change"); change["revision"] != 3.0 || change["actor"] != "alice" {
		t.Fatalf("the replayed change was delivered again or the change is missing: %v", change)
	}
	// 全体の置き換えは fields ["*"] と state 全体の patch で配信する。
	if recorder := f.serve("alice", http.MethodPut, path, `{"name":"改名","state":{"c":1},"baseRevision":3}`); recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body)
	}
	if change := bob.nextOf(t, "change"); change["fields"].([]any)[0] != "*" || change["name"] != "改名" ||
		change["patch"].(map[string]any)["c"] != 1.0 {
		t.Fatalf("change=%v", change)
	}

	// 閲覧の共有先は変更できない。
	if recorder := f.serve("carol", http.MethodPost, path+"/changes",
		`{"clientChangeId":"c-4","baseRevision":4,"patch":{"a":6}}`); recorder.Code != http.StatusForbidden {
		t.Fatalf("view share status=%d", recorder.Code)
	}
	for _, body := range []string{`{"clientChangeId":"c-5","baseRevision":4,"patch":{}}`,
		`{"clientChangeId":"c-5","baseRevision":4,"patch":[1]}`, `{"clientChangeId":"","baseRevision":4,"patch":{"a":1}}`,
		`{"clientChangeId":"c-5","patch":{"a":1}}`} {
		if recorder := f.serve("alice", http.MethodPost, path+"/changes", body); recorder.Code != http.StatusBadRequest {
			t.Errorf("body=%s status=%d", body, recorder.Code)
		}
	}

	// presence は読める利用者の自分の接続だけを変え、切断で消える。
	carol := openStream(t, f.server, f.cookies["carol"], id, "conn-carol")
	carol.nextOf(t, "snapshot")
	if recorder := f.serve("carol", http.MethodPost, path+"/presence",
		`{"connectionId":"conn-carol","selection":{"node":"n-1"},"following":"conn-alice"}`); recorder.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body)
	}
	for {
		presence := alice.nextOf(t, "presence")
		if strings.Contains(mustJSON(t, presence), `"selection":{"node":"n-1"}`) {
			if !strings.Contains(mustJSON(t, presence), `"following":"conn-alice"`) ||
				!strings.Contains(mustJSON(t, presence), `"displayName":"CAROL"`) {
				t.Fatalf("presence=%v", presence)
			}
			break
		}
	}
	for _, body := range []string{`{"connectionId":"conn-alice","selection":null,"following":null}`,
		`{"connectionId":"conn-none","selection":null,"following":null}`} {
		if recorder := f.serve("carol", http.MethodPost, path+"/presence", body); recorder.Code != http.StatusNotFound {
			t.Errorf("body=%s status=%d", body, recorder.Code)
		}
	}
	if recorder := f.serve("carol", http.MethodPost, path+"/presence",
		`{"connectionId":"conn-carol","selection":"`+strings.Repeat("x", 2048)+`","following":null}`); recorder.Code != http.StatusBadRequest {
		t.Errorf("large selection status=%d", recorder.Code)
	}
	bob.cancel()
	for {
		if ids := presenceIds(alice.nextOf(t, "presence")); len(ids) == 2 {
			if ids[0] != "conn-alice" || ids[1] != "conn-carol" {
				t.Fatalf("ids=%v", ids)
			}
			break
		}
	}
	// 接続し直すと最新の snapshot から始まる。
	reopened := openStream(t, f.server, f.cookies["bob"], id, "conn-bob")
	if snapshot := reopened.next(t); snapshot["type"] != "snapshot" || snapshot["revision"] != 4.0 ||
		snapshot["name"] != "改名" {
		t.Fatalf("snapshot=%v", snapshot)
	}
	// 値が null の欄は state から消え、配信の patch は null をそのまま持つ。
	if recorder := f.serve("alice", http.MethodPost, path+"/changes",
		`{"clientChangeId":"c-6","baseRevision":4,"patch":{"c":null}}`); recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body)
	}
	if change := mustJSON(t, reopened.nextOf(t, "change")); !strings.Contains(change, `"patch":{"c":null}`) {
		t.Fatalf("change=%s", change)
	}
	if read := decodeWorkspace(t, f.serve("bob", http.MethodGet, path, ""), http.StatusOK); string(read.State) != `{}` {
		t.Fatalf("state=%s", read.State)
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// 共有の取り消し・役割の取り外し・アカウントの無効化・削除の後の次のイベントで、closed を送って閉じる。
func TestWorkspaceSyncClosesConnectionsThatLostTheWorkspace(t *testing.T) {
	f := newSyncFixture(t)
	id := f.createShared(t, `{}`, map[string]string{"bob": "edit", "carol": "view", "erin": "view"})
	path := "/api/v0/workspaces/" + id
	alice := openStream(t, f.server, f.cookies["alice"], id, "a")
	streams := map[string]*eventStream{}
	for _, name := range []string{"bob", "carol", "erin"} {
		streams[name] = openStream(t, f.server, f.cookies[name], id, name)
		streams[name].nextOf(t, "snapshot")
	}
	expectClosed := func(name, reason string) {
		t.Helper()
		if closed := streams[name].nextOf(t, "closed"); closed["reason"] != reason {
			t.Fatalf("%s closed=%v", name, closed)
		}
		streams[name].closedAfter(t)
	}
	touch := func(revision int, clientChangeId string) {
		t.Helper()
		if recorder := f.serve("alice", http.MethodPost, path+"/changes", `{"clientChangeId":"`+clientChangeId+
			`","baseRevision":`+strconv.Itoa(revision)+`,"patch":{"x":1}}`); recorder.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body)
		}
	}
	if recorder := f.serve("alice", http.MethodDelete, path+"/shares/carol", ""); recorder.Code != http.StatusNoContent {
		t.Fatalf("unshare status=%d", recorder.Code)
	}
	expectClosed("carol", "revoked")
	if err := f.access.Revoke("alice", "bob"); err != nil {
		t.Fatal(err)
	}
	touch(1, "c-1")
	expectClosed("bob", "revoked")
	f.accounts.disable("erin")
	touch(2, "c-2")
	expectClosed("erin", "revoked")
	if recorder := f.serve("alice", http.MethodDelete, path, ""); recorder.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d", recorder.Code)
	}
	if closed := alice.nextOf(t, "closed"); closed["reason"] != "deleted" {
		t.Fatalf("closed=%v", closed)
	}
}

// server の停止の合図で SSE の handler が戻り、停止を妨げない。
func TestWorkspaceSyncHandlerReturnsOnShutdown(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	handler := api.NewSessionHandler(
		api.NewWorkspaceHandler(next, pipeline.NewWorkspaceStore(nil), nil, nil, api.NewWorkspaceHub()), nil)
	created := decodeWorkspace(t, serveSession(handler, http.MethodPost, "/api/v0/workspaces",
		`{"name":"x","state":{}}`, nil), http.StatusCreated)
	server := httptest.NewUnstartedServer(handler)
	ctx, cancel := context.WithCancel(context.Background())
	server.Config.BaseContext = func(net.Listener) context.Context { return ctx }
	server.Start()
	defer server.Close()
	stream := openStream(t, server, nil, created.Id, "local-1")
	stream.nextOf(t, "snapshot")
	// アカウントを持たない起動の利用者は local である。
	if presence := mustJSON(t, stream.nextOf(t, "presence")); !strings.Contains(presence, `"login":"local"`) ||
		!strings.Contains(presence, `"displayName":"local"`) || !strings.Contains(presence, `"selection":null`) {
		t.Fatalf("presence=%v", presence)
	}
	cancel()
	shutdownCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	if err := server.Config.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("shutdown err=%v", err)
	}
	stream.closedAfter(t)
}

// revokeOnFirstRead は、最初の Get の直後に hook を実行する。権限を確かめた後、書く前に取り消しを入れる。
type revokeOnFirstRead struct {
	*pipeline.WorkspaceStore
	once sync.Once
	hook func()
}

func (s *revokeOnFirstRead) Get(id string) (pipeline.Workspace, bool) {
	workspace, ok := s.WorkspaceStore.Get(id)
	if s.hook != nil {
		s.once.Do(s.hook)
	}
	return workspace, ok
}

// 権限を確かめてから書くまでの間に共有を取り消す・弱めると、書かない。
func TestWorkspaceWritesRecheckTheAccessBeforeWriting(t *testing.T) {
	accounts := newFakeAccounts()
	access := pipeline.NewAccessStore([]pipeline.Member{
		{Login: "alice", Role: pipeline.RoleViewer}, {Login: "bob", Role: pipeline.RoleViewer},
	})
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	for _, tc := range []struct {
		name, method, suffix, body string
		revoke                     func(store *pipeline.WorkspaceStore, id string) error
		status                     int
	}{
		{"change after unshare", http.MethodPost, "/changes", `{"clientChangeId":"c-1","baseRevision":1,"patch":{"a":2}}`,
			func(store *pipeline.WorkspaceStore, id string) error { return store.Unshare(id, "alice", "bob") },
			http.StatusNotFound},
		{"replace after the view share", http.MethodPut, "", `{"name":"x","state":{"a":2},"baseRevision":1}`,
			func(store *pipeline.WorkspaceStore, id string) error {
				_, err := store.Share(id, "alice", "bob", pipeline.WorkspaceView)
				return err
			}, http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &revokeOnFirstRead{WorkspaceStore: pipeline.NewWorkspaceStore(nil)}
			handler := api.NewSessionHandler(api.NewAccessHandler(
				api.NewWorkspaceHandler(next, store, access, accounts, api.NewWorkspaceHub()), access, accounts), accounts)
			created, err := store.Create("alice", "w", []byte(`{"a":1}`))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.Share(created.Id, "alice", "bob", pipeline.WorkspaceEdit); err != nil {
				t.Fatal(err)
			}
			store.hook = func() {
				if err := tc.revoke(store.WorkspaceStore, created.Id); err != nil {
					t.Error(err)
				}
			}
			recorder := serveSession(handler, tc.method, "/api/v0/workspaces/"+created.Id+tc.suffix, tc.body,
				login(t, handler, "bob", "bob-password"))
			if recorder.Code != tc.status {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body)
			}
			if stored, _ := store.WorkspaceStore.Get(created.Id); stored.Revision != 1 || string(stored.State) != `{"a":1}` {
				t.Fatalf("the revoked write was applied: %+v", stored)
			}
		})
	}
}

// 別の利用者の connectionId・未来の base・別の利用者の clientChangeId・本文の後ろのデータを退ける。
func TestWorkspaceSyncRejectsForeignIdsAndFutureBases(t *testing.T) {
	f := newSyncFixture(t)
	id := f.createShared(t, `{}`, map[string]string{"bob": "edit"})
	path := "/api/v0/workspaces/" + id
	openStream(t, f.server, f.cookies["alice"], id, "same").nextOf(t, "snapshot")
	request, err := http.NewRequest(http.MethodGet, f.server.URL+path+"/events?connectionId=same", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.AddCookie(f.cookies["bob"])
	response, err := f.server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("foreign connectionId status=%d", response.StatusCode)
	}
	future := f.serve("alice", http.MethodPost, path+"/changes", `{"clientChangeId":"c-1","baseRevision":9,"patch":{"a":1}}`)
	if future.Code != http.StatusConflict || !strings.Contains(future.Body.String(), `"fields":["*"]`) {
		t.Fatalf("future base status=%d body=%s", future.Code, future.Body)
	}
	if recorder := f.serve("alice", http.MethodPost, path+"/changes",
		`{"clientChangeId":"c-2","baseRevision":1,"patch":{"a":1}}`); recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body)
	}
	if recorder := f.serve("bob", http.MethodPost, path+"/changes",
		`{"clientChangeId":"c-2","baseRevision":1,"patch":{"a":1}}`); recorder.Code != http.StatusBadRequest {
		t.Fatalf("foreign clientChangeId status=%d body=%s", recorder.Code, recorder.Body)
	}
	if recorder := f.serve("alice", http.MethodPost, path+"/presence",
		`{"connectionId":"same","selection":null,"following":null}{}`); recorder.Code != http.StatusBadRequest {
		t.Fatalf("trailing presence status=%d", recorder.Code)
	}
}
