package core_test

import (
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// responseTypes は API の応答が持つ組と、端末割当の判定が使う型である。
//
// 下の test はこの並びから到達できる項目だけを見る。**型を足したときに並びへ足し忘れると、
// 指摘が 0 件のまま検査の対象から外れる。** core に公開の struct を足すときは同じ変更で
// ここへ足す。
func responseTypes() []reflect.Type {
	return []reflect.Type{
		reflect.TypeOf(core.RawAndNormalized{}),
		reflect.TypeOf(core.RecordField{}),
		reflect.TypeOf(core.Timestamp{}),
		reflect.TypeOf(core.RequestedTime{}),
		reflect.TypeOf(core.TimeRange{}),
		reflect.TypeOf(core.TimeComparison{}),
		reflect.TypeOf(core.SourceIdentity{}),
		reflect.TypeOf(core.RecordLocator{}),
		reflect.TypeOf(core.UrlFragmentJoin{}),
		reflect.TypeOf(core.UrlFragmentSegment{}),
		reflect.TypeOf(core.UrlFragment{}),
		reflect.TypeOf(core.DecodedBytes{}),
		reflect.TypeOf(core.ZipListing{}),
		reflect.TypeOf(core.ZipEntryName{}),
		reflect.TypeOf(core.RecordRange{}),
		reflect.TypeOf(core.ProcessRef{}),
		reflect.TypeOf(core.TrailInputRef{}),
		reflect.TypeOf(core.TrailStep{}),
		reflect.TypeOf(core.DerivationTrail{}),
		reflect.TypeOf(core.BothSideCounts{}),
		reflect.TypeOf(core.ObservationKind{}),
		reflect.TypeOf(core.ObservationKindSelector{}),
		reflect.TypeOf(core.ObservationKindSelectorItem{}),
		reflect.TypeOf(core.MatchAssumption{}),
		reflect.TypeOf(core.MatchCondition{}),
		reflect.TypeOf(core.TimeWindow{}),
		reflect.TypeOf(core.Candidate{}),
		reflect.TypeOf(core.CandidateStage{}),
		reflect.TypeOf(core.CandidateSet{}),
		reflect.TypeOf(core.ImportCount{}),
		reflect.TypeOf(core.DiagnosisCount{}),
		reflect.TypeOf(core.ImportFailure{}),
		reflect.TypeOf(core.ImportStatus{}),
		reflect.TypeOf(core.ApiError{}),
		reflect.TypeOf(core.GraphNode{}),
		reflect.TypeOf(core.SubgraphNode{}),
		reflect.TypeOf(core.GraphEdge{}),
		reflect.TypeOf(core.GraphEvidence{}),
		reflect.TypeOf(core.EdgeMatch{}),
		reflect.TypeOf(core.EdgeMatchTable{}),
		reflect.TypeOf(core.EdgeMatchRecord{}),
		reflect.TypeOf(core.EdgeMatchStage{}),
		reflect.TypeOf(core.EdgeMatchRef{}),
		reflect.TypeOf(core.EdgeRecordPair{}),
		reflect.TypeOf(core.EdgePairCondition{}),
		reflect.TypeOf(core.EdgeEvidenceAuthentication{}),
		reflect.TypeOf(core.NodeEdgeCount{}),
		reflect.TypeOf(core.NodeAttribute{}),
		reflect.TypeOf(core.NodeAttributeValue{}),
		reflect.TypeOf(core.NodeIdentityValue{}),
		reflect.TypeOf(core.TerminalAssignment{}),
		reflect.TypeOf(core.TerminalResolutionInput{}),
		reflect.TypeOf(core.TerminalResolution{}),
		reflect.TypeOf(core.ClockDependency{}),
		reflect.TypeOf(core.Assertion{}),
		reflect.TypeOf(core.AssertionRevision{}),
		reflect.TypeOf(core.AssertionTarget{}),
		reflect.TypeOf(core.AssertionEdgeRef{}),
		reflect.TypeOf(core.AssertionRecordRef{}),
		reflect.TypeOf(core.AssertionBasis{}),
		reflect.TypeOf(core.RecordNumbers{}),
		reflect.TypeOf(core.RecordComparison{}),
		reflect.TypeOf(core.SearchQuery{}),
		reflect.TypeOf(core.RecordConditions{}),
		reflect.TypeOf(core.AssistPermissionRevision{}),
		reflect.TypeOf(core.AssistConversation{}),
		reflect.TypeOf(core.AssistTurn{}),
		reflect.TypeOf(core.AssistShortRef{}),
		reflect.TypeOf(core.AssistRecordTarget{}),
		reflect.TypeOf(core.AssistMatchCondition{}),
		reflect.TypeOf(core.AssistVersions{}),
		reflect.TypeOf(core.AssistDisclosure{}),
		reflect.TypeOf(core.AssistEvent{}),
		reflect.TypeOf(core.TerminalSummary{}),
		reflect.TypeOf(core.TerminalDetail{}),
		reflect.TypeOf(core.TerminalEvent{}),
		reflect.TypeOf(core.AssistProposal{}),
		reflect.TypeOf(core.AssistProposalDecision{}),
		reflect.TypeOf(core.AssistMatchCondition{}),
	}
}

// responseField は型の 1 項目を持つ。
type responseField struct {
	typeName  string
	fieldName string
	jsonName  string
	fieldType reflect.Type
}

// walkResponseFields は responseTypes から到達できる公開の項目をすべて集める。
func walkResponseFields(t *testing.T) []responseField {
	t.Helper()
	visited := make(map[reflect.Type]bool)
	collected := make([]responseField, 0)
	var walk func(structType reflect.Type)
	walk = func(structType reflect.Type) {
		if visited[structType] {
			return
		}
		visited[structType] = true
		for index := range structType.NumField() {
			field := structType.Field(index)
			if field.PkgPath != "" {
				continue
			}
			fieldType := unwrapType(field.Type)
			// 埋め込みの field は json の項目を作らず、埋め込んだ型の項目がそのまま出る。
			if field.Anonymous {
				walk(fieldType)
				continue
			}
			collected = append(collected, responseField{
				typeName:  structType.Name(),
				fieldName: field.Name,
				jsonName:  jsonName(field),
				fieldType: fieldType,
			})
			if fieldType.Kind() == reflect.Struct && fieldType != reflect.TypeOf(time.Time{}) {
				walk(fieldType)
			}
		}
	}
	for _, responseType := range responseTypes() {
		walk(responseType)
	}
	if len(collected) == 0 {
		t.Fatal("no field was collected; the walk is not looking at the response types")
	}
	return collected
}

// unwrapType は pointer と集合の殻を外した型を返す。
func unwrapType(fieldType reflect.Type) reflect.Type {
	for {
		switch fieldType.Kind() {
		case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map:
			fieldType = fieldType.Elem()
		default:
			return fieldType
		}
	}
}

// jsonName は json tag の項目名を返す。
func jsonName(field reflect.StructField) string {
	tag := field.Tag.Get("json")
	name, _, _ := strings.Cut(tag, ",")
	return name
}

// lowerFirst は識別子の 1 文字目を小文字にする。
func lowerFirst(name string) string {
	runes := []rune(name)
	if len(runes) == 0 {
		return name
	}
	runes[0] = unicode.ToLower(runes[0])
	return string(runes)
}

// 応答に置かないと決めた項目名が 1 つも無いことを確かめる。
//
// 順位・確信度・点数・しきい値・危険度は、評価の根拠を持たないまま判断を数値で名乗る形で
// ある。1 件を答として名乗る項目と、時計ずれを補正した
// 時刻を持つ項目も置かない。成功と失敗を 2 値で持つ項目は、区分ごとの件数に替える。
func TestForbiddenItemNamesAreAbsent(t *testing.T) {
	forbidden := []string{
		"rank", "order", "position",
		"confidence", "probability", "likelihood",
		"score", "weight", "points",
		"threshold", "cutoff", "minScore",
		"suspicionLevel", "severity", "riskLevel",
		"matchedRecord", "correspondingRecord", "resolvedCounterpart", "bestMatch",
		"clockSkewSeconds", "adjustedTime", "correctedTime",
		"removedMembers", "narrowedFrom",
		"succeeded", "failed", "ok", "isValid", "errorRate", "failureRatio", "isComplete",
	}

	for _, field := range walkResponseFields(t) {
		for _, name := range forbidden {
			if strings.EqualFold(field.jsonName, name) || strings.EqualFold(field.fieldName, name) {
				t.Errorf("%s carries the item %q, which the response must not place",
					field.typeName, name)
			}
		}
	}
}

// json tag の項目名が field の名前と 1 字も違わないことを確かめる。
func TestJsonNamesMatchTheFieldNames(t *testing.T) {
	for _, field := range walkResponseFields(t) {
		if field.jsonName == "" {
			t.Errorf("%s.%s has no json tag", field.typeName, field.fieldName)
			continue
		}
		if want := lowerFirst(field.fieldName); field.jsonName != want {
			t.Errorf("%s.%s has the json name %q, want %q",
				field.typeName, field.fieldName, field.jsonName, want)
		}
	}
}

// 時刻を持つ項目が Timestamp か RequestedTime であり、どちらも precision を持つ。
//
// 裸の time.Time を持つ項目は精度を失う。観測された時刻は Timestamp、要求が与えた時刻は
// RequestedTime が持つ。
func TestEveryTimeItemCarriesAPrecision(t *testing.T) {
	timeType := reflect.TypeOf(time.Time{})
	for _, field := range walkResponseFields(t) {
		if field.fieldType == timeType {
			t.Errorf("%s.%s carries a time without a precision",
				field.typeName, field.fieldName)
		}
	}

	for name, carrier := range map[string]reflect.Type{
		"Timestamp":     reflect.TypeOf(core.Timestamp{}),
		"RequestedTime": reflect.TypeOf(core.RequestedTime{}),
	} {
		precisionField, found := carrier.FieldByName("Precision")
		if !found {
			t.Fatalf("%s must carry a precision", name)
		}
		if got := jsonName(precisionField); got != "precision" {
			t.Errorf("the precision item of %s is named %q, want %q", name, got, "precision")
		}
	}
}
