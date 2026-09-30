package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/output"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// ワークスペースの同期の path と上限。
const (
	workspaceChangesPath  = "/api/v0/workspaces/{workspaceId}/changes"
	workspaceEventsPath   = "/api/v0/workspaces/{workspaceId}/events"
	workspacePresencePath = "/api/v0/workspaces/{workspaceId}/presence"
	// syncSendBuffer は接続ごとに送信を待てるイベントの数である。満ちた接続は閉じる。
	syncSendBuffer = 64
	// syncKeepalive は、イベントが無い間に keepalive の行を送る間隔である。
	syncKeepalive = 30 * time.Second
	// maxConnectionIdBytes は client が作る接続の識別子の長さの上限である。
	maxConnectionIdBytes = 64
	// maxSelectionBytes は presence の selection の JSON の大きさの上限である。
	maxSelectionBytes = 2 << 10
)

// 接続を閉じる理由。closed のイベントの reason の値である。
const (
	syncClosedDeleted = "deleted"
	syncClosedRevoked = "revoked"
)

// WorkspaceHub は、ワークスペースの接続中の利用者へ、変更と presence を配信する。server に 1 つ作り、
// NewWorkspaceHandler へ渡す。presence はメモリだけに置く。
//
// **遅い接続が書き込みとほかの接続を止めない。** 配信は接続ごとの buffer (syncSendBuffer 件) へ
// 待たずに入れ、満ちた接続は閉じる。client は接続し直して snapshot から始める。
type WorkspaceHub struct {
	// mu は接続の集合と、ワークスペースへの書き込みから配信までを 1 つの順に並べる。書き込みの順と
	// 配信の順が一致し、接続の直後の snapshot と以後の change の間に抜けも重なりも無い。
	// ponytail: server 全体で 1 つの lock である。書き込みが多くなればワークスペースごとの lock へ分ける。
	mu          sync.Mutex
	connections map[string][]*syncConnection
}

// NewWorkspaceHub は接続を持たない hub を返す。
func NewWorkspaceHub() *WorkspaceHub {
	return &WorkspaceHub{connections: map[string][]*syncConnection{}}
}

// syncConnection は SSE の接続 1 本である。
type syncConnection struct {
	id          string
	login       string
	displayName string
	selection   json.RawMessage
	following   *string
	send        chan syncMessage
	// dropped は、buffer が満ちて hub が接続を外したときに閉じる。
	dropped chan struct{}
}

// syncMessage は送る 1 件の SSE のイベントである。final のイベントを送った接続は閉じる。
type syncMessage struct {
	data  []byte
	final bool
}

type syncSnapshot struct {
	Type      string          `json:"type"`
	Revision  int64           `json:"revision"`
	State     json.RawMessage `json:"state"`
	UpdatedBy string          `json:"updatedBy"`
	Name      string          `json:"name"`
}

// syncChange は変更 1 件のイベントである。Name は全体の置き換え (fields が ["*"]) だけが持つ。
type syncChange struct {
	Type           string                     `json:"type"`
	Revision       int64                      `json:"revision"`
	Fields         []string                   `json:"fields"`
	Patch          map[string]json.RawMessage `json:"patch"`
	Actor          string                     `json:"actor"`
	ClientChangeId string                     `json:"clientChangeId"`
	Name           *string                    `json:"name,omitempty"`
}

type syncPresenceItem struct {
	ConnectionId string          `json:"connectionId"`
	Login        string          `json:"login"`
	DisplayName  string          `json:"displayName"`
	Selection    json.RawMessage `json:"selection"`
	Following    *string         `json:"following"`
}

type syncPresence struct {
	Type        string             `json:"type"`
	Connections []syncPresenceItem `json:"connections"`
}

type syncClosed struct {
	Type   string `json:"type"`
	Reason string `json:"reason"`
}

// changeRequestBody は欄ごとの変更の要求の本文である。
type changeRequestBody struct {
	ClientChangeId string          `json:"clientChangeId"`
	BaseRevision   *int64          `json:"baseRevision"`
	Patch          json.RawMessage `json:"patch"`
}

type changeResponse struct {
	Revision       int64  `json:"revision"`
	ClientChangeId string `json:"clientChangeId"`
	UpdatedBy      string `json:"updatedBy"`
}

// changeConflict は、同じ欄を変えた後の変更があるときの応答に足す値である。
type changeConflict struct {
	Fields    []string        `json:"fields"`
	Revision  int64           `json:"revision"`
	State     json.RawMessage `json:"state"`
	UpdatedBy string          `json:"updatedBy"`
}

type changeConflictResponse struct {
	core.ApiError
	Conflict changeConflict `json:"conflict"`
}

// presenceRequestBody は presence の要求の本文である。
type presenceRequestBody struct {
	ConnectionId string          `json:"connectionId"`
	Selection    json.RawMessage `json:"selection"`
	Following    *string         `json:"following"`
}

func marshalEvent(event any) []byte {
	data, err := json.Marshal(event)
	if err != nil {
		// 配信する型はどれも JSON にできる。
		panic(err)
	}
	return data
}

// commit は、hub の順の中で write を実行し、write が返したイベントを配信する。
func (h *WorkspaceHub) commit(id string, write func() (*syncMessage, error)) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	message, err := write()
	if err != nil || message == nil {
		return err
	}
	h.broadcastLocked(id, *message)
	return nil
}

// broadcastLocked は id の接続へ message を配信する。buffer が満ちた接続は外し、残りへ presence を
// 配信し直す。呼び出し側が mu を持つ。
func (h *WorkspaceHub) broadcastLocked(id string, message syncMessage) {
	for {
		kept := h.connections[id][:0]
		for _, connection := range h.connections[id] {
			select {
			case connection.send <- message:
				kept = append(kept, connection)
			default:
				close(connection.dropped)
			}
		}
		if len(kept) == len(h.connections[id]) {
			return
		}
		h.setLocked(id, kept)
		message = h.presenceLocked(id)
	}
}

func (h *WorkspaceHub) setLocked(id string, connections []*syncConnection) {
	if len(connections) == 0 {
		delete(h.connections, id)
		return
	}
	h.connections[id] = connections
}

// presenceLocked は id の接続の一覧のイベントを返す。呼び出し側が mu を持つ。
func (h *WorkspaceHub) presenceLocked(id string) syncMessage {
	event := syncPresence{Type: "presence", Connections: []syncPresenceItem{}}
	for _, connection := range h.connections[id] {
		selection := connection.selection
		if selection == nil {
			selection = json.RawMessage("null")
		}
		event.Connections = append(event.Connections, syncPresenceItem{ConnectionId: connection.id,
			Login: connection.login, DisplayName: connection.displayName, Selection: selection,
			Following: connection.following})
	}
	slices.SortFunc(event.Connections, func(a, b syncPresenceItem) int { return strings.Compare(a.ConnectionId, b.ConnectionId) })
	return syncMessage{data: marshalEvent(event)}
}

// subscribe は connection を id の接続に加え、snapshot を最初のイベントにし、presence を配信する。
// 同じ識別子の接続が同じ利用者にあれば、前の接続を外す。別の利用者にあれば errConnectionInUse を、
// ワークスペースが無ければ pipeline.ErrWorkspaceNotFound を返す。
func (h *WorkspaceHub) subscribe(id string, connection *syncConnection, snapshot func() (syncMessage, bool)) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	connections := h.connections[id]
	index := slices.IndexFunc(connections, func(other *syncConnection) bool { return other.id == connection.id })
	if index >= 0 && connections[index].login != connection.login {
		return errConnectionInUse
	}
	message, ok := snapshot()
	if !ok {
		return pipeline.ErrWorkspaceNotFound
	}
	if index >= 0 {
		close(connections[index].dropped)
		connections = slices.Delete(slices.Clone(connections), index, index+1)
	}
	connection.send <- message
	h.setLocked(id, append(connections, connection))
	h.broadcastLocked(id, h.presenceLocked(id))
	return nil
}

var (
	// errConnectionInUse は、別の利用者の接続が同じ connectionId を使っていることを表す。
	errConnectionInUse = errors.New("another account uses the connectionId")
	// errWorkspaceNotEditable は、書き込みの時点で利用者がワークスペースを変更できなくなったことを表す。
	errWorkspaceNotEditable = errors.New("the access to the workspace does not allow the operation")
)

// checkEditable は、hub の lock の中で、要求の利用者が id のワークスペースをまだ変更できるかを確かめ直す。
// 読めなければ pipeline.ErrWorkspaceNotFound、変更できなければ errWorkspaceNotEditable を返す。
func (h workspacesHandler) checkEditable(r *http.Request, id string) error {
	login := requestOwner(r)
	workspace, ok := h.store.Get(id)
	access, shared := accessOf(workspace, login)
	if !ok || !shared {
		return pipeline.ErrWorkspaceNotFound
	}
	if h.access != nil {
		if _, member := h.access.Role(login); !member {
			return pipeline.ErrWorkspaceNotFound
		}
	}
	if access != accessOwner && access != string(pipeline.WorkspaceEdit) {
		return errWorkspaceNotEditable
	}
	return nil
}

// unsubscribe は connection を外し、外したなら presence を配信する。
func (h *WorkspaceHub) unsubscribe(id string, connection *syncConnection) {
	h.mu.Lock()
	defer h.mu.Unlock()
	connections := h.connections[id]
	index := slices.Index(connections, connection)
	if index < 0 {
		return
	}
	h.setLocked(id, slices.Delete(slices.Clone(connections), index, index+1))
	h.broadcastLocked(id, h.presenceLocked(id))
}

// updatePresence は login の接続 connectionId の presence を置き換えて配信する。接続が無ければ偽を返す。
func (h *WorkspaceHub) updatePresence(id, login string, body presenceRequestBody) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	index := slices.IndexFunc(h.connections[id], func(connection *syncConnection) bool {
		return connection.id == body.ConnectionId && connection.login == login
	})
	if index < 0 {
		return false
	}
	connection := h.connections[id][index]
	connection.selection, connection.following = body.Selection, body.Following
	h.broadcastLocked(id, h.presenceLocked(id))
	return true
}

// changeMessage は変更のイベントを返す。
func changeMessage(change pipeline.WorkspaceChange, patch map[string]json.RawMessage, name *string) *syncMessage {
	return &syncMessage{data: marshalEvent(syncChange{Type: "change", Revision: change.Revision, Fields: change.Fields,
		Patch: patch, Actor: change.Actor, ClientChangeId: change.ClientChangeId, Name: name})}
}

// replacedMessage は全体の置き換えのイベントを返す。patch は state 全体を持つ。
func replacedMessage(workspace pipeline.Workspace, change pipeline.WorkspaceChange) *syncMessage {
	var state map[string]json.RawMessage
	if err := json.Unmarshal(workspace.State, &state); err != nil {
		state = map[string]json.RawMessage{}
	}
	return changeMessage(change, state, &workspace.Name)
}

func closedMessage(reason string) *syncMessage {
	return &syncMessage{data: marshalEvent(syncClosed{Type: "closed", Reason: reason}), final: true}
}

// change は state の最上位の欄を置き換える変更を適用し、接続中の利用者へ配信する。
func (h workspacesHandler) change(w http.ResponseWriter, r *http.Request) {
	workspace, access, ok := h.readable(w, r)
	if !ok || !permitted(w, access, accessOwner, string(pipeline.WorkspaceEdit)) {
		return
	}
	invalid := func(message string) {
		writeError(w, http.StatusBadRequest, core.ApiError{Code: core.ApiErrorCodeInvalidRequest, Message: message})
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var body changeRequestBody
	if err := decoder.Decode(&body); err != nil || decoder.Decode(new(any)) != io.EOF {
		invalid("the request body is not a readable change")
		return
	}
	var patch map[string]json.RawMessage
	if body.BaseRevision == nil || !bytes.HasPrefix(bytes.TrimSpace(body.Patch), []byte("{")) ||
		json.Unmarshal(body.Patch, &patch) != nil {
		invalid("baseRevision and a patch object are required")
		return
	}
	var result pipeline.WorkspaceChangeResult
	err := h.hub.commit(workspace.Id, func() (*syncMessage, error) {
		if err := h.checkEditable(r, workspace.Id); err != nil {
			return nil, err
		}
		var err error
		result, err = h.store.Change(workspace.Id, requestOwner(r), body.ClientChangeId, *body.BaseRevision, patch)
		if err != nil || result.Replayed {
			return nil, err
		}
		return changeMessage(result.Change, patch, nil), nil
	})
	var conflict *pipeline.WorkspaceConflictError
	if errors.As(err, &conflict) {
		writeJSON(w, http.StatusConflict, changeConflictResponse{
			ApiError: core.ApiError{Code: core.ApiErrorCodeWorkspaceChanged,
				Message: "a change after the base revision changed the same fields"},
			Conflict: changeConflict{Fields: conflict.Fields, Revision: conflict.Current.Revision,
				State: conflict.Current.State, UpdatedBy: conflict.Current.UpdatedBy},
		})
		return
	}
	if err != nil {
		writeWorkspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, changeResponse{Revision: result.Change.Revision,
		ClientChangeId: result.Change.ClientChangeId, UpdatedBy: result.Change.Actor})
}

// presence は、要求の利用者の接続の選択と追従先を置き換えて配信する。
func (h workspacesHandler) presence(w http.ResponseWriter, r *http.Request) {
	workspace, _, ok := h.readable(w, r)
	if !ok {
		return
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, requestBodyLimit))
	decoder.DisallowUnknownFields()
	var body presenceRequestBody
	if err := decoder.Decode(&body); err != nil || decoder.Decode(new(any)) != io.EOF ||
		len(body.Selection) > maxSelectionBytes ||
		(body.Following != nil && (*body.Following == "" || len(*body.Following) > maxConnectionIdBytes)) {
		writeError(w, http.StatusBadRequest, core.ApiError{
			Code: core.ApiErrorCodeInvalidRequest, Message: "the request body is not a readable presence",
		})
		return
	}
	if bytes.Equal(bytes.TrimSpace(body.Selection), []byte("null")) {
		body.Selection = nil
	}
	if !h.hub.updatePresence(workspace.Id, requestOwner(r), body) {
		writeError(w, http.StatusNotFound, core.ApiError{
			Code: core.ApiErrorCodeRecordNotFound, Message: "no connection of the account matches the connectionId",
		})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// events は、ワークスペースの snapshot、変更、presence を SSE で送り続ける。
//
// **各イベントと keepalive を送る前に、利用者がまだ読めるかを確かめる。** 共有、調査の役割、
// セッション (アカウントの無効化とセッションの失効を含む) のどれかを失っていれば closed を送って閉じる。
// 別の process (CLI) がアカウントを無効にしたときも、次のイベントか keepalive の時点で検出する。
// 検出までの時間は最大で keepalive の間隔 (syncKeepalive) である。要求の context が終われば戻る。
func (h workspacesHandler) events(w http.ResponseWriter, r *http.Request) {
	workspace, _, ok := h.readable(w, r)
	if !ok {
		return
	}
	connectionId := r.URL.Query().Get("connectionId")
	if connectionId == "" || len(connectionId) > maxConnectionIdBytes {
		writeError(w, http.StatusBadRequest, core.ApiError{
			Code: core.ApiErrorCodeInvalidRequest, Message: "connectionId must have 1 to 64 bytes",
		})
		return
	}
	login, displayName := requestOwner(r), localOwner
	if account, signedIn := requestAccount(r); signedIn {
		displayName = account.DisplayName
	}
	connection := &syncConnection{id: connectionId, login: login, displayName: displayName,
		send: make(chan syncMessage, syncSendBuffer), dropped: make(chan struct{})}
	err := h.hub.subscribe(workspace.Id, connection, func() (syncMessage, bool) {
		current, ok := h.store.Get(workspace.Id)
		return syncMessage{data: marshalEvent(syncSnapshot{Type: "snapshot", Revision: current.Revision,
			State: current.State, UpdatedBy: current.UpdatedBy, Name: current.Name})}, ok
	})
	if errors.Is(err, errConnectionInUse) {
		writeError(w, http.StatusConflict, core.ApiError{
			Code: core.ApiErrorCodeInvalidRequest, Message: "another account uses the connectionId",
		})
		return
	}
	if err != nil {
		writeWorkspaceError(w, err)
		return
	}
	defer h.hub.unsubscribe(workspace.Id, connection)
	controller := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	// server の WriteTimeout が長い接続を切らない。
	_ = controller.SetWriteDeadline(time.Time{})
	send := func(data []byte) bool {
		if _, err := w.Write(data); err != nil {
			return false
		}
		return controller.Flush() == nil
	}
	if !send(nil) {
		return
	}
	keepalive := time.NewTicker(syncKeepalive)
	defer keepalive.Stop()
	for {
		var message syncMessage
		select {
		case <-r.Context().Done():
			return
		case <-connection.dropped:
			return
		case message = <-connection.send:
		case <-keepalive.C:
			message = syncMessage{}
		}
		if reason := h.lostReason(r, workspace.Id); reason != "" && !message.final {
			// #nosec G706 -- output.Sanitize が制御文字と改行を除いてから log へ渡す。
			slog.Info("closed a workspace connection", "reason", reason, "login", output.Sanitize(login))
			send(append(append([]byte("data: "), closedMessage(reason).data...), '\n', '\n'))
			return
		}
		if message.data == nil {
			if !send([]byte(": keepalive\n\n")) {
				return
			}
			continue
		}
		if !send(append(append([]byte("data: "), message.data...), '\n', '\n')) || message.final {
			return
		}
	}
}

// lostReason は、要求の利用者がワークスペースを読めなくなった理由を返す。読めるなら空文字列である。
func (h workspacesHandler) lostReason(r *http.Request, id string) string {
	workspace, ok := h.store.Get(id)
	if !ok {
		return syncClosedDeleted
	}
	login := requestOwner(r)
	if _, readable := accessOf(workspace, login); !readable {
		return syncClosedRevoked
	}
	if h.access != nil {
		if _, member := h.access.Role(login); !member {
			return syncClosedRevoked
		}
	}
	if h.accounts != nil {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			return syncClosedRevoked
		}
		if account, err := h.accounts.SessionAccount(r.Context(), cookie.Value); err != nil || account.Login != login {
			return syncClosedRevoked
		}
	}
	return ""
}
