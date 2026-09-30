package apache_test

import (
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/apache"
)

func TestParseAccessRequestLine(t *testing.T) {
	record := accessRecordOf(t, fullShapeLine)
	requestLine, failure := apache.ParseAccessRequestLine(record)
	if failure != nil {
		t.Fatalf("ParseAccessRequestLine() returned failure %+v", failure)
	}
	if requestLine.Method != "GET" {
		t.Errorf("Method = %q, want GET", requestLine.Method)
	}
	if requestLine.Target != "/" {
		t.Errorf("Target = %q, want /", requestLine.Target)
	}
	if requestLine.Protocol != "HTTP/1.1" {
		t.Errorf("Protocol = %q, want HTTP/1.1", requestLine.Protocol)
	}
}

func TestParseAccessRequestLineWithQueryString(t *testing.T) {
	line := `203.0.113.10 - - [01/Feb/2000:11:20:15 +0900] "POST /handler.exe?a=1&b=2 HTTP/1.1" 200 100 "-" "-"`
	record := accessRecordOf(t, line)
	requestLine, failure := apache.ParseAccessRequestLine(record)
	if failure != nil {
		t.Fatalf("ParseAccessRequestLine() returned failure %+v", failure)
	}
	if want := "/handler.exe?a=1&b=2"; requestLine.Target != want {
		t.Errorf("Target = %q, want %q", requestLine.Target, want)
	}
}

func TestParseAccessRequestLineRejectsMalformed(t *testing.T) {
	cases := map[string]string{
		"missing protocol": `203.0.113.10 - - [01/Feb/2000:11:20:10 +0900] "GET /" 200 12 "-" "-"`,
		"invalid protocol": `203.0.113.10 - - [01/Feb/2000:11:20:10 +0900] "GET / FTP/1.1" 200 12 "-" "-"`,
	}
	for name, line := range cases {
		t.Run(name, func(t *testing.T) {
			record := accessRecordOf(t, line)
			_, failure := apache.ParseAccessRequestLine(record)
			if failure == nil {
				t.Fatal("ParseAccessRequestLine() did not return a failure")
			}
			if failure.Stage != "normalize" {
				t.Errorf("failure.Stage = %q, want normalize", failure.Stage)
			}
		})
	}
}
