package api_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

const terminalAssignmentsPath = "/api/v0/terminal-assignments"

// 分析者が与えた割当を記録し、一覧で読み返せる。
func TestTerminalAssignmentsEndpointRecordsTheAnalystAssignment(t *testing.T) {
	handler := graphHandler(t)

	created := requestJSON(t, handler, http.MethodPost, terminalAssignmentsPath,
		analystAssignmentBody(t, handler), http.StatusCreated)
	var assignment core.TerminalAssignment
	decodeInto(t, created, &assignment)

	if assignment.Origin != core.TerminalAssignmentOriginAnalystSupplied {
		t.Errorf("the origin is %q, want %q",
			assignment.Origin, core.TerminalAssignmentOriginAnalystSupplied)
	}
	if assignment.Derivation == "" {
		t.Error("the assignment carries no derivation, want the analyst reasoning")
	}
	if len(assignment.BasisRecordRefs) == 0 {
		t.Error("the assignment carries no basis record, want the records the analyst named")
	}

	listed := requestJSON(t, handler, http.MethodGet, terminalAssignmentsPath, nil, http.StatusOK)
	var response struct {
		Assignments     []core.TerminalAssignment `json:"assignments"`
		AssignmentCount int64                     `json:"assignmentCount"`
	}
	decodeInto(t, listed, &response)
	if response.AssignmentCount != int64(len(response.Assignments)) {
		t.Errorf("the response counts %d assignments and carries %d",
			response.AssignmentCount, len(response.Assignments))
	}
	if len(response.Assignments) != 1 {
		t.Fatalf("the listing holds %d assignments, want 1", len(response.Assignments))
	}
	if response.Assignments[0].ClientIp != assignment.ClientIp {
		t.Errorf("the listed assignment is for %q, want %q",
			response.Assignments[0].ClientIp, assignment.ClientIp)
	}
}

// 同じ内容の割当の 2 回目を 409 で拒み、一覧に 1 件だけ残す。期間だけが違う割当は記録する。
func TestTerminalAssignmentsEndpointRejectsTheSameAssignmentTwice(t *testing.T) {
	handler := graphHandler(t)
	body := analystAssignmentMap(t, handler)
	requestJSON(t, handler, http.MethodPost, terminalAssignmentsPath,
		encodeBody(t, body), http.StatusCreated)
	rejected := requestJSON(t, handler, http.MethodPost, terminalAssignmentsPath,
		encodeBody(t, body), http.StatusConflict)
	var apiError core.ApiError
	decodeInto(t, rejected, &apiError)
	if apiError.Code != core.ApiErrorCodeTerminalAssignmentAlreadyRecorded {
		t.Errorf("the code is %q, want %q", apiError.Code, core.ApiErrorCodeTerminalAssignmentAlreadyRecorded)
	}
	body["author"] = "another analyst"
	requestJSON(t, handler, http.MethodPost, terminalAssignmentsPath,
		encodeBody(t, body), http.StatusConflict)

	validRange := body["assignmentValidRange"].(map[string]any)
	validRange["to"] = validRange["from"]
	requestJSON(t, handler, http.MethodPost, terminalAssignmentsPath,
		encodeBody(t, body), http.StatusCreated)

	listed := requestJSON(t, handler, http.MethodGet, terminalAssignmentsPath, nil, http.StatusOK)
	var response struct {
		Assignments []core.TerminalAssignment `json:"assignments"`
	}
	decodeInto(t, listed, &response)
	if len(response.Assignments) != 2 {
		t.Errorf("the listing holds %d assignments, want 2", len(response.Assignments))
	}
}

// 接続元 IP、端末の識別子、端末の表示名は、分かるものだけで記録できる。
func TestTerminalAssignmentsEndpointRecordsAnySubsetOfTheTerminalItems(t *testing.T) {
	cases := map[string]func(map[string]any){
		"接続元 IP を省き、収集元に付ける": func(body map[string]any) {
			delete(body, "clientIp")
			body["appliesToSourceId"] = body["sourceId"]
		},
		"表示名だけを与える": func(body map[string]any) {
			delete(body, "clientIp")
			delete(body, "terminalId")
			body["appliesToSourceId"] = body["sourceId"]
		},
		"接続元 IP だけを与える": func(body map[string]any) {
			delete(body, "terminalId")
			delete(body, "terminalHostname")
			body["appliesToSourceId"] = body["sourceId"]
		},
		"端末の識別子と表示名だけを与え、収集元に付けない": func(body map[string]any) {
			delete(body, "clientIp")
		},
	}
	for name, shape := range cases {
		t.Run(name, func(t *testing.T) {
			handler := graphHandler(t)
			body := analystAssignmentMap(t, handler)
			shape(body)
			created := requestJSON(t, handler, http.MethodPost, terminalAssignmentsPath,
				encodeBody(t, body), http.StatusCreated)
			var assignment core.TerminalAssignment
			decodeInto(t, created, &assignment)
			for item, got := range map[string]string{
				"clientIp": assignment.ClientIp, "terminalId": assignment.TerminalId,
				"terminalHostname": assignment.TerminalHostname,
			} {
				want, _ := body[item].(string)
				if got != want {
					t.Errorf("the recorded %s is %q, want %q", item, got, want)
				}
			}
			// 省いた項目は応答の JSON に出ない。
			var raw map[string]any
			decodeInto(t, created, &raw)
			for _, item := range []string{"clientIp", "terminalId", "terminalHostname"} {
				if _, sent := body[item]; !sent {
					if _, present := raw[item]; present {
						t.Errorf("the response carries the omitted item %q", item)
					}
				}
			}
		})
	}
}

// 分析者が記録した、端末が名乗るホスト名の並びを保存して返す。
func TestTerminalAssignmentsEndpointRecordsTheHostnames(t *testing.T) {
	handler := graphHandler(t)
	body := analystAssignmentMap(t, handler)
	body["terminalHostnames"] = []string{"pc01", "pc01.example.test"}
	created := requestJSON(t, handler, http.MethodPost, terminalAssignmentsPath,
		encodeBody(t, body), http.StatusCreated)
	var assignment core.TerminalAssignment
	decodeInto(t, created, &assignment)
	if strings.Join(assignment.TerminalHostnames, ",") != "pc01,pc01.example.test" {
		t.Errorf("the recorded hostnames are %q, want the two names", assignment.TerminalHostnames)
	}
	body["terminalHostnames"] = []string{"pc 01"}
	requestJSON(t, handler, http.MethodPost, terminalAssignmentsPath, encodeBody(t, body), http.StatusBadRequest)
}

// 導いた筋道と根拠のレコードを欠く割当、端末の 3 項目をすべて欠く割当、読めない IP、
// 取り込み結果に無い収集元を指す割当を保存しない。
func TestTerminalAssignmentsEndpointRejectsAnAssignmentWithoutItsBasis(t *testing.T) {
	handler := graphHandler(t)
	cases := map[string]func(map[string]any){
		"導いた筋道が無い":   func(body map[string]any) { delete(body, "derivation") },
		"根拠のレコードが無い": func(body map[string]any) { delete(body, "basisRecordRefs") },
		"記録した分析者が無い": func(body map[string]any) { delete(body, "author") },
		"端末の 3 項目が無い": func(body map[string]any) {
			delete(body, "clientIp")
			delete(body, "terminalId")
			delete(body, "terminalHostname")
			body["appliesToSourceId"] = body["sourceId"]
		},
		"接続元 IP も付ける収集元もホスト名も無い": func(body map[string]any) {
			delete(body, "clientIp")
			delete(body, "terminalHostname")
		},
		"接続元 IP が IP アドレスとして読めない": func(body map[string]any) {
			body["clientIp"] = "198.51.100.300"
		},
		"期間の収集元が無い": func(body map[string]any) {
			body["sourceId"] = "source-the-investigation-does-not-hold"
		},
		"期間の収集元の内容の識別が異なる": func(body map[string]any) {
			body["sourceContentSha256"] = strings.Repeat("0", 64)
		},
		"端末を付ける収集元が無い": func(body map[string]any) {
			body["appliesToSourceId"] = "source-the-investigation-does-not-hold"
		},
	}
	for name, without := range cases {
		t.Run(name, func(t *testing.T) {
			body := analystAssignmentMap(t, handler)
			without(body)
			response := requestJSON(t, handler, http.MethodPost, terminalAssignmentsPath,
				encodeBody(t, body), http.StatusBadRequest)
			var apiError core.ApiError
			decodeInto(t, response, &apiError)
			if apiError.Code != core.ApiErrorCodeInvalidRequest {
				t.Errorf("the error code is %q, want %q",
					apiError.Code, core.ApiErrorCodeInvalidRequest)
			}
		})
	}
}

// 保存先の内部の失敗を、要求の誤りとして返さない。
//
// 分析者が入力を直しても記録できない状態を、入力の誤りとして出さない。
func TestTerminalAssignmentsEndpointReportsAStoreFailureAsInternal(t *testing.T) {
	result := graphImportResult(t)
	body := analystAssignmentMap(t, testHandler(result))

	store := &failingAssignmentStore{
		result: result, failure: errors.New("the store is unreachable"),
	}
	handler := handlerOf(store)
	response := requestJSON(t, handler, http.MethodPost, terminalAssignmentsPath,
		encodeBody(t, body), http.StatusInternalServerError)
	var apiError core.ApiError
	decodeInto(t, response, &apiError)
	if apiError.Code != core.ApiErrorCodeInternalError {
		t.Errorf("the error code is %q, want %q",
			apiError.Code, core.ApiErrorCodeInternalError)
	}

	// 要求起因の失敗は今までどおり 400 で返る。
	store.failure = fmt.Errorf("%w: the assignment is incomplete",
		pipeline.ErrTerminalAssignmentNotStorable)
	response = requestJSON(t, handler, http.MethodPost, terminalAssignmentsPath,
		encodeBody(t, body), http.StatusBadRequest)
	decodeInto(t, response, &apiError)
	if apiError.Code != core.ApiErrorCodeInvalidRequest {
		t.Errorf("the error code is %q, want %q",
			apiError.Code, core.ApiErrorCodeInvalidRequest)
	}
}

// failingAssignmentStore は割当の記録が必ず失敗する保存先である。
type failingAssignmentStore struct {
	result  pipeline.ImportResult
	failure error
}

func (s *failingAssignmentStore) ImportResult() pipeline.ImportResult {
	return s.result
}

func (s *failingAssignmentStore) Assertions() pipeline.AssertionStore {
	return pipeline.NewMemoryAssertionStore(&testAssertionClock{})
}

func (s *failingAssignmentStore) TerminalAssignments() pipeline.TerminalAssignmentStore {
	return s
}

func (s *failingAssignmentStore) Assist() pipeline.AssistStore {
	return pipeline.UnavailableAssistStore()
}

func (s *failingAssignmentStore) AssistProposals() pipeline.AssistProposalStore {
	return pipeline.NewUnavailableAssistProposalStore()
}

func (s *failingAssignmentStore) List() ([]core.TerminalAssignment, int64) {
	return nil, 0
}

func (s *failingAssignmentStore) Create(
	pipeline.TerminalAssignmentDraft,
) (core.TerminalAssignment, error) {
	return core.TerminalAssignment{}, s.failure
}

func (s *failingAssignmentStore) Revision() int64 { return 0 }

// 未知の項目を持つ要求を退ける。
func TestTerminalAssignmentsEndpointRejectsAnUnknownItem(t *testing.T) {
	handler := graphHandler(t)
	body := analystAssignmentMap(t, handler)
	body["origin"] = string(core.TerminalAssignmentOriginObservedInSource)
	requestJSON(t, handler, http.MethodPost, terminalAssignmentsPath,
		encodeBody(t, body), http.StatusBadRequest)
}

// analystAssignmentMap は、取り込み済みの収集元を材料に分析者の割当の本文を組む。
//
// 適用期間はその収集元の観測期間である。根拠のレコードは同じ収集元の最初のレコードを
// 指す。**原資料の文字列を写していない。** 値は取り込み結果から取り出す。
func analystAssignmentMap(t *testing.T, handler http.Handler) map[string]any {
	t.Helper()
	sources := decodeSources(t, requestSources(t, handler, "/api/v0/sources"),
		http.StatusOK)
	if len(sources.Sources) == 0 {
		t.Fatal("the import result holds no source")
	}
	identity := sources.Sources[0].Source
	if identity.ObservedRangeFirst == nil || identity.ObservedRangeLast == nil {
		t.Fatalf("the source %q holds no observed range", identity.FileName)
	}
	return map[string]any{
		"clientIp":            "198.51.100.77",
		"terminalId":          "terminal-supplied-by-the-analyst",
		"terminalHostname":    "linux-host.example.test",
		"sourceId":            identity.SourceId,
		"sourceContentSha256": identity.ContentSha256,
		"assignmentValidRange": map[string]any{
			"from": identity.ObservedRangeFirst,
			"to":   identity.ObservedRangeLast,
		},
		"derivation": "別の端末の ssh の接続先と、接続先 port から導いた",
		"basisRecordRefs": []map[string]any{{
			"sourceContentSha256": identity.ContentSha256,
			"positionKind":        string(core.PositionKindLineNumber),
			"lineNumber":          1,
		}},
		"author": "analyst",
	}
}

func analystAssignmentBody(t *testing.T, handler http.Handler) []byte {
	t.Helper()
	return encodeBody(t, analystAssignmentMap(t, handler))
}

// requestJSON は 1 つの要求を送り、期待した status の応答の本文を返す。
func requestJSON(
	t *testing.T, handler http.Handler, method, path string, body []byte, wantStatus int,
) []byte {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader(body)
	}
	request := httptest.NewRequest(method, path, reader)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != wantStatus {
		t.Fatalf("%s %s returned %d, want %d. body: %s",
			method, path, recorder.Code, wantStatus, recorder.Body.String())
	}
	return recorder.Body.Bytes()
}

func decodeInto(t *testing.T, body []byte, into any) {
	t.Helper()
	if err := json.Unmarshal(body, into); err != nil {
		t.Fatalf("decoding the response %s: %v", body, err)
	}
}

func encodeBody(t *testing.T, body map[string]any) []byte {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("encoding the request body: %v", err)
	}
	return encoded
}
