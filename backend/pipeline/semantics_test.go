package pipeline_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// markiiCommunicationLine は接続の 4 項目を持つ通信のレコードである。
// 期待値は本 file の各 test が同じ文字列から読んで決める。
const markiiCommunicationLine = `01/02/2024 03:04:05.006 +0000 sn=7 evt=net subEvt=con ` +
	`psGUID=p tmid=t com=c csid=s psPath=app srcIP=192.0.2.10 srcPort=50002 ` +
	`dstIP=198.51.100.42 dstPort=80`

func parseOne(t *testing.T, parser pipeline.SourceParser, input string) pipeline.ParsedRecord {
	t.Helper()
	parser.Reset(strings.NewReader(input))
	record, failure, err := parser.Next()
	if err != nil || failure != nil {
		t.Fatalf("Next = %+v, %v", failure, err)
	}
	return record
}

func textOf(t *testing.T, fields []core.RecordField, name string) core.RawAndNormalized {
	t.Helper()
	for _, field := range fields {
		if field.Name == name {
			if field.Text == nil {
				t.Fatalf("field %q carries no text value", name)
			}
			return *field.Text
		}
	}
	t.Fatalf("field %q is absent from the set", name)
	return core.RawAndNormalized{}
}

func semanticOf(t *testing.T, fields []core.RecordField, name string) core.SemanticKey {
	t.Helper()
	for _, field := range fields {
		if field.Name == name {
			return field.Semantic
		}
	}
	t.Fatalf("field %q is absent from the set", name)
	return ""
}

func comparableOf(t *testing.T, value core.RawAndNormalized) string {
	t.Helper()
	got, ok := value.ComparableValue()
	if !ok {
		t.Fatalf("value %+v carries nothing to compare", value)
	}
	return got
}

func TestMarkIICommunicationCarriesSemantics(t *testing.T) {
	record := parseOne(t, pipeline.NewTestMarkIIParser(), markiiCommunicationLine)
	semantics := record.Semantics
	if semantics == nil {
		t.Fatal("a communication record carried no semantics")
	}
	if err := semantics.ObservationKind.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := comparableOf(t, textOf(t, semantics.ObservationKind.Raw, "evt")); got != "net" {
		t.Errorf("evt = %q, want %q", got, "net")
	}
	if got := comparableOf(t, textOf(t, semantics.ObservationKind.Raw, "subEvt")); got != "con" {
		t.Errorf("subEvt = %q, want %q", got, "con")
	}
	if semantics.ProcessRef == nil ||
		semantics.ProcessRef.ProcessId != "p" || semantics.ProcessRef.TerminalId != "t" {
		t.Errorf("process reference = %+v", semantics.ProcessRef)
	}
	if semantics.ProcessRef.SourceId != "" || semantics.ProcessRef.SourceContentSha256 != "" {
		t.Errorf("the parser filled the source identity: %+v", semantics.ProcessRef)
	}
	if semantics.ParentProcessId != nil {
		t.Errorf("a communication record carried a parent process id: %+v", semantics.ParentProcessId)
	}
	if got := comparableOf(t, textOf(t, semantics.Fields, "com")); got != "c" {
		t.Errorf("com = %q, want %q", got, "c")
	}
	checkMarkIIEndpoint(t, semantics.Endpoint)
}

// endpointItem は一覧に出す接続の 1 項目の期待値である。
type endpointItem struct {
	name     string
	semantic core.SemanticKey
	value    string
}

func checkEndpointItems(
	t *testing.T, side string, got []core.RecordField, want []endpointItem,
) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s carries %d items, want %d: %+v", side, len(got), len(want), got)
	}
	for index, wanted := range want {
		field := got[index]
		if field.Name != wanted.name {
			t.Errorf("%s at %d name = %q, want %q", side, index, field.Name, wanted.name)
		}
		if field.Semantic != wanted.semantic {
			t.Errorf("%s at %d semantic = %q, want %q",
				side, index, field.Semantic, wanted.semantic)
		}
		if field.Text == nil {
			t.Fatalf("%s at %d carries no text value", side, index)
		}
		if value := comparableOf(t, *field.Text); value != wanted.value {
			t.Errorf("%s at %d value = %q, want %q", side, index, value, wanted.value)
		}
	}
}

func checkMarkIIEndpoint(t *testing.T, endpoint *pipeline.RecordEndpoint) {
	t.Helper()
	if endpoint == nil {
		t.Fatal("a communication record carried no endpoint")
	}
	// 名前は原資料の key の文字列である。markii 形式は接続の 4 項目とも欄を持つ。
	checkEndpointItems(t, "clientEndpoint", endpoint.ClientEndpoint, []endpointItem{
		{"srcIP", core.SemanticKeyConnectionSourceAddress, "192.0.2.10"},
		{"srcPort", core.SemanticKeyConnectionSourcePort, "50002"},
	})
	checkEndpointItems(t, "destination", endpoint.Destination, []endpointItem{
		{"dstIP", core.SemanticKeyConnectionDestinationAddress, "198.51.100.42"},
		{"dstPort", core.SemanticKeyConnectionDestinationPort, "80"},
	})
}

func TestMarkIIProcessStartCarriesParentProcessId(t *testing.T) {
	record := parseOne(t, pipeline.NewTestMarkIIParser(), markiiScanLine+" parentGUID=q")
	semantics := record.Semantics
	if semantics == nil {
		t.Fatal("a process start record carried no semantics")
	}
	if semantics.Endpoint != nil {
		t.Errorf("a process start record carried an endpoint: %+v", semantics.Endpoint)
	}
	if semantics.ParentProcessId == nil {
		t.Fatal("a process start record carried no parent process id")
	}
	if got := comparableOf(t, *semantics.ParentProcessId); got != "q" {
		t.Errorf("parentGUID = %q, want %q", got, "q")
	}
	if semantics.ProcessRef == nil || semantics.ProcessRef.ProcessId != "p" {
		t.Errorf("process reference = %+v", semantics.ProcessRef)
	}
}

func TestMarkIIProcessStartWithoutParentProcessIdKeepsItemAbsent(t *testing.T) {
	record := parseOne(t, pipeline.NewTestMarkIIParser(), markiiScanLine)
	if record.Semantics == nil || record.Semantics.ParentProcessId == nil {
		t.Fatal("a process start record carried no parent process id item")
	}
	if record.Semantics.ParentProcessId.ValueState != core.ValueStateItemAbsent {
		t.Errorf("parentGUID valueState = %q, want %q",
			record.Semantics.ParentProcessId.ValueState, core.ValueStateItemAbsent)
	}
}

// 通信でもプロセス開始でもないレコードは、観測の種別と項目の集合だけを持つ。
// evt が other の組は V3.0 の Recorder が出力する組に無いため、意味の状態は undetermined である。
func TestMarkIIOtherEventCarriesKindAndFieldsOnly(t *testing.T) {
	record := parseOne(t, pipeline.NewTestMarkIIParser(),
		strings.Replace(markiiScanLine, "evt=ps", "evt=other", 1))
	semantics := record.Semantics
	if semantics == nil {
		t.Fatal("a record whose evt is other carried no semantics")
	}
	if semantics.ProcessRef != nil || semantics.Endpoint != nil || semantics.ParentProcessId != nil {
		t.Errorf("semantics carried a reference: %+v", semantics)
	}
	if err := semantics.ObservationKind.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := semantics.ObservationKind.Status; got != core.ObservationKindStatusUndetermined {
		t.Errorf("status = %q, want %q", got, core.ObservationKindStatusUndetermined)
	}
	if got := comparableOf(t, textOf(t, semantics.ObservationKind.Raw, "evt")); got != "other" {
		t.Errorf("evt = %q, want %q", got, "other")
	}
	if got := comparableOf(t, textOf(t, semantics.Fields, "psPath")); got != "app" {
		t.Errorf("psPath = %q, want %q", got, "app")
	}
	header := semantics.Fields[0]
	if header.Name != "headerTime" || header.Timestamp == nil ||
		header.Timestamp.Normalized == nil || *header.Timestamp.Normalized != "2024-01-02T03:04:05.006Z" {
		t.Errorf("the first field = %+v", header)
	}
}

func TestSquidRecordCarriesSemantics(t *testing.T) {
	record := parseOne(t, pipeline.NewTestSquidParser(), squidScanLine)
	semantics := record.Semantics
	if semantics == nil {
		t.Fatal("a Squid record carried no semantics")
	}
	if err := semantics.ObservationKind.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(semantics.ObservationKind.Raw) != 0 || semantics.ObservationKind.Status != "" {
		t.Errorf("observation kind = %+v", semantics.ObservationKind)
	}
	if semantics.ProcessRef != nil || semantics.ParentProcessId != nil {
		t.Errorf("a Squid record carried a process: %+v %+v",
			semantics.ProcessRef, semantics.ParentProcessId)
	}
	for _, field := range semantics.Fields {
		if err := field.Validate(); err != nil {
			t.Fatalf("field %q did not validate: %v", field.Name, err)
		}
	}
	// combined の欄と、%ru から導く requestMethod と requestTargetHost と
	// requestTargetPort を合わせた集合である。応答の fields はこの集合から名前を選び、
	// clientTerminal と clientPort と process を足した別の集合になる。
	//
	// **名前の集合を期待値にする。** 欄を足した変更では、足した名前が差分に出る。
	names := make([]string, 0, len(semantics.Fields))
	for _, field := range semantics.Fields {
		names = append(names, field.Name)
	}
	slices.Sort(names)
	wantNames := []string{
		"clientIp", "ident", "user", "requestTime", "requestLine", "statusCode",
		"replyBytes", "referer", "userAgent", "squidStatus",
		"requestMethod", "requestTarget", "requestTargetHost", "requestTargetPort", "requestVersion",
	}
	slices.Sort(wantNames)
	if !slices.Equal(names, wantNames) {
		t.Errorf("fields = %v, want %v", names, wantNames)
	}
	if got := comparableOf(t, textOf(t, semantics.Fields, "requestMethod")); got != "GET" {
		t.Errorf("requestMethod = %q, want %q", got, "GET")
	}
	if got := comparableOf(t, textOf(t, semantics.Fields, "requestTargetHost")); got != "example.test" {
		t.Errorf("requestTargetHost = %q, want %q", got, "example.test")
	}
	if got := comparableOf(t, textOf(t, semantics.Fields, "requestTargetPort")); got != "80" {
		t.Errorf("requestTargetPort = %q, want %q", got, "80")
	}
	checkSquidRequestVersion(t, semantics)
	checkSquidEndpoint(t, semantics)
}

// checkSquidRequestVersion は、要求行の 3 番目の token が原資料の文字列のまま残り、比べられる値が
// 小文字になることを確かめる。
func checkSquidRequestVersion(t *testing.T, semantics *pipeline.RecordSemantics) {
	t.Helper()
	version := textOf(t, semantics.Fields, "requestVersion")
	if got := version.RawText; got == nil || *got != "HTTP/1.1" {
		t.Errorf("requestVersion rawText = %v, want %q", got, "HTTP/1.1")
	}
	if got := comparableOf(t, version); got != "http/1.1" {
		t.Errorf("requestVersion comparable = %q, want %q", got, "http/1.1")
	}
	if got := semanticOf(t, semantics.Fields, "requestVersion"); got != core.SemanticKeyHttpRequestVersion {
		t.Errorf("requestVersion semantic = %q, want %q", got,
			core.SemanticKeyHttpRequestVersion)
	}
}

func checkSquidEndpoint(t *testing.T, semantics *pipeline.RecordSemantics) {
	t.Helper()
	endpoint := semantics.Endpoint
	if endpoint == nil {
		t.Fatal("a Squid record carried no endpoint")
	}
	// combined は接続元 port の欄を持たず、接続先 port は同じレコードの %ru から導いた
	// 関連付けの鍵である。2 項目は一覧の要素にならない。
	checkEndpointItems(t, "clientEndpoint", endpoint.ClientEndpoint, []endpointItem{
		{"clientIp", core.SemanticKeyConnectionSourceAddress, "192.0.2.1"},
	})
	checkEndpointItems(t, "destination", endpoint.Destination, []endpointItem{
		{"requestTargetHost", core.SemanticKeyConnectionDestinationHostname, "example.test"},
	})
	// 反対側。一覧に出ない接続先 port の値は項目の集合に残る。
	if got := comparableOf(t, textOf(t, semantics.Fields, "requestTargetPort")); got != "80" {
		t.Errorf("requestTargetPort = %q, want %q", got, "80")
	}
}

func TestSquidRequestTargetPortSources(t *testing.T) {
	for _, tt := range []struct {
		target     string
		port       string
		valueState core.ValueState
	}{
		{target: "http://example.test/", port: "80", valueState: core.ValueStatePresent},
		{target: "https://example.test/", port: "443", valueState: core.ValueStatePresent},
		{target: "http://example.test:8080/", port: "8080", valueState: core.ValueStatePresent},
		{target: "HTTPS://example.test/", port: "443", valueState: core.ValueStatePresent},
		{target: "urn:example:record", valueState: core.ValueStateDerivationUndetermined},
		{target: "/path?x=1", valueState: core.ValueStateDerivationUndetermined},
	} {
		line := strings.Replace(squidScanLine, "http://example.test/", tt.target, 1)
		record := parseOne(t, pipeline.NewTestSquidParser(), line)
		if record.Semantics == nil {
			t.Fatalf("target %q carried no semantics", tt.target)
		}
		port := textOf(t, record.Semantics.Fields, "requestTargetPort")
		if port.ValueState != tt.valueState {
			t.Errorf("target %q port valueState = %q, want %q",
				tt.target, port.ValueState, tt.valueState)
		}
		got, ok := port.ComparableValue()
		if ok != (tt.valueState == core.ValueStatePresent) || got != tt.port {
			t.Errorf("target %q port = %q %v, want %q", tt.target, got, ok, tt.port)
		}
	}
}

// TestSquidRequestTargetHostDropsUserinfoPortAndBrackets は接続先 IP が持つ文字列から
// userinfo と port と IPv6 の角括弧が外れ、rawText に %ru 全体が残ることを確かめる。
func TestSquidRequestTargetHostDropsUserinfoPortAndBrackets(t *testing.T) {
	for _, tt := range []struct {
		target, host, port string
	}{
		{target: "http://reader@198.51.100.42:8080/a", host: "198.51.100.42", port: "8080"},
		{target: "http://reader@[2001:db8::9]:8443/a", host: "2001:db8::9", port: "8443"},
		{target: "https://[2001:db8::9]/a", host: "2001:db8::9", port: "443"},
	} {
		line := strings.Replace(squidScanLine, "http://example.test/", tt.target, 1)
		record := parseOne(t, pipeline.NewTestSquidParser(), line)
		host := textOf(t, record.Semantics.Fields, "requestTargetHost")
		if got := comparableOf(t, host); got != tt.host {
			t.Errorf("target %q requestTargetHost = %q, want %q", tt.target, got, tt.host)
		}
		if value, ok := host.RawTextValue(); !ok || value != tt.target {
			t.Errorf("target %q requestTargetHost rawText = %q %v", tt.target, value, ok)
		}
		derivation, hasDerivation := host.DerivationValue()
		want := "要求先の URI の authority から userinfo と port と IPv6 の角括弧を外した host"
		if !hasDerivation || derivation != want {
			t.Errorf("target %q requestTargetHost derivation = %q %v, want %q",
				tt.target, derivation, hasDerivation, want)
		}
		port := textOf(t, record.Semantics.Fields, "requestTargetPort")
		if got := comparableOf(t, port); got != tt.port {
			t.Errorf("target %q requestTargetPort = %q, want %q", tt.target, got, tt.port)
		}
	}
}

// TestSquidTargetWithoutAuthorityKeepsDerivationUndetermined は、同じレコードの中の導出が
// 失敗した項目が derivation_undetermined になり、導出の元にした %ru の文字列を derivation に
// 持つことを確かめる。NewDerivationUndeterminedValue が rawText を拒む。
func TestSquidTargetWithoutAuthorityKeepsDerivationUndetermined(t *testing.T) {
	line := strings.Replace(squidScanLine, "http://example.test/", "/path?x=1", 1)
	record := parseOne(t, pipeline.NewTestSquidParser(), line)
	for _, tt := range []struct {
		item       string
		value      core.RawAndNormalized
		derivation string
	}{
		{
			item:       "requestTargetHost",
			value:      textOf(t, record.Semantics.Fields, "requestTargetHost"),
			derivation: "要求先 %ru の文字列 /path?x=1 が authority を持たない",
		},
		{
			item:       "requestTargetPort",
			value:      textOf(t, record.Semantics.Fields, "requestTargetPort"),
			derivation: "要求先 %ru の文字列 /path?x=1 が port と scheme のどちらも書いていない",
		},
	} {
		if tt.value.ValueState != core.ValueStateDerivationUndetermined {
			t.Fatalf("%s valueState = %q, want %q",
				tt.item, tt.value.ValueState, core.ValueStateDerivationUndetermined)
		}
		if _, hasRaw := tt.value.RawTextValue(); hasRaw {
			t.Errorf("an undetermined %s carried an original spelling", tt.item)
		}
		derivation, hasDerivation := tt.value.DerivationValue()
		if !hasDerivation || derivation != tt.derivation {
			t.Errorf("%s derivation = %q %v, want %q",
				tt.item, derivation, hasDerivation, tt.derivation)
		}
	}
}

// TestSquidTargetPortDerivationCarriesControlByteFromPath は、authority の外に制御 byte を
// 持つ要求先が、同じ制御 byte を derivation へそのまま運ぶことを確かめる。字句解析が拒む
// 制御 byte は CR / LF / TAB / NUL の 4 つで、authority の検査が読む範囲は authority 部分
// だけである。無害化は出力境界が担う。
func TestSquidTargetPortDerivationCarriesControlByteFromPath(t *testing.T) {
	const target = "custom://example.test/a\x1bb"
	line := strings.Replace(squidScanLine, "http://example.test/", target, 1)
	record := parseOne(t, pipeline.NewTestSquidParser(), line)
	if record.Semantics == nil {
		t.Fatal("a record with a control byte in the request target carried no semantics")
	}
	port := textOf(t, record.Semantics.Fields, "requestTargetPort")
	if port.ValueState != core.ValueStateDerivationUndetermined {
		t.Fatalf("requestTargetPort valueState = %q, want %q",
			port.ValueState, core.ValueStateDerivationUndetermined)
	}
	derivation, hasDerivation := port.DerivationValue()
	want := "要求先 %ru の文字列 custom://example.test/a\x1bb が port と scheme のどちらも書いていない"
	if !hasDerivation || derivation != want {
		t.Errorf("requestTargetPort derivation = %q %v, want %q", derivation, hasDerivation, want)
	}
}
