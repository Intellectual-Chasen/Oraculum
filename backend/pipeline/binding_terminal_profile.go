package pipeline

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/winregistry"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 端末の情報を読む registry の key と値。
var (
	selectKey          = regexp.MustCompile(`(?i)^\\Select$`)
	osVersionKey       = regexp.MustCompile(`(?i)^\\Microsoft\\Windows NT\\CurrentVersion$`)
	tcpipInterfaceKey  = regexp.MustCompile(`(?i)^\\(ControlSet\d{3})\\Services\\Tcpip\\Parameters\\Interfaces\\([^\\]+)$`)
	timeZoneKey        = regexp.MustCompile(`(?i)^\\(ControlSet\d{3})\\Control\\TimeZoneInformation$`)
	osValueNames       = []string{"ProductName", "DisplayVersion", "CurrentBuild", "UBR", "EditionID", "SystemRoot"}
	addressValueNames  = []string{"DhcpIPAddress", "IPAddress", "LeaseObtainedTime"}
	timeZoneValueNames = []string{"TimeZoneKeyName", "Bias", "ActiveTimeBias"}
)

// registryRecord は、収集の registry のレコード 1 件と、その key の path である。
type registryRecord struct {
	path   string
	record RecordEntry
}

// addTerminalProfiles は、収集の端末ごとに、収集の registry のレコードから端末の情報を組む。
func (g *Graph) addTerminalProfiles(result ImportResult, terminals sourceTerminals) {
	registries := make(map[*collectionTerminal][]registryRecord)
	last := make(map[*collectionTerminal]*core.Timestamp)
	for _, publication := range result.publications {
		if publication.status.PublicationState == core.PublicationStateWithheld {
			continue
		}
		collection := terminals[publication.status.SourceId].collection
		if collection == nil {
			continue
		}
		for _, record := range publication.records {
			if at, readable := instantOf(record.ObservedAt); readable {
				if held, _ := instantOf(last[collection]); last[collection] == nil || at.After(held) {
					last[collection] = record.ObservedAt
				}
			}
			if publication.parser.FormatKey != winregistry.FormatKeyHive || record.Semantics == nil {
				continue
			}
			if path, found := rawValueOf(record.Semantics.Fields, "KeyPath"); found {
				registries[collection] = append(registries[collection], registryRecord{path, record})
			}
		}
	}
	for collection, records := range registries {
		profile := terminalProfileOf(records, last[collection])
		profile.Names = collection.history
		if g.terminalProfiles == nil {
			g.terminalProfiles = make(map[string]TerminalProfile)
		}
		g.terminalProfiles[nodeIdOf(collection.key)] = profile
		// 端末のノードを他のレコードが作っていない収集でも、registry の値を端末のノードへ置く。
		terminalAt := g.ensureNode(collection.key, core.NodeObservationReferenced)
		g.addProfileAttributes(terminalAt, profile)
		g.addProfileAddressEdges(terminalAt, collection.key, profile)
	}
}

// osValueSemantics は、端末のノードの属性にする CurrentVersion の値の名前と語彙の項目である。
var osValueSemantics = map[string]core.SemanticKey{
	"ProductName":    core.SemanticKeyTerminalOsProductName,
	"DisplayVersion": core.SemanticKeyTerminalOsDisplayVersion,
	"CurrentBuild":   core.SemanticKeyTerminalOsBuild,
	"UBR":            core.SemanticKeyTerminalOsUbr,
	"EditionID":      core.SemanticKeyTerminalOsEdition,
}

// addProfileAttributes は、registry が記録した OS の値を、値を持つ key のレコードを根拠にして
// 端末のノードの属性へ足す。
func (g *Graph) addProfileAttributes(terminalAt int, profile TerminalProfile) {
	for _, value := range profile.OperatingSystem {
		semantic, mapped := osValueSemantics[value.Name]
		recordAt, found := g.recordAtLocator(value.RecordRef)
		if !mapped || !found {
			continue
		}
		raw, err := core.NewRawValue(core.ValueStatePresent, value.Value)
		if err != nil {
			continue
		}
		field, err := core.NewTextField("Value."+value.Name, semantic, raw)
		if err != nil {
			continue
		}
		g.addAttribute(terminalAt, &field, recordAt)
	}
}

// addProfileAddressEdges は、registry が記録したインターフェースの IP アドレスを、端末から IP への
// terminal_address のエッジにする。根拠はインターフェースの key のレコードである。DHCP のリースを
// 得た時刻を持つ key のエッジは、リースの期間を持つ。
//
// 0.0.0.0 は、DHCP を使うインターフェースの IPAddress が持つ、アドレスを割り当てていない値である。
func (g *Graph) addProfileAddressEdges(terminalAt int, terminal core.NodeKey, profile TerminalProfile) {
	for _, address := range profile.Addresses {
		for _, value := range address.Values {
			if value.Name != "DhcpIPAddress" && value.Name != "IPAddress" {
				continue
			}
			recordAt, found := g.recordAtLocator(value.RecordRef)
			if !found {
				continue
			}
			for ip := range strings.FieldsSeq(value.Value) {
				if ip == "0.0.0.0" {
					continue
				}
				g.addProfileAddressEdge(terminalAt, terminal, value.Name, ip, recordAt, address)
			}
		}
	}
}

// addProfileAddressEdge は IP アドレス 1 つの terminal_address のエッジを足す。
func (g *Graph) addProfileAddressEdge(
	terminalAt int, terminal core.NodeKey, name, ip string, recordAt int, address TerminalAddress,
) {
	raw, err := core.NewRawValue(core.ValueStatePresent, ip)
	if err != nil {
		return
	}
	field, err := core.NewTextField("Value."+name, core.SemanticKeyTerminalIpAddress, raw)
	if err != nil {
		return
	}
	fields := []core.RecordField{field}
	// IP のノードの鍵は、レコードの項目から組むときと同じ経路で組む。
	linked := core.NewRecordGraphInScope(fields, core.RecordScope{Terminal: &terminal, NamesTerminal: true})
	for _, link := range linked.Links {
		if link.Kind != core.EdgeKindTerminalAddress {
			continue
		}
		ipAt := g.ensureNode(link.Target, core.NodeObservationReferenced)
		if label, found := nodeLabelOf(fields, link.Target); found {
			g.nodes[ipAt].applyLabel(label)
		}
		edge := g.ensureEdge(link.Kind, core.RelationStateObserved, terminalAt, ipAt)
		g.addEdgeEvidence(edge, recordAt)
		if address.From != nil && address.To != nil {
			held := g.edges[edge].ensureBasis()
			// 2 つのインターフェースが同じ IP を持つときは、早いリースの時刻を期間の始まりにする。
			from, _ := address.From.Instant()
			if held.lease == nil {
				held.lease = &core.TimeRange{From: *address.From, To: *address.To}
			} else if earlier, _ := held.lease.From.Instant(); from.Before(earlier) {
				held.lease.From = *address.From
			}
		}
	}
}

// terminalProfileOf は、1 つの収集の registry のレコードから端末の情報を組む。last は収集の
// 全レコードの時刻のうち最も遅い時刻である。
//
// **ControlSet の下の値は、Select の Current が選ぶ ControlSet だけから読む。** Select の key が
// 無い収集、または Current が 2 つ以上の値を持つ収集では、ControlSet の下の値を読まない。
func terminalProfileOf(records []registryRecord, last *core.Timestamp) TerminalProfile {
	var current string
	currents := 0
	for _, entry := range records {
		if !selectKey.MatchString(entry.path) {
			continue
		}
		if value, found := rawValueOf(entry.record.Semantics.Fields, "Value.Current"); found {
			if number, readable := dwordOf(value); readable {
				if set := fmt.Sprintf("ControlSet%03d", number); !strings.EqualFold(set, current) {
					current, currents = set, currents+1
				}
			}
		}
	}
	if currents != 1 {
		current = ""
	}
	selected := func(set string) bool { return current != "" && strings.EqualFold(set, current) }
	var profile TerminalProfile
	for _, entry := range records {
		fields := entry.record.Semantics.Fields
		switch {
		case osVersionKey.MatchString(entry.path):
			profile.OperatingSystem = append(profile.OperatingSystem, profileValuesOf(entry.record, osValueNames)...)
		case timeZoneKey.MatchString(entry.path):
			if selected(timeZoneKey.FindStringSubmatch(entry.path)[1]) {
				profile.TimeZone = append(profile.TimeZone, profileValuesOf(entry.record, timeZoneValueNames)...)
			}
		case tcpipInterfaceKey.MatchString(entry.path):
			parts := tcpipInterfaceKey.FindStringSubmatch(entry.path)
			values := profileValuesOf(entry.record, addressValueNames)
			if !selected(parts[1]) || len(values) == 0 {
				continue
			}
			address := TerminalAddress{Interface: parts[2], Values: values}
			if lease, found := rawValueOf(fields, "Value.LeaseObtainedTime"); found {
				if from, built := leaseTimestampOf(lease); built {
					address.From, address.To = &from, cloneTimestampPointer(last)
				}
			}
			profile.Addresses = append(profile.Addresses, address)
		}
	}
	return profile
}

// profileValuesOf は、レコードが持つ names の値を、names の順に返す。空の値は除く。
func profileValuesOf(record RecordEntry, names []string) []TerminalProfileValue {
	var values []TerminalProfileValue
	for _, name := range names {
		if value, found := rawValueOf(record.Semantics.Fields, "Value."+name); found && value != "" {
			values = append(values, TerminalProfileValue{Name: name, Value: value, RecordRef: cloneLocator(record.Locator)})
		}
	}
	return values
}

// rawValueOf は、name の項目の原資料の文字列を返す。
func rawValueOf(fields []core.RecordField, name string) (string, bool) {
	for _, field := range fields {
		if field.Name == name && field.Text != nil {
			return field.Text.RawTextValue()
		}
	}
	return "", false
}

// dwordOf は、registry の reader が DWORD を書いた `<10 進> (0x<16 進>)` の文字列から 10 進の値を返す。
func dwordOf(text string) (uint32, bool) {
	decimal, _, _ := strings.Cut(text, " ")
	value, err := strconv.ParseUint(decimal, 10, 32)
	return uint32(value), err == nil
}

// leaseTimestampOf は、LeaseObtainedTime の DWORD を UNIX の秒の時刻にする。0 の値は時刻を持たない。
func leaseTimestampOf(text string) (core.Timestamp, bool) {
	seconds, readable := dwordOf(text)
	if !readable || seconds == 0 {
		return core.Timestamp{}, false
	}
	normalized := time.Unix(int64(seconds), 0).UTC().Format(time.RFC3339)
	timestamp, err := core.NewTimestamp(core.Timestamp{
		RawText: &text, Normalized: &normalized,
		NormalizedForm: core.NormalizedFormRFC3339Absolute, Precision: core.PrecisionSecond,
		OffsetState: core.OffsetStateEpoch, Clock: core.ClockFileProperty, Meaning: core.MeaningProperty,
		ValueState: core.ValueStatePresent,
	})
	return timestamp, err == nil
}

// TerminalProfile は、収集の端末のノード nodeId について、収集の registry が記録した端末の情報を
// 返す。registry を持たない収集の端末では名前の記録 (Graph.TerminalNames) だけを持つ。収集の
// 端末でないノードでは全ての並びが要素数 0 である。
func (g Graph) TerminalProfile(nodeId string) TerminalProfile {
	profile, found := g.terminalProfiles[nodeId]
	if !found {
		return TerminalProfile{Names: g.TerminalNames(nodeId)}
	}
	profile.Names = cloneTerminalNames(profile.Names)
	return profile
}
