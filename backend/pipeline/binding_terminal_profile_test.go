// in-package test: 収集の registry の値から収集の端末の情報を組むことを確かめる。
package pipeline

import (
	"io"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// registryKeyParser は、1 行を registry の key 1 つのレコードとして返す。行は
// `<時刻>|<key の path>|<値の名前>=<値>;...` である。値 ComputerName は端末自身の名前にする。
type registryKeyParser struct {
	t      *testing.T
	lines  []string
	next   int
	offset int64
}

func (p *registryKeyParser) Identity() ParserIdentity {
	return ParserIdentity{
		ParserID: "registry-key-test", FormatKey: WindowsRegistryHiveFormatKey, PositionKind: core.PositionKindByteRange,
		TimePrecision: core.PrecisionSecond, RecordedByOneTerminal: true, ItemSemantics: []core.SemanticKey{},
		ConnectionRequestKinds: []core.ObservationKindSelector{}, ConnectionMatchConditions: []ConnectionMatchCondition{},
		TranscriptIdentityItems: []string{},
	}
}

func (p *registryKeyParser) Reset(input io.Reader) {
	content, _ := io.ReadAll(input)
	p.lines, p.next, p.offset = strings.SplitAfter(strings.TrimSuffix(string(content), "\n"), "\n"), 0, 0
}

func (p *registryKeyParser) Next() (ParsedRecord, *core.ImportFailure, error) {
	if p.next == len(p.lines) {
		return ParsedRecord{}, nil, io.EOF
	}
	line := p.lines[p.next]
	p.next++
	parts := strings.SplitN(strings.TrimSuffix(line, "\n"), "|", 3)
	fields := []core.RecordField{syntheticField(p.t, "KeyPath", "", parts[1])}
	var namings []TerminalNaming
	for _, pair := range strings.Split(parts[2], ";") {
		name, value, _ := strings.Cut(pair, "=")
		fields = append(fields, syntheticField(p.t, "Value."+name, "", value))
		if name == "ComputerName" {
			namings = append(namings, TerminalNaming{Name: value})
		}
	}
	length := int64(len(line))
	record := ParsedRecord{
		RawText: line, LineNumber: int64(p.next), ByteOffset: p.offset, ByteLength: &length,
		ObservedAt: utcAt(p.t, parts[0]),
		Semantics: &RecordSemantics{
			ObservationKind: core.ObservationKind{Raw: []core.RecordField{}}, Fields: fields,
		},
		TerminalNamings: namings,
	}
	p.offset += length
	return record, nil, nil
}

func registryKeySource(t *testing.T, name, collection string, lines ...string) collectionSource {
	t.Helper()
	return collectionSource{
		scanned: scanIndexSource(t, &registryKeyParser{t: t}, name, WindowsRegistryHiveFormatKey,
			strings.Join(lines, "\n")+"\n"),
		collection: collection,
	}
}

// 端末の情報は、Select の Current が選ぶ ControlSet の値だけを持ち、値ごとに記録した key の
// レコードを指す。IP アドレスの期間は、リースを得た時刻から収集の最後のレコードの時刻までである。
// 別の収集の値を持たない。
func TestTerminalProfileReadsTheCurrentControlSet(t *testing.T) {
	system := registryKeySource(t, "SYSTEM", "triage-p",
		`2001-02-03T01:00:00Z|\Select|Current=4`,
		`2001-02-03T01:00:00Z|\ControlSet004\Control\ComputerName\ComputerName|ComputerName=HOST-P`,
		`2001-02-03T01:00:00Z|\ControlSet003\Services\Tcpip\Parameters\Interfaces\{if-old}|DhcpIPAddress=192.0.2.1`,
		`2001-02-03T01:00:00Z|\ControlSet004\Services\Tcpip\Parameters\Interfaces\{if-a}|DhcpIPAddress=192.0.2.20;LeaseObtainedTime=981162000 (0x3A7B6010)`,
		`2001-02-03T01:00:00Z|\ControlSet004\Services\Tcpip\Parameters\Interfaces\{if-b}|IPAddress=198.51.100.7`,
		`2001-02-03T01:00:00Z|\ControlSet003\Control\TimeZoneInformation|TimeZoneKeyName=Old Zone`,
		`2001-02-03T09:00:00Z|\ControlSet004\Control\TimeZoneInformation|TimeZoneKeyName=Synthetic Zone;Bias=4294967176 (0xFFFFFF88);ActiveTimeBias=4294967176 (0xFFFFFF88)`)
	software := registryKeySource(t, "SOFTWARE", "triage-p",
		`2001-02-03T02:00:00Z|\Microsoft\Windows NT\CurrentVersion|ProductName=Synthetic OS;CurrentBuild=12345;UBR=6 (0x00000006);SystemRoot=Y:\SynthWin`)
	other := registryKeySource(t, "SYSTEM", "triage-q",
		`2001-02-03T01:00:00Z|\Select|Current=3`,
		`2001-02-03T01:00:00Z|\ControlSet003\Control\ComputerName\ComputerName|ComputerName=HOST-Q`,
		`2001-02-03T01:00:00Z|\ControlSet003\Control\TimeZoneInformation|TimeZoneKeyName=Other Zone`)
	graph := NewGraph(collectionImportResult(t, system, software, other), AllMatchConditions())
	key, _ := core.CollectionTerminalNodeKey(collectionDigest([]string{
		system.scanned.Measurement.ContentSha256, software.scanned.Measurement.ContentSha256,
	}))
	profile := graph.TerminalProfile(nodeIdOf(key))

	valuesOf := func(values []TerminalProfileValue) string {
		var texts []string
		for _, value := range values {
			texts = append(texts, value.Name+"="+value.Value)
		}
		return strings.Join(texts, ";")
	}
	if got := valuesOf(profile.OperatingSystem); got != `ProductName=Synthetic OS;CurrentBuild=12345;UBR=6 (0x00000006);SystemRoot=Y:\SynthWin` {
		t.Errorf("the operating system is %s", got)
	}
	if got := valuesOf(profile.TimeZone); got != "TimeZoneKeyName=Synthetic Zone;Bias=4294967176 (0xFFFFFF88);ActiveTimeBias=4294967176 (0xFFFFFF88)" {
		t.Errorf("the time zone is %s", got)
	}
	if len(profile.TimeZone) > 0 {
		if ref := profile.TimeZone[0].RecordRef; ref.SourceFileName != "SYSTEM" || ref.LineNumber == nil || *ref.LineNumber != 7 {
			t.Errorf("the time zone is recorded by %+v, want the line 7 of SYSTEM", ref)
		}
	}
	if len(profile.Names) != 1 || profile.Names[0].Name != "HOST-P" {
		t.Errorf("the names are %+v", profile.Names)
	}
	if len(profile.Addresses) != 2 {
		t.Fatalf("the addresses are %+v, want the two interfaces of ControlSet004", profile.Addresses)
	}
	leased := profile.Addresses[0]
	if leased.Interface != "{if-a}" || valuesOf(leased.Values) != "DhcpIPAddress=192.0.2.20;LeaseObtainedTime=981162000 (0x3A7B6010)" {
		t.Errorf("the leased address is %+v", leased)
	}
	if leased.From == nil || *leased.From.Normalized != "2001-02-03T01:00:00Z" ||
		leased.To == nil || *leased.To.Normalized != "2001-02-03T09:00:00Z" {
		t.Errorf("the lease period is %+v to %+v", leased.From, leased.To)
	}
	if static := profile.Addresses[1]; static.From != nil || valuesOf(static.Values) != "IPAddress=198.51.100.7" {
		t.Errorf("the static address is %+v", static)
	}
	terminalAt, present := graph.nodeAt[nodeIdOf(key)]
	if !present {
		t.Fatal("the graph holds no node for the collection terminal")
	}
	attributes := map[core.SemanticKey]string{}
	for _, attribute := range graph.nodes[terminalAt].attributes {
		value, _ := attribute.field.Text.RawTextValue()
		if record := graph.records[attribute.evidence[0]]; record.locator.SourceFileName != "SOFTWARE" {
			t.Errorf("the attribute %s points at %+v, want the CurrentVersion record", attribute.field.Semantic, record.locator)
		}
		attributes[attribute.field.Semantic] = value
	}
	if attributes[core.SemanticKeyTerminalOsProductName] != "Synthetic OS" ||
		attributes[core.SemanticKeyTerminalOsBuild] != "12345" || attributes[core.SemanticKeyTerminalOsUbr] != "6 (0x00000006)" {
		t.Errorf("the terminal node attributes are %v", attributes)
	}
	addressEdges := map[string]core.GraphEdge{}
	for _, edge := range graph.edges {
		if edge.kind == core.EdgeKindTerminalAddress && edge.source == terminalAt {
			label, _ := graph.nodes[edge.target].label.RawTextValue()
			addressEdges[label] = graph.responseEdge(edge, edge.evidence)
		}
	}
	if len(addressEdges) != 2 {
		t.Fatalf("the terminal_address edges reach %v, want the two addresses of ControlSet004", addressEdges)
	}
	if leased := addressEdges["192.0.2.20"].ApplicableRange; leased == nil ||
		*leased.From.Normalized != "2001-02-03T01:00:00Z" || *leased.To.Normalized != "2001-02-03T09:00:00Z" {
		t.Errorf("the leased address edge has the range %+v", leased)
	}
	if static, found := addressEdges["198.51.100.7"]; !found || static.EvidenceCount != 1 {
		t.Errorf("the static address edge is %+v", static)
	}
	if absent := graph.TerminalProfile("absent"); len(absent.OperatingSystem)+len(absent.Addresses)+len(absent.TimeZone) != 0 {
		t.Errorf("an absent node has the profile %+v", absent)
	}
}
