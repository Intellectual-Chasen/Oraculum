package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/output"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// terminalsHandler は、端末 1 台の Security のイベントを取り込んだ handler を返す。
// 198.51.100.9 から 3 回失敗し 1 回成功する。
func terminalsHandler(t *testing.T) http.Handler {
	t.Helper()
	event := func(recordID, eventID, second, data string) string {
		return `<Event><System><Provider Name="Microsoft-Windows-Security-Auditing"/><EventID>` + eventID +
			`</EventID><TimeCreated SystemTime="2001-02-03T04:05:0` + second + `Z"/><EventRecordID>` + recordID +
			`</EventRecordID><Channel>Security</Channel><Computer>host09.example.test</Computer></System>` +
			`<EventData>` + data + `</EventData></Event>`
	}
	logon := `<Data Name="TargetUserName">user91</Data><Data Name="TargetDomainName">EXAMPLE</Data>` +
		`<Data Name="LogonType">3</Data><Data Name="IpAddress">198.51.100.9</Data>`
	document := strings.Join([]string{"<Events>",
		event("91", "4625", "1", logon), event("92", "4625", "2", logon), event("93", "4625", "3", logon),
		event("94", "4624", "4", logon+`<Data Name="TargetLogonId">0x94</Data>`), "</Events>"}, "\n")
	registry, err := pipeline.NewFormatRegistry(pipeline.WindowsEventFormats())
	if err != nil {
		t.Fatal(err)
	}
	runner, err := pipeline.NewRunner(pipeline.Config{
		Open:    func(string) (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(document)), nil },
		Parsers: registry, Minter: pipeline.DigestMinter{}, Ordinals: pipeline.NewInMemoryOrdinals(),
		Sanitize: output.Sanitize, Revision: "api-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := runner.Run([]pipeline.SourcePlan{
		{FormatKey: "windows_event_xml", FileName: "events.xml", OriginPath: "events.xml"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return testHandler(result)
}

// getTerminalJSON は path の応答を status と共に読み、本体を target へ入れる。
func getTerminalJSON(t *testing.T, handler http.Handler, path string, status int, target any) {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, withAllMatchConditions(path), nil))
	if response.Code != status {
		t.Fatalf("%s status=%d body=%s", path, response.Code, response.Body.String())
	}
	if target != nil {
		if err := json.Unmarshal(response.Body.Bytes(), target); err != nil {
			t.Fatal(err)
		}
	}
}

// 一覧、詳細、レコードの 3 つの操作が、接続元 IP ごとの失敗の件数と、時刻順のレコードの続きを返す。
func TestTerminalOperationsReturnTheRemoteLogons(t *testing.T) {
	handler := terminalsHandler(t)
	var list struct {
		Terminals []struct {
			Node                   struct{ Id string } `json:"node"`
			RecordCount            int64               `json:"recordCount"`
			CategorizedRecordCount int64               `json:"categorizedRecordCount"`
		} `json:"terminals"`
	}
	getTerminalJSON(t, handler, "/api/v0/terminals", http.StatusOK, &list)
	if len(list.Terminals) != 1 || list.Terminals[0].CategorizedRecordCount != 4 {
		t.Fatalf("the terminals are %+v", list.Terminals)
	}
	id := url.PathEscape(list.Terminals[0].Node.Id)

	var detail struct {
		RemoteLogons []struct {
			SourceIp     string `json:"sourceIp"`
			FailureCount int64  `json:"failureCount"`
			SuccessCount int64  `json:"successCount"`
		} `json:"remoteLogons"`
		Categories []struct {
			Category       string   `json:"category"`
			RecordCount    int64    `json:"recordCount"`
			PresentSources []string `json:"presentSources"`
		} `json:"categories"`
	}
	getTerminalJSON(t, handler, "/api/v0/terminals/"+id, http.StatusOK, &detail)
	if len(detail.RemoteLogons) != 1 || detail.RemoteLogons[0].SourceIp != "198.51.100.9" ||
		detail.RemoteLogons[0].FailureCount != 3 || detail.RemoteLogons[0].SuccessCount != 1 {
		t.Errorf("the remote logons are %+v", detail.RemoteLogons)
	}
	for _, category := range detail.Categories {
		if category.Category == "installation" && (category.RecordCount != 0 || len(category.PresentSources) != 0) {
			t.Errorf("installation is %+v, want no record and no source", category)
		}
	}

	var events struct {
		Events []struct{ EventTime struct{ Normalized string } } `json:"events"`
	}
	getTerminalJSON(t, handler, "/api/v0/terminals/"+id+"/events?category=remote_logon&sourceIp=198.51.100.9",
		http.StatusOK, &events)
	if len(events.Events) != 4 || events.Events[0].EventTime.Normalized != "2001-02-03T04:05:01Z" {
		t.Errorf("the events are %+v", events)
	}
	getTerminalJSON(t, handler, "/api/v0/terminals/"+id+"/events?category=unknown", http.StatusBadRequest, nil)
	for _, param := range []string{"limit=3", "cursor=3"} {
		getTerminalJSON(t, handler, "/api/v0/terminals/"+id+"/events?"+param, http.StatusBadRequest, nil)
	}
	getTerminalJSON(t, handler, "/api/v0/terminals/absent", http.StatusNotFound, nil)
}
