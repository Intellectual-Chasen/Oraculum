package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// ログインの失敗は、loginFailureWindow の期間の中で上限の回数を数えると退け、その期間を過ぎると数え直す。
func TestLoginFailuresAreLimitedWithinTheWindow(t *testing.T) {
	clock := time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)
	handler := NewSessionHandler(nil, nil).(*sessionHandler)
	handler.now = func() time.Time { return clock }
	for range loginFailureLimit {
		release, ok := handler.reserve("alice")
		if !ok {
			t.Fatal("refused before the limit")
		}
		release(true)
	}
	if _, ok := handler.reserve("alice"); ok {
		t.Fatal("allowed over the limit")
	}
	release, ok := handler.reserve("bob")
	if !ok {
		t.Fatal("the limit of another login applied")
	}
	release(false)
	clock = clock.Add(loginFailureWindow)
	release, ok = handler.reserve("alice")
	if !ok {
		t.Fatal("refused after the window")
	}
	release(false)
	if len(handler.failures) != 0 || len(handler.inFlight) != 0 {
		t.Fatalf("records kept: failures=%v inFlight=%v", handler.failures, handler.inFlight)
	}
}

// hash を求めるログインが CPU の数だけ実行中なら、次のログインを 429 で退ける。
func TestLoginsBeyondTheHashingSlotsAreRefused(t *testing.T) {
	handler := NewSessionHandler(nil, stubAccounts{}).(*sessionHandler)
	for range cap(handler.hashing) {
		handler.hashing <- struct{}{}
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, sessionPath,
		strings.NewReader(`{"login":"alice","password":"alice-password"}`)))
	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body)
	}
}

// stubAccounts は、どの操作も実装しない Accounts である。操作を呼ぶと panic する。
type stubAccounts struct{ Accounts }

// 同じログイン名へ同時に送ったログインも、上限の数までしか試せない。
func TestConcurrentLoginsDoNotExceedTheLimit(t *testing.T) {
	handler := NewSessionHandler(nil, nil).(*sessionHandler)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var releases []func(bool)
	refused := 0
	for range 50 {
		wg.Go(func() {
			release, ok := handler.reserve("alice")
			mu.Lock()
			defer mu.Unlock()
			if ok {
				releases = append(releases, release)
			} else {
				refused++
			}
		})
	}
	wg.Wait()
	if len(releases) != loginFailureLimit || refused != 50-loginFailureLimit {
		t.Fatalf("reserved=%d refused=%d", len(releases), refused)
	}
	for _, release := range releases {
		release(true)
	}
	if _, ok := handler.reserve("alice"); ok {
		t.Fatal("allowed after the failures")
	}
}
