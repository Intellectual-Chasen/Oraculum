package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/output"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// runFixtureDir は pipeline と同じ byte 列を読む。pipeline と api の検査が同じ fixture と
// 期待値の manifest を共有するため、internal/testdata/run に集約する。
const runFixtureDir = "../internal/testdata/run/"

// sourcesResponse は`/api/v0/sources` の応答の項目名を test 側で固定する。decodeJSON が
// DisallowUnknownFields で読むため、handler が項目を足すと test が失敗する。
type sourcesResponse struct {
	Sources      []sourceResponseItem `json:"sources"`
	SourceCount  int64                `json:"sourceCount"`
	EmptyReason  core.EmptyReason     `json:"emptyReason,omitempty"`
	SkippedFiles []core.SkippedFile   `json:"skippedFiles"`
}

type sourceResponseItem struct {
	Source                   core.SourceIdentity `json:"source"`
	ImportStatus             core.ImportStatus   `json:"importStatus"`
	InterpretedObservedRange *core.TimeRange     `json:"interpretedObservedRange,omitempty"`
	ImportTimeOffset         core.UtcOffset      `json:"importTimeOffset,omitempty"`
}

type runFixture struct {
	File   string                `json:"file"`
	Format core.FormatKey        `json:"format"`
	Digest string                `json:"digest"`
	Size   int64                 `json:"size"`
	State  core.PublicationState `json:"state"`
}

// 収集元を全件返す。要求は上限も続きを取る位置も持たないため、取り込んだ収集元は
// 1 回の要求ですべて応答に並ぶ。
func TestSourcesEndpointReturnsEveryIdentityAndStatusTogether(t *testing.T) {
	fixtures := runFixtures(t)
	handler := testHandler(testImportResult(t, fixtures))

	page := decodeSources(t, requestSources(t, handler, "/api/v0/sources"), http.StatusOK)
	if page.SourceCount != int64(len(page.Sources)) {
		t.Fatalf("sourceCount=%d sources=%d want the same number",
			page.SourceCount, len(page.Sources))
	}
	if len(page.Sources) != len(fixtures) {
		t.Fatalf("sources=%d want the %d read fixtures", len(page.Sources), len(fixtures))
	}
	if page.EmptyReason != "" {
		t.Fatalf("emptyReason=%q want empty", page.EmptyReason)
	}
	if page.SkippedFiles == nil || len(page.SkippedFiles) != 0 {
		t.Fatalf("skippedFiles=%v want an empty list for sources given one by one", page.SkippedFiles)
	}
	assertPageMatchesFixtures(t, page, fixtures)

	// 取り込みを保留した収集元も同じ組に載る。`/api/v0/sources` は import_withheld を返さない。
	// ImportStatus.UnmarshalJSON は Validate を呼ばないため、decode 経路では欠落を
	// 検出できない。
	withheld := sourceItemOfFixtureState(t, page, fixtures, core.PublicationStateWithheld)
	if withheld.ImportStatus.WithheldReason != core.WithheldReasonIdentifierCollision {
		t.Fatalf("withheldReason=%q want identifier_collision",
			withheld.ImportStatus.WithheldReason)
	}
}

func TestSourcesEndpointListsTheSkippedFilesOfTheCollection(t *testing.T) {
	skipped := []core.SkippedFile{
		{OriginPath: "triage/notes.txt", Reason: core.SkippedFileReasonUnsupportedFormat, DetectedKind: "text"},
		{OriginPath: "triage/empty.dat", Reason: core.SkippedFileReasonEmptyFile},
	}
	result := testImportResult(t, runFixtures(t)).WithSkippedFiles(skipped)
	page := decodeSources(t, requestSources(t, testHandler(result), "/api/v0/sources"), http.StatusOK)
	if !reflect.DeepEqual(page.SkippedFiles, skipped) {
		t.Errorf("skippedFiles=%+v want %+v", page.SkippedFiles, skipped)
	}
}

// sourceItemOfFixtureState は、指定した publicationState の fixture に対応する応答の
// 要素を、収集元の file 名で指定して返す。
func sourceItemOfFixtureState(
	t *testing.T, page sourcesResponse, fixtures []runFixture, state core.PublicationState,
) sourceResponseItem {
	t.Helper()
	for _, fixture := range fixtures {
		if fixture.State != state {
			continue
		}
		for _, item := range page.Sources {
			if item.Source.FileName == fixture.File {
				if item.ImportStatus.PublicationState != state {
					t.Fatalf("%s publicationState=%q want %q",
						fixture.File, item.ImportStatus.PublicationState, state)
				}
				return item
			}
		}
		t.Fatalf("the response carries no source named %q", fixture.File)
	}
	t.Fatalf("no fixture carries the publication state %q", state)
	return sourceResponseItem{}
}

// **上限と続きを取る位置を受けない。** 綴りを通知せずに既定値で処理しないよう、未知の
// 項目として退ける。
func TestSourcesEndpointRejectsALimitAndACursor(t *testing.T) {
	handler := testHandler(testImportResult(t, runFixtures(t)))
	for _, testCase := range []struct {
		name string
		path string
	}{
		{"limit", "/api/v0/sources?limit=10"},
		{"cursor", "/api/v0/sources?cursor=any-cursor"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			response := requestSources(t, handler, testCase.path)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status=%d want 400 body=%s", response.Code, response.Body.String())
			}
			var problem core.ApiError
			decodeJSON(t, response.Body, &problem)
			if problem.Code != core.ApiErrorCodeInvalidRequest {
				t.Fatalf("code=%q want invalid_request", problem.Code)
			}
		})
	}
}

// 収集元の一覧は要求の項目を 1 つも読まない。値を持つ要求をすべて退ける。
func TestSourcesEndpointRejectsInvalidRequests(t *testing.T) {
	fixtures := runFixtures(t)
	for _, testCase := range []struct {
		name string
		path string
	}{
		{"unknown parameter", "/api/v0/sources?unknown=x"},
		{"empty value", "/api/v0/sources?unknown="},
		{"a known parameter of another operation", "/api/v0/sources?depth=1"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			response := requestSources(t, testHandler(testImportResult(t, fixtures)), testCase.path)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status=%d want 400 body=%s", response.Code, response.Body.String())
			}
			var problem core.ApiError
			decodeJSON(t, response.Body, &problem)
			if problem.Code != core.ApiErrorCodeInvalidRequest || problem.Message == "" {
				t.Fatalf("error=%+v", problem)
			}
			if len(problem.MissingParameters) != 0 {
				t.Fatalf("missingParameters=%v want none", problem.MissingParameters)
			}
			if err := problem.Validate(); err != nil {
				t.Fatalf("invalid API error: %v", err)
			}
		})
	}
}

func TestSourcesEndpointRejectsOtherMethods(t *testing.T) {
	handler := testHandler(testImportResult(t, runFixtures(t)))
	request := httptest.NewRequest(http.MethodPost, "/api/v0/sources", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d want 405", response.Code)
	}
	// ServeMux は GET の pattern に HEAD も対応付けるため、Allow は 2 method を並べる。
	if response.Header().Get("Allow") != "GET, HEAD" {
		t.Fatalf("allow=%q want \"GET, HEAD\"", response.Header().Get("Allow"))
	}
}

func TestSourcesEndpointShowsEmptyReasonWhenNoSourcesExist(t *testing.T) {
	decoded := decodeSources(t, requestSources(t,
		testHandler(emptyImportResult(t)), "/api/v0/sources"), http.StatusOK)
	if decoded.SourceCount != 0 || len(decoded.Sources) != 0 {
		t.Fatalf("response=%+v", decoded)
	}
	if decoded.EmptyReason != core.EmptyReasonNoSourceIngested {
		t.Fatalf("emptyReason=%q want no_source_ingested", decoded.EmptyReason)
	}
}

func assertPageMatchesFixtures(t *testing.T, page sourcesResponse, fixtures []runFixture) {
	t.Helper()
	for i, fixture := range fixtures {
		item := page.Sources[i]
		identity := item.Source
		if identity.FileName != fixture.File {
			t.Fatalf("sources[%d].fileName=%q want %q", i, identity.FileName, fixture.File)
		}
		if identity.ContentSha256 != fixture.Digest {
			t.Fatalf("sources[%d].contentSha256=%q want %q", i, identity.ContentSha256, fixture.Digest)
		}
		if identity.SizeBytes != fixture.Size {
			t.Fatalf("sources[%d].sizeBytes=%d want %d", i, identity.SizeBytes, fixture.Size)
		}
		status := item.ImportStatus
		if status.SourceId != identity.SourceId {
			t.Fatalf("sources[%d].importStatus.sourceId=%q sources[%d].source.sourceId=%q", i, status.SourceId, i, identity.SourceId)
		}
		if status.PublicationState != fixture.State {
			t.Fatalf("sources[%d].importStatus.publicationState=%q want %q", i, status.PublicationState, fixture.State)
		}
	}
}

func requestSources(t *testing.T, handler http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func decodeSources(t *testing.T, response *httptest.ResponseRecorder, wantStatus int) sourcesResponse {
	t.Helper()
	if response.Code != wantStatus {
		t.Fatalf("status=%d want %d body=%s", response.Code, wantStatus, response.Body.String())
	}
	var decoded sourcesResponse
	decodeJSON(t, response.Body, &decoded)
	return decoded
}

func decodeJSON(t *testing.T, reader io.Reader, value any) {
	t.Helper()
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		t.Fatalf("trailing JSON: %v", err)
	}
}

func runFixtures(t *testing.T) []runFixture {
	t.Helper()
	data, err := os.ReadFile(runFixtureDir + "manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []runFixture
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	if len(fixtures) != 4 {
		t.Fatalf("run manifest cases=%d want 4", len(fixtures))
	}
	return fixtures
}

func testImportResult(t *testing.T, fixtures []runFixture) pipeline.ImportResult {
	t.Helper()
	plans := make([]pipeline.SourcePlan, len(fixtures))
	for i, fixture := range fixtures {
		plans[i] = pipeline.SourcePlan{
			FormatKey:  fixture.Format,
			OriginPath: "testdata/" + fixture.File,
			FileName:   fixture.File,
		}
	}
	result, err := newTestRunner(t).Run(plans)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func emptyImportResult(t *testing.T) pipeline.ImportResult {
	t.Helper()
	result, err := newTestRunner(t).Run(nil)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func newTestRunner(t *testing.T) *pipeline.Runner {
	t.Helper()
	runner, err := pipeline.NewRunner(pipeline.Config{
		Open: func(path string) (io.ReadCloser, error) {
			return os.Open(filepath.Join(runFixtureDir, filepath.Base(path)))
		},
		Parsers:  testFormatRegistry(t),
		Minter:   pipeline.DigestMinter{},
		Ordinals: pipeline.NewInMemoryOrdinals(),
		Sanitize: output.Sanitize,
		Revision: "api-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	return runner
}
