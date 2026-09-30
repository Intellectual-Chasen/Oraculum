package api

import (
	"strconv"
	"testing"
)

// buffer が満ちた接続を閉じても、書き込みとほかの接続は止まらない。
func TestWorkspaceHubDropsAFullConnectionWithoutBlocking(t *testing.T) {
	hub := NewWorkspaceHub()
	connection := func(id string) *syncConnection {
		return &syncConnection{id: id, login: "u", send: make(chan syncMessage, syncSendBuffer), dropped: make(chan struct{})}
	}
	slow, fast := connection("slow"), connection("fast")
	snapshot := func() (syncMessage, bool) { return syncMessage{data: []byte(`{}`)}, true }
	if hub.subscribe("ws", slow, snapshot) != nil || hub.subscribe("ws", fast, snapshot) != nil {
		t.Fatal("the subscription was refused")
	}
	received := 0
	drain := func() {
		for {
			select {
			case <-fast.send:
				received++
			default:
				return
			}
		}
	}
	// commit は待たずに戻る。slow が読まなくても、書き込みは最後まで進む。
	for index := range syncSendBuffer * 2 {
		data := []byte(strconv.Itoa(index))
		if err := hub.commit("ws", func() (*syncMessage, error) { return &syncMessage{data: data}, nil }); err != nil {
			t.Fatal(err)
		}
		drain()
	}
	select {
	case <-slow.dropped:
	default:
		t.Fatal("the full connection was kept")
	}
	// fast は snapshot・自分の接続の presence・書き込み・slow を外した後の presence を受け取る。
	if received != 1+1+syncSendBuffer*2+1 {
		t.Fatalf("received=%d", received)
	}
}
