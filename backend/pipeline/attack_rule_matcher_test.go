package pipeline

import (
	"reflect"
	"slices"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/attackrules"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

func TestAttackRuleMatchesThreeEdgeTopologyGenerically(t *testing.T) {
	records := []RecordEntry{
		processTopologyRecord(t, 1, "origin", "", ""),
		processTopologyRecord(t, 2, "middle", "origin", ""),
		processTopologyRecord(t, 3, "sink", "middle", ""),
		processTopologyRecord(t, 4, "origin", "", "sink"),
	}
	graph := graphOfRecords(t, records)
	rule := attackrules.Rule{
		ID: "synthetic.three-edge", Title: "Three edge topology", Description: "Synthetic topology",
		References: []string{"https://example.test/rules/three-edge"},
		Attack:     []attackrules.AttackRef{{ID: "T1055", Basis: attackrules.AttackBasisInferred}},
		Variants: []attackrules.Variant{{ID: "default", Pattern: attackrules.Pattern{
			Nodes: map[string]attackrules.PatternNode{
				"origin": {Kind: string(core.NodeKindProcess)},
				"middle": {Kind: string(core.NodeKindProcess)},
				"sink":   {Kind: string(core.NodeKindProcess)},
			},
			Edges: map[string]attackrules.PatternEdge{
				"first":  {Kind: string(core.EdgeKindProcessParentChild), From: "origin", To: "middle"},
				"second": {Kind: string(core.EdgeKindProcessParentChild), From: "middle", To: "sink"},
				"third":  {Kind: string(core.EdgeKindProcessInjection), From: "origin", To: "sink"},
			},
			DistinctNodes: [][]string{{"origin", "sink"}},
			Evidence: attackrules.Evidence{
				Required: []string{"first", "third"}, Supporting: []string{"second"},
			},
		}}},
	}
	evaluation, err := graph.AttackRuleMatches(GraphQuery{Depth: 1}, []attackrules.Rule{rule})
	if err != nil {
		t.Fatalf("AttackRuleMatches() error = %v", err)
	}
	if len(evaluation.NotEvaluated) != 0 || len(evaluation.Matches) != 1 {
		t.Fatalf("AttackRuleMatches() = %+v; want one evaluated three-edge match", evaluation)
	}
	match := evaluation.Matches[0]
	if len(match.Edges) != len(rule.Variants[0].Pattern.Edges) {
		t.Fatalf("matched edge roles = %+v; want every edge in the topology", match.Edges)
	}
	for _, role := range []string{"first", "second", "third"} {
		found := false
		for _, edge := range match.Edges {
			found = found || edge.Role == role
		}
		if !found {
			t.Errorf("match omitted edge role %q", role)
		}
	}
	for _, edge := range match.Edges {
		if edge.Role == "second" && (edge.EvidenceRole != "supporting" || len(edge.Evidence) == 0) {
			t.Errorf("supporting edge = %+v; want supporting role and evidence", edge)
		}
	}
}

func TestRuleRegexPredicateReusesItsCompiledExpression(t *testing.T) {
	pattern := `^agent\.exe$`
	predicate := attackrules.ValuePredicate{Regex: &pattern}
	cache := attackRuleRegexCache{}
	if !matchRulePredicate(predicate, []string{"agent.exe"}, true, cache) {
		t.Fatal("matchRulePredicate() did not match the regex")
	}
	compiled := cache[pattern]
	if compiled == nil {
		t.Fatal("matchRulePredicate() did not cache its compiled regex")
	}
	if !matchRulePredicate(predicate, []string{"agent.exe"}, true, cache) || cache[pattern] != compiled {
		t.Fatal("matchRulePredicate() did not reuse its compiled regex")
	}
}

func TestAttackRuleCandidatesAreIndexedByEdgeKind(t *testing.T) {
	candidates := []attackRuleEdgeCandidate{
		{at: 4, view: core.GraphEdge{Kind: core.EdgeKindProcessParentChild}},
		{at: 7, view: core.GraphEdge{Kind: core.EdgeKindProcessInjection}},
		{at: 9, view: core.GraphEdge{Kind: core.EdgeKindProcessParentChild}},
	}
	indexed := indexAttackRuleCandidatesByKind(candidates)
	got := indexed[core.EdgeKindProcessParentChild]
	if len(got) != 2 || got[0].at != 4 || got[1].at != 9 {
		t.Fatalf("parent-child candidates = %+v; want candidate indices [4 9] in input order", got)
	}
	if got := indexed[core.EdgeKindProcessInjection]; len(got) != 1 || got[0].at != 7 {
		t.Fatalf("injection candidates = %+v; want candidate index [7]", got)
	}
}

func TestAttackRuleMatchesExistingRemoteServicesRuleThroughGenericEngine(t *testing.T) {
	rule := attackrules.Rule{
		ID: "attack.t1021.remote-services", Title: "Remote Services", Description: "Remote session topology",
		References: []string{"https://attack.mitre.org/techniques/T1021/"},
		Attack:     []attackrules.AttackRef{{ID: "T1021", Basis: attackrules.AttackBasisInferred}},
		Variants: []attackrules.Variant{{ID: "default", Pattern: attackrules.Pattern{
			Hosts: []string{"session_source", "remote_terminal"},
			Nodes: map[string]attackrules.PatternNode{"client_ip": {Kind: string(core.NodeKindIp)}},
			Edges: map[string]attackrules.PatternEdge{
				"session": {Kind: string(core.EdgeKindTerminalRemoteSession), From: "session_source", To: "remote_terminal"},
				"address": {Kind: string(core.EdgeKindTerminalAddress), From: "session_source", To: "client_ip"},
			},
			Joins:    []attackrules.Join{{Equal: []string{"session.connection.source_address", "client_ip.terminal.ip_address"}}},
			Evidence: attackrules.Evidence{Required: []string{"session", "address"}},
		}}},
	}
	evaluation, err := sessionGraph(t).AttackRuleMatches(GraphQuery{Depth: 1}, []attackrules.Rule{rule})
	if err != nil {
		t.Fatalf("AttackRuleMatches() error = %v", err)
	}
	if len(evaluation.NotEvaluated) != 0 || len(evaluation.Matches) != 1 {
		t.Fatalf("T1021 evaluation = %+v; want one evaluated match", evaluation)
	}
	match := evaluation.Matches[0]
	if match.RuleID != rule.ID || match.VariantID != "default" || len(match.Edges) != len(rule.Variants[0].Pattern.Edges) {
		t.Fatalf("T1021 topology match = %+v", match)
	}
	if match.Edges[0].EvidenceRole != "required" || len(match.Edges[0].Evidence) == 0 || len(match.Edges[1].Evidence) == 0 {
		t.Fatalf("T1021 evidence = %+v; want evidence for both required edges", match.Edges)
	}
}

func TestAttackRuleInputsDistinguishMissingFromZeroMatches(t *testing.T) {
	rule := singleInjectionRule()
	rule.Variants[0].Pattern.Inputs = []string{"windows.sysmon"}
	noCapability := graphOfRecords(t, []RecordEntry{processTopologyRecord(t, 1, "origin", "", "target")})
	missing, err := noCapability.AttackRuleMatches(GraphQuery{Depth: 1}, []attackrules.Rule{rule})
	if err != nil || len(missing.Matches) != 0 || len(missing.NotEvaluated) != 1 || missing.NotEvaluated[0].Reason != "missing_input" {
		t.Fatalf("missing input evaluation = %+v, %v; want not evaluated with missing_input", missing, err)
	}

	observedNoTopology := graphOfRecords(t, []RecordEntry{inputCapabilityRecord(t, 1)})
	zero, err := observedNoTopology.AttackRuleMatches(GraphQuery{Depth: 1}, []attackrules.Rule{rule})
	if err != nil || len(zero.NotEvaluated) != 0 || len(zero.Matches) != 0 {
		t.Fatalf("observed input with no topology = %+v, %v; want evaluated zero matches", zero, err)
	}

	observedTopology := graphOfRecords(t, []RecordEntry{
		processTopologyRecord(t, 1, "origin", "", "target"), inputCapabilityRecord(t, 2),
	})
	matched, err := observedTopology.AttackRuleMatches(GraphQuery{Depth: 1}, []attackrules.Rule{rule})
	if err != nil || len(matched.NotEvaluated) != 0 || len(matched.Matches) != 1 {
		t.Fatalf("observed input with topology = %+v, %v; want one match", matched, err)
	}
}

func TestAttackRuleInputsUseTheRequestedCaseScope(t *testing.T) {
	withInput := settleSource(t, 0)
	withInput.Plan.FileName = "sysmon.xml"
	withInput.Plan.CaseId = stringPointer("case-with-input")
	withInput.Records = []RecordEntry{windowsCapabilityRecord(t, 61,
		"Microsoft-Windows-Sysmon/Operational", "")}
	withInput.Records[0].Locator.SourceFileName = withInput.Plan.FileName
	withoutInput := settleSource(t, 0)
	withoutInput.Plan.FileName = "other.xml"
	withoutInput.Plan.CaseId = stringPointer("case-without-input")
	withoutInput.Records = []RecordEntry{windowsCapabilityRecord(t, 62, "Other/Operational", "Other-Provider")}
	withoutInput.Records[0].Locator.SourceFileName = withoutInput.Plan.FileName
	result, err := newImportResult([]scannedSource{withInput, withoutInput},
		[]core.ImportStatus{
			settleStatus(t, withInput, "source-with-input"),
			settleStatus(t, withoutInput, "source-without-input"),
		}, "run", settleRawTextRef)
	if err != nil {
		t.Fatal(err)
	}
	result.identities = map[string]core.SourceIdentity{
		"source-with-input":    {SourceId: "source-with-input", CaseId: stringPointer("case-with-input")},
		"source-without-input": {SourceId: "source-without-input", CaseId: stringPointer("case-without-input")},
	}
	graph := NewGraph(result, AllMatchConditions())
	if graph.caseOfSource["source-without-input"] != "case-without-input" {
		t.Fatal("the fixture does not scope the second source to case-without-input")
	}
	rule := singleInjectionRule()
	rule.Variants[0].Pattern.Inputs = []string{"windows.sysmon"}

	evaluation, err := graph.AttackRuleMatches(GraphQuery{
		Depth: 1, RecordFilter: RecordFilter{Case: "case-without-input"},
	}, []attackrules.Rule{rule})
	if err != nil || len(evaluation.Matches) != 0 || len(evaluation.NotEvaluated) != 1 ||
		evaluation.NotEvaluated[0].Reason != "missing_input" {
		t.Fatalf("case-scoped evaluation = %+v, %v; want missing_input for case-without-input", evaluation, err)
	}
}

func TestAttackRuleInputsUseTheRequestedTopologyScope(t *testing.T) {
	withInputFields := []core.RecordField{
		syntheticField(t, "Channel", core.SemanticKeyWindowsEventChannel, "Microsoft-Windows-Sysmon/Operational"),
	}
	graph := graphOfRecords(t, []RecordEntry{
		processTopologyRecord(t, 71, "outside-origin", "", "outside-target", withInputFields...),
		processTopologyRecord(t, 72, "inside-origin", "", "inside-target"),
	})
	if graph.records[1].processNode == 0 {
		t.Fatal("fixture does not record the in-scope process node")
	}
	nodeIndex := int(graph.records[1].processNode - 1)
	rule := singleInjectionRule()
	rule.Variants[0].Pattern.Inputs = []string{attackrules.InputWindowsSysmon}

	evaluation, err := graph.AttackRuleMatches(GraphQuery{
		NodeIds:   []string{graph.nodes[nodeIndex].id},
		Depth:     1,
		EdgeKinds: []core.EdgeKind{core.EdgeKindProcessInjection},
	}, []attackrules.Rule{rule})
	if err != nil || len(evaluation.Matches) != 0 || len(evaluation.NotEvaluated) != 1 ||
		evaluation.NotEvaluated[0].Reason != "missing_input" {
		t.Fatalf("topology-scoped evaluation = %+v, %v; want missing_input outside the visible query topology", evaluation, err)
	}
}

func TestObservedInputCapabilitiesMatchTheExecutableRegistry(t *testing.T) {
	records := []RecordEntry{
		windowsCapabilityRecord(t, 1, "Microsoft-Windows-Sysmon/Operational", "Microsoft-Windows-Sysmon"),
		windowsCapabilityRecord(t, 2, "Security", "Microsoft-Windows-Security-Auditing"),
		windowsCapabilityRecord(t, 3, "Example/Operational", "Example-Provider"),
	}
	got := make([]string, 0, len(records)-1)
	for _, record := range records {
		if capability := observeInputCapability(graphFieldsOf(record)); capability != "" {
			got = append(got, capability)
		}
	}
	slices.Sort(got)
	want := attackrules.SupportedInputCapabilities()
	slices.Sort(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("observed capabilities = %v; schema/runtime registry = %v", got, want)
	}
}

func TestAttackRuleMatchIDIsStableAndBindingSpecific(t *testing.T) {
	graph := graphOfRecords(t, []RecordEntry{
		processTopologyRecord(t, 1, "origin-a", "", "target-a"),
		processTopologyRecord(t, 2, "origin-b", "", "target-b"),
	})
	rule := singleInjectionRule()
	first, err := graph.AttackRuleMatches(GraphQuery{Depth: 1}, []attackrules.Rule{rule})
	if err != nil || len(first.Matches) != 2 {
		t.Fatalf("first evaluation = %+v, %v; want two topology bindings", first, err)
	}
	second, err := graph.AttackRuleMatches(GraphQuery{Depth: 1}, []attackrules.Rule{rule})
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("repeated evaluation differs: first=%+v second=%+v error=%v", first, second, err)
	}
	if first.Matches[0].MatchID == first.Matches[1].MatchID {
		t.Fatalf("distinct bindings share MatchID %q", first.Matches[0].MatchID)
	}
	reordered := reverseAttackCandidateTestEdges(graph)
	reorderedMatches, err := reordered.AttackRuleMatches(GraphQuery{Depth: 1}, []attackrules.Rule{rule})
	if err != nil || !reflect.DeepEqual(first, reorderedMatches) {
		t.Fatalf("map/edge iteration changed matches: first=%+v reordered=%+v error=%v", first, reorderedMatches, err)
	}
}

func TestAttackRulePredicateOperatorsEvaluatePositiveAndNegativeValues(t *testing.T) {
	value := func(input string) *string { return &input }
	truth := true
	tests := []struct {
		name      string
		predicate attackrules.ValuePredicate
		input     string
		present   bool
		want      bool
	}{
		{name: "eq", predicate: attackrules.ValuePredicate{Eq: value("Agent.exe")}, input: "Agent.exe", present: true, want: true},
		{name: "ieq", predicate: attackrules.ValuePredicate{IEq: value("agent.exe")}, input: "Agent.exe", present: true, want: true},
		{name: "in", predicate: attackrules.ValuePredicate{In: []string{"agent.exe", "helper.exe"}}, input: "agent.exe", present: true, want: true},
		{name: "basename_in", predicate: attackrules.ValuePredicate{BasenameIn: []string{"agent.exe"}}, input: `C:\Tools\Agent.exe`, present: true, want: true},
		{name: "has_token", predicate: attackrules.ValuePredicate{HasToken: value("--inspect")}, input: "agent.exe --inspect file.bin", present: true, want: true},
		{name: "prefix", predicate: attackrules.ValuePredicate{Prefix: value("agent")}, input: "agent.exe", present: true, want: true},
		{name: "contains", predicate: attackrules.ValuePredicate{Contains: value("gent.ex")}, input: "agent.exe", present: true, want: true},
		{name: "regex", predicate: attackrules.ValuePredicate{Regex: value(`^agent\.exe$`)}, input: "agent.exe", present: true, want: true},
		{name: "mask_any", predicate: attackrules.ValuePredicate{MaskAny: []string{"0x04", "0x10"}}, input: "0x14", present: true, want: true},
		{name: "present", predicate: attackrules.ValuePredicate{Present: &truth}, present: true, want: true},
		{name: "absent", predicate: attackrules.ValuePredicate{Absent: &truth}, present: false, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := matchRulePredicate(test.predicate, []string{test.input}, test.present, make(attackRuleRegexCache)); got != test.want {
				t.Errorf("positive predicate = %t, want %t", got, test.want)
			}
			if got := matchRulePredicate(test.predicate, []string{"different"}, !test.present, make(attackRuleRegexCache)); got {
				t.Errorf("negative predicate = %t, want false", got)
			}
		})
	}
	got := make([]string, 0, len(tests))
	for _, test := range tests {
		got = append(got, test.name)
	}
	slices.Sort(got)
	want := attackrules.SupportedValuePredicates()
	slices.Sort(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("schema predicate operators %v differ from tested matcher operators %v", want, got)
	}
}

func TestAttackRuleJoinOperatorsEvaluate(t *testing.T) {
	tests := []struct {
		name  string
		join  attackrules.Join
		left  string
		right string
	}{
		{name: "equal", join: attackrules.Join{Equal: []string{"a.process.id", "b.process.id"}}, left: "process-a", right: "process-a"},
		{name: "ieq", join: attackrules.Join{IEq: []string{"a.process.user_name", "b.process.user_name"}}, left: "SYSTEM", right: "system"},
		{name: "has_token", join: attackrules.Join{HasToken: []string{"a.process.command_line", "b.process.command_line"}}, left: "agent.exe --inspect file.bin", right: "--inspect"},
		{name: "contains", join: attackrules.Join{Contains: []string{"a.process.command_line", "b.process.command_line"}}, left: "agent.exe --inspect", right: "inspect"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if !ruleJoinMatches(test.join, test.left, test.right) {
				t.Fatalf("ruleJoinMatches(%q, %q) = false", test.left, test.right)
			}
			if ruleJoinMatches(test.join, test.left, "unmatched") {
				t.Fatalf("ruleJoinMatches(%q, unmatched) = true", test.left)
			}
		})
	}
}

func TestAttackRuleMissingRequiredEvidenceDoesNotMatch(t *testing.T) {
	graph := graphOfRecords(t, []RecordEntry{processTopologyRecord(t, 1, "origin", "", "target")})
	for index := range graph.edges {
		if graph.edges[index].kind == core.EdgeKindProcessInjection {
			graph.edges[index].evidence = nil
		}
	}
	evaluation, err := graph.AttackRuleMatches(GraphQuery{Depth: 1}, []attackrules.Rule{singleInjectionRule()})
	if err != nil || len(evaluation.Matches) != 0 {
		t.Fatalf("empty required evidence evaluation = %+v, %v; want no matches", evaluation, err)
	}
}

func TestAttackRuleEdgeWhereFiltersEvidenceInQueryScope(t *testing.T) {
	graph := graphOfRecords(t, []RecordEntry{
		processTopologyRecord(t, 1, "origin", "", "target", syntheticField(t, "evtCategory", core.SemanticKeyEventCategory, "process")),
		processTopologyRecord(t, 2, "origin", "", "target", syntheticField(t, "evtCategory", core.SemanticKeyEventCategory, "network")),
	})
	rule := singleInjectionRule()
	patternEdge := rule.Variants[0].Pattern.Edges["injection"]
	patternEdge.Where = map[string]attackrules.ValuePredicate{
		string(core.SemanticKeyEventCategory): {Eq: stringPointer("process")},
	}
	rule.Variants[0].Pattern.Edges["injection"] = patternEdge
	evaluation, err := graph.AttackRuleMatches(GraphQuery{Depth: 1}, []attackrules.Rule{rule})
	if err != nil || len(evaluation.Matches) != 1 || len(evaluation.Matches[0].Edges[0].Evidence) != 1 {
		t.Fatalf("edge where evaluation = %+v, %v; want one matching evidence record", evaluation, err)
	}
	negative := GraphQuery{Depth: 1, RecordFilter: RecordFilter{EventCategory: "absent"}}
	filtered, err := graph.AttackRuleMatches(negative, []attackrules.Rule{rule})
	if err != nil || len(filtered.Matches) != 0 {
		t.Fatalf("negative edge where evaluation = %+v, %v; want no match", filtered, err)
	}
}

func TestAttackRuleNodeWherePositiveAndNegative(t *testing.T) {
	graph := graphOfRecords(t, []RecordEntry{
		processTopologyRecord(t, 1, "origin", "", "target", syntheticField(t, "user", core.SemanticKeyProcessUserName, "SYSTEM")),
	})
	rule := singleInjectionRule()
	rule.Variants[0].Pattern.Nodes["source"] = attackrules.PatternNode{
		Kind: string(core.NodeKindProcess),
		Where: map[string]attackrules.ValuePredicate{
			string(core.SemanticKeyProcessUserName): {IEq: stringPointer("system")},
		},
	}
	evaluation, err := graph.AttackRuleMatches(GraphQuery{Depth: 1}, []attackrules.Rule{rule})
	if err != nil || len(evaluation.Matches) != 1 {
		t.Fatalf("positive node where evaluation = %+v, %v", evaluation, err)
	}
	rule.Variants[0].Pattern.Nodes["source"] = attackrules.PatternNode{
		Kind: string(core.NodeKindProcess),
		Where: map[string]attackrules.ValuePredicate{
			string(core.SemanticKeyProcessUserName): {Eq: stringPointer("other")},
		},
	}
	negative, err := graph.AttackRuleMatches(GraphQuery{Depth: 1}, []attackrules.Rule{rule})
	if err != nil || len(negative.Matches) != 0 {
		t.Fatalf("negative node where evaluation = %+v, %v", negative, err)
	}
}

func TestAttackRuleOrderBeforeAndWithin(t *testing.T) {
	first := processTopologyRecord(t, 1, "origin", "", "")
	first.ObservedAt = matchTestTime(t, 10)
	second := processTopologyRecord(t, 2, "middle", "origin", "")
	second.ObservedAt = matchTestTime(t, 12)
	third := processTopologyRecord(t, 3, "sink", "middle", "")
	third.ObservedAt = matchTestTime(t, 16)
	injection := processTopologyRecord(t, 4, "origin", "", "sink")
	injection.ObservedAt = matchTestTime(t, 18)
	graph := graphOfRecords(t, []RecordEntry{first, second, third, injection})
	rule := threeEdgeRule()
	within := int64(5)
	rule.Variants[0].Pattern.Order = []attackrules.Order{{Before: []string{"first", "second"}, Within: &within}}
	evaluation, err := graph.AttackRuleMatches(GraphQuery{Depth: 1}, []attackrules.Rule{rule})
	if err != nil || len(evaluation.Matches) != 1 {
		t.Fatalf("ordered topology = %+v, %v; want before edge within five seconds", evaluation, err)
	}
	within = 0
	zeroWindow, err := graph.AttackRuleMatches(GraphQuery{Depth: 1}, []attackrules.Rule{rule})
	if err != nil || len(zeroWindow.Matches) != 0 {
		t.Fatalf("zero-second order window = %+v, %v; want no match", zeroWindow, err)
	}
	within = 3
	outsideWindow, err := graph.AttackRuleMatches(GraphQuery{Depth: 1}, []attackrules.Rule{rule})
	if err != nil || len(outsideWindow.Matches) != 0 {
		t.Fatalf("topology outside within window = %+v, %v; want no match", outsideWindow, err)
	}
	rule.Variants[0].Pattern.Order[0] = attackrules.Order{Before: []string{"second", "first"}}
	reversed, err := graph.AttackRuleMatches(GraphQuery{Depth: 1}, []attackrules.Rule{rule})
	if err != nil || len(reversed.Matches) != 0 {
		t.Fatalf("reversed order = %+v, %v; want no match", reversed, err)
	}
}

func TestAttackRuleDistinctNodesAndHostBindings(t *testing.T) {
	separate := graphOfRecords(t, []RecordEntry{processTopologyRecord(t, 1, "origin", "", "target")})
	rule := singleInjectionRule()
	rule.Variants[0].Pattern.DistinctNodes = [][]string{{"source", "target"}}
	positive, err := separate.AttackRuleMatches(GraphQuery{Depth: 1}, []attackrules.Rule{rule})
	if err != nil || len(positive.Matches) != 1 {
		t.Fatalf("distinct endpoint graph = %+v, %v; want one match", positive, err)
	}
	same := graphOfRecords(t, []RecordEntry{processTopologyRecord(t, 1, "same", "", "same")})
	negative, err := same.AttackRuleMatches(GraphQuery{Depth: 1}, []attackrules.Rule{rule})
	if err != nil || len(negative.Matches) != 0 {
		t.Fatalf("same-node graph = %+v, %v; want distinct_nodes rejection", negative, err)
	}

	for _, allowSame := range []bool{false, true} {
		hostRule := singleInjectionRule()
		hostRule.Variants[0].Pattern.Hosts = []string{"source_host", "target_host"}
		hostRule.Variants[0].Pattern.Nodes["source"] = attackrules.PatternNode{Kind: string(core.NodeKindProcess), On: "source_host"}
		hostRule.Variants[0].Pattern.Nodes["target"] = attackrules.PatternNode{Kind: string(core.NodeKindProcess), On: "target_host"}
		if allowSame {
			hostRule.Variants[0].Pattern.SameHostAllowed = [][]string{{"source_host", "target_host"}}
		}
		evaluation, err := separate.AttackRuleMatches(GraphQuery{Depth: 1}, []attackrules.Rule{hostRule})
		want := 0
		if allowSame {
			want = 1
		}
		if err != nil || len(evaluation.Matches) != want {
			t.Fatalf("same-host allowed=%t evaluation = %+v, %v; want %d matches", allowSame, evaluation, err, want)
		}
	}

	remote := genericRemoteServicesRule()
	crossHost, err := sessionGraph(t).AttackRuleMatches(GraphQuery{Depth: 1}, []attackrules.Rule{remote})
	if err != nil || len(crossHost.Matches) != 1 {
		t.Fatalf("cross-host remote session = %+v, %v; want one match", crossHost, err)
	}
}

func singleInjectionRule() attackrules.Rule {
	return attackrules.Rule{
		ID: "synthetic.injection", Title: "Injection", Description: "Synthetic injection",
		References: []string{"https://example.test/rules/injection"},
		Attack:     []attackrules.AttackRef{{ID: "T1055", Basis: attackrules.AttackBasisOfficial}},
		Variants: []attackrules.Variant{{ID: "default", Pattern: attackrules.Pattern{
			Nodes: map[string]attackrules.PatternNode{
				"source": {Kind: string(core.NodeKindProcess)},
				"target": {Kind: string(core.NodeKindProcess)},
			},
			Edges: map[string]attackrules.PatternEdge{
				"injection": {Kind: string(core.EdgeKindProcessInjection), From: "source", To: "target"},
			},
			Evidence: attackrules.Evidence{Required: []string{"injection"}},
		}}},
	}
}

func threeEdgeRule() attackrules.Rule {
	return attackrules.Rule{
		ID: "synthetic.three-edge", Title: "Three edge topology", Description: "Synthetic topology",
		References: []string{"https://example.test/rules/three-edge"},
		Attack:     []attackrules.AttackRef{{ID: "T1055", Basis: attackrules.AttackBasisInferred}},
		Variants: []attackrules.Variant{{ID: "default", Pattern: attackrules.Pattern{
			Nodes: map[string]attackrules.PatternNode{
				"origin": {Kind: string(core.NodeKindProcess)},
				"middle": {Kind: string(core.NodeKindProcess)},
				"sink":   {Kind: string(core.NodeKindProcess)},
			},
			Edges: map[string]attackrules.PatternEdge{
				"first":  {Kind: string(core.EdgeKindProcessParentChild), From: "origin", To: "middle"},
				"second": {Kind: string(core.EdgeKindProcessParentChild), From: "middle", To: "sink"},
				"third":  {Kind: string(core.EdgeKindProcessInjection), From: "origin", To: "sink"},
			},
			DistinctNodes: [][]string{{"origin", "sink"}},
			Evidence: attackrules.Evidence{
				Required: []string{"first", "third"}, Supporting: []string{"second"},
			},
		}}},
	}
}

func genericRemoteServicesRule() attackrules.Rule {
	return attackrules.Rule{
		ID: "attack.t1021.remote-services", Title: "Remote Services", Description: "Remote session topology",
		References: []string{"https://attack.mitre.org/techniques/T1021/"},
		Attack:     []attackrules.AttackRef{{ID: "T1021", Basis: attackrules.AttackBasisOfficial}},
		Variants: []attackrules.Variant{{ID: "default", Pattern: attackrules.Pattern{
			Hosts: []string{"session_source", "remote_terminal"},
			Nodes: map[string]attackrules.PatternNode{"client_ip": {Kind: string(core.NodeKindIp)}},
			Edges: map[string]attackrules.PatternEdge{
				"session": {Kind: string(core.EdgeKindTerminalRemoteSession), From: "session_source", To: "remote_terminal"},
				"address": {Kind: string(core.EdgeKindTerminalAddress), From: "session_source", To: "client_ip"},
			},
			Joins:    []attackrules.Join{{Equal: []string{"session.connection.source_address", "client_ip.terminal.ip_address"}}},
			Evidence: attackrules.Evidence{Required: []string{"session", "address"}},
		}}},
	}
}

func foundationAttackRules(t *testing.T) []attackrules.Rule {
	t.Helper()
	set, err := LoadAttackRuleSet("../rules/attack")
	if err != nil {
		t.Fatal(err)
	}
	return set.Rules
}

func reverseAttackCandidateTestEdges(graph Graph) Graph {
	reversed := graph
	reversed.edges = slices.Clone(graph.edges)
	slices.Reverse(reversed.edges)
	reversed.edgeAt = make(map[string]int, len(reversed.edges))
	reversed.candidateEdgeAt = nil
	for index, edge := range reversed.edges {
		reversed.edgeAt[edge.id] = index
	}
	reversed.adjacency = make([]nodeAdjacency, len(graph.adjacency))
	for index, adjacency := range graph.adjacency {
		outgoing, incoming := slices.Clone(adjacency.outgoing), slices.Clone(adjacency.incoming)
		for edgeIndex := range outgoing {
			outgoing[edgeIndex] = len(reversed.edges) - 1 - outgoing[edgeIndex]
		}
		for edgeIndex := range incoming {
			incoming[edgeIndex] = len(reversed.edges) - 1 - incoming[edgeIndex]
		}
		reversed.adjacency[index] = nodeAdjacency{outgoing: outgoing, incoming: incoming}
	}
	return reversed
}

func inputCapabilityRecord(t *testing.T, line int64) RecordEntry {
	return windowsCapabilityRecord(t, line, "Microsoft-Windows-Sysmon/Operational", "")
}

func windowsCapabilityRecord(t *testing.T, line int64, channel, provider string) RecordEntry {
	return RecordEntry{
		Locator: core.RecordLocator{SourceFileName: "synthetic.log", PositionKind: core.PositionKindLineNumber, LineNumber: &line},
		Semantics: &RecordSemantics{
			ObservationKind: core.ObservationKind{Raw: []core.RecordField{}},
			Fields: []core.RecordField{
				syntheticField(t, "Channel", core.SemanticKeyWindowsEventChannel, channel),
				syntheticField(t, "Provider", core.SemanticKeyWindowsEventProvider, provider),
			},
		},
	}
}

func stringPointer(value string) *string { return &value }

func processTopologyRecord(
	t *testing.T, line int64, processID, parentID, injectionTargetID string, extra ...core.RecordField,
) RecordEntry {
	t.Helper()
	fields := []core.RecordField{
		syntheticField(t, "tmid", core.SemanticKeyTerminalId, "terminal-a"),
		syntheticField(t, "psGUID", core.SemanticKeyProcessId, processID),
	}
	if parentID != "" {
		fields = append(fields, syntheticField(t, "ppGUID", core.SemanticKeyParentProcessId, parentID))
	}
	if injectionTargetID != "" {
		fields = append(fields, syntheticField(t, "tpsGUID", core.SemanticKeyInjectionTargetProcessId, injectionTargetID))
	}
	fields = append(fields, extra...)
	return RecordEntry{
		Locator: core.RecordLocator{SourceFileName: "synthetic.log", PositionKind: core.PositionKindLineNumber, LineNumber: &line},
		Semantics: &RecordSemantics{
			ObservationKind: core.ObservationKind{Raw: []core.RecordField{}},
			Fields:          fields,
		},
	}
}
