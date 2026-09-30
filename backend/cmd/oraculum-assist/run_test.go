// in-package test: 非公開の parseArgs に起動引数を渡す。
package main

import (
	"bytes"
	"strings"
	"testing"
)

// server の URL は http か https の origin だけを受け付ける。
func TestParseArgs(t *testing.T) {
	for _, accepted := range []string{"http://127.0.0.1:8080", "https://oraculum.example.test/", "http://[::1]:8080"} {
		server, err := parseArgs([]string{serverFlag, accepted})
		if err != nil {
			t.Errorf("%s: %v", accepted, err)
			continue
		}
		if server.Path != "" {
			t.Errorf("%s: path = %q, want the origin only", accepted, server.Path)
		}
	}
	for _, rejected := range [][]string{
		{}, {serverFlag}, {"--other", "http://127.0.0.1:8080"},
		{serverFlag, "ftp://127.0.0.1"}, {serverFlag, "127.0.0.1:8080"},
		{serverFlag, "http://127.0.0.1:8080/api/v0"}, {serverFlag, "http://127.0.0.1:8080/?x=1"},
		{serverFlag, "http://user:pass@127.0.0.1:8080"},
		{serverFlag, "http://127.0.0.1:8080", serverFlag, "http://127.0.0.1:8081"},
	} {
		if _, err := parseArgs(rejected); err == nil {
			t.Errorf("%q was accepted", rejected)
		}
	}
}

// 引数の誤りは使い方を出して止まる。
func TestRunPrintsTheUsageOnABadArgument(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--other"}, &stdout, &stderr); code != 1 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(stderr.String(), usage) {
		t.Errorf("stderr = %q, want the usage", stderr.String())
	}
}
