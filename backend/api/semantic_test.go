package api_test

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// Squid のレコードを指した `/api/v0/records` の応答の fields が、Squid の欄と要求行から導く項目の語彙の項目を持つ。
func TestRecordsResponseCarriesTheSemanticOfTheSquidRecord(t *testing.T) {
	manifest := recordsFixtures(t)
	handler := testHandler(twoFormatImportResult(t))
	identity := recordsSourceIdentity(t, handler, manifest.SquidRecord.File)

	decoded := decodeRecord(t, requestSources(t, handler, recordsPath+"?"+
		recordsSourceQuery(identity)+"&lineNumber="+
		strconv.FormatInt(manifest.SquidRecord.LineNumber, 10)), http.StatusOK)

	assertSemanticByFieldName(t, decoded.Fields, manifest.SquidRecord.SemanticByFieldName)
}

// markii 形式のレコードを指した `/api/v0/records` の応答の fields と observationKind の raw が、
// markii 形式の key の語彙の項目を持つ。
func TestRecordsResponseCarriesTheSemanticOfTheMarkIIRecord(t *testing.T) {
	manifest := recordsFixtures(t)
	handler := testHandler(twoFormatImportResult(t))
	identity := recordsSourceIdentity(t, handler, manifest.MarkiiRecord.File)

	decoded := decodeRecord(t, requestSources(t, handler, recordsPath+"?"+
		recordsSourceQuery(identity)+"&sequenceNumber="+
		strconv.FormatInt(manifest.MarkiiRecord.SequenceNumber, 10)), http.StatusOK)

	assertSemanticByFieldName(t, decoded.Fields, manifest.MarkiiRecord.SemanticByFieldName)
	assertObservationKindSemantic(t, decoded.ObservationKind)
}

// `/api/v0/edges/{id}` の根拠の observationKind の raw が、観測の種別の語彙の項目を持つ。
//
// **`/api/v0/graph` は根拠の中身を持たない。** 根拠はエッジを選んだときに
// `/api/v0/edges/{id}` が返す。
func TestGraphEvidenceCarriesTheSemanticOfTheObservationKind(t *testing.T) {
	handler := graphHandler(t)
	page := decodeGraph(t, handler, wholeGraphQuery)

	if len(page.Edges) == 0 {
		t.Fatal("the subgraph carries no edge, so the check reads nothing")
	}
	observed := 0
	for _, edge := range page.Edges {
		for _, evidence := range decodeEdge(t, handler, edge.Id, "").Edge.Evidence {
			if len(evidence.ObservationKind.Raw) == 0 {
				continue
			}
			observed++
			assertObservationKindSemantic(t, evidence.ObservationKind)
		}
	}
	if observed == 0 {
		t.Fatal("no evidence record carries an observation kind, so the check reads nothing")
	}
}

// assertSemanticByFieldName は name ごとの語彙の項目を確かめる。
// want に載らない name の項目は semantic を持たない。
func assertSemanticByFieldName(
	t *testing.T, fields []core.RecordField, want map[string]core.SemanticKey,
) {
	t.Helper()
	if len(want) == 0 {
		t.Fatal("the manifest carries no expected semantic, so the check reads nothing")
	}
	seen := make(map[string]bool, len(want))
	for _, field := range fields {
		expected := want[field.Name]
		if field.Semantic != expected {
			t.Errorf("semantic of %q = %q, want %q", field.Name, field.Semantic, expected)
		}
		if expected != "" {
			seen[field.Name] = true
			if !field.Semantic.IsKnown() {
				t.Errorf("semantic of %q = %q, which is outside the vocabulary",
					field.Name, field.Semantic)
			}
		}
	}
	for name := range want {
		if !seen[name] {
			t.Errorf("the response carries no field named %q", name)
		}
	}
}

// assertObservationKindSemantic は observationKind の raw の 2 件を確かめる。
func assertObservationKindSemantic(t *testing.T, kind core.ObservationKind) {
	t.Helper()
	want := []core.SemanticKey{core.SemanticKeyEventCategory, core.SemanticKeyEventAction}
	if len(kind.Raw) != len(want) {
		t.Fatalf("observationKind.raw carries %d items, want %d", len(kind.Raw), len(want))
	}
	for index, semantic := range want {
		if kind.Raw[index].Semantic != semantic {
			t.Errorf("semantic of observationKind.raw[%d] = %q, want %q",
				index, kind.Raw[index].Semantic, semantic)
		}
	}
}
