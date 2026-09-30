package attackrules

import (
	"strings"
	"testing"
)

const executableThreeEdgeRuleYAML = `id: synthetic.three-edge
title: Three edge topology
description: Synthetic topology.
references: [https://example.test/rules/three-edge]
attack: [{id: T1055, basis: inferred}]
variants:
  - id: default
    pattern:
      nodes: {origin: {kind: process}, middle: {kind: process}, target: {kind: process}}
      edges:
        first: {kind: process_parent_child, from: origin, to: middle}
        second: {kind: process_parent_child, from: middle, to: target}
        third: {kind: process_injection, from: origin, to: target}
      evidence: {required: [first, second, third]}
`

func TestDecodeAcceptsExecutableTopologyRule(t *testing.T) {
	data := []byte(`id: synthetic.three-edge
title: Three edge topology
description: Synthetic generic topology rule.
references: [https://example.test/rules/three-edge]
attack:
  - id: T1055.001
    basis: inferred
distinguishes_from: [synthetic.other]
variants:
  - id: linux-process-chain
    platforms: [Linux]
    rationale: The chain is represented by three observed process edges.
    pattern:
      hosts: [origin_host, target_host]
      same_host_allowed: [[origin_host, target_host]]
      distinct_nodes: [[origin, terminal]]
      inputs: [windows.sysmon]
      nodes:
        origin:
          kind: process
          on: origin_host
          where:
            process.binary_path:
              basename_in: [agent, helper]
        middle:
          kind: process
        terminal:
          kind: process
          on: target_host
          where:
            process.command_line:
              has_token: --inspect
      edges:
        first:
          kind: process_parent_child
          from: origin
          to: middle
          where:
            process.user_name:
              ieq: system
        second:
          kind: process_parent_child
          from: middle
          to: terminal
        third:
          kind: process_injection
          from: origin
          to: terminal
      joins:
        - equal: [origin.process.id, third.injection_target_process.id]
      order:
        - before: [first, second]
          within: 30
      evidence:
        required: [first, third]
        supporting: [second]
`)

	rule, err := Decode("synthetic.three-edge.yaml", data)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if rule.ID != "synthetic.three-edge" || rule.Variants[0].Pattern.Edges["third"].Kind != "process_injection" {
		t.Fatalf("Decode() returned unexpected rule: %+v", rule)
	}
}

func TestDecodeRejectsUnsupportedFutureSyntax(t *testing.T) {
	for _, field := range []string{"not_exists", "paths"} {
		t.Run(field, func(t *testing.T) {
			data := []byte(`id: synthetic.unsupported
title: Unsupported syntax
description: Synthetic rule.
references: [https://example.test/rules/unsupported]
attack: [{id: T1055, basis: official}]
variants:
  - id: default
    pattern:
      nodes: {source: {kind: process}, target: {kind: process}}
      edges: {edge: {kind: process_injection, from: source, to: target}}
      ` + field + `: []
`)
			_, err := Decode("synthetic.unsupported.yaml", data)
			if err == nil || !strings.Contains(err.Error(), field) {
				t.Fatalf("Decode() error = %v, want rejection mentioning %q", err, field)
			}
		})
	}
}

func TestDecodeRejectsUnsupportedInputCapabilityAndInvalidMask(t *testing.T) {
	base := `id: synthetic.invalid
title: Invalid executable rule
description: Invalid input or mask.
references: [https://example.test/rules/invalid]
attack: [{id: T1055, basis: official}]
variants:
  - id: default
    pattern:
      inputs: [process_events]
      nodes: {source: {kind: process}, target: {kind: process}}
      edges:
        edge:
          kind: process_injection
          from: source
          to: target
          where: {process.id: {mask_any: ["0x1"]}}
      evidence: {required: [edge]}
`
	if _, err := Decode("synthetic.invalid.yaml", []byte(base)); err == nil || !strings.Contains(err.Error(), "unsupported input capability") {
		t.Fatalf("Decode() error = %v, want unsupported input capability", err)
	}
	base = strings.Replace(base, "inputs: [process_events]", "inputs: [windows.sysmon]", 1)
	base = strings.Replace(base, `mask_any: ["0x1"]`, "mask_any: [not-a-mask]", 1)
	if _, err := Decode("synthetic.invalid.yaml", []byte(base)); err == nil || !strings.Contains(err.Error(), "invalid mask_any") {
		t.Fatalf("Decode() error = %v, want invalid mask_any", err)
	}
}

func TestDecodeRejectsDuplicateYAMLKey(t *testing.T) {
	data := []byte(`id: synthetic.duplicate
title: First title
title: Second title
description: Synthetic rule.
references: [https://example.test/rules/duplicate]
attack: [{id: T1055, basis: official}]
variants:
  - id: default
    pattern:
      nodes: {source: {kind: process}, target: {kind: process}}
      edges: {edge: {kind: process_injection, from: source, to: target}}
      evidence: {required: [edge]}
`)
	_, err := Decode("synthetic.duplicate.yaml", data)
	if err == nil || !strings.Contains(err.Error(), "already defined") {
		t.Fatalf("Decode() error = %v, want duplicate key error", err)
	}
}

func TestRuleValidateRejectsDuplicateVariantID(t *testing.T) {
	rule, err := Decode("synthetic.three-edge.yaml", []byte(executableThreeEdgeRuleYAML))
	if err != nil {
		t.Fatal(err)
	}
	rule.Variants = append(rule.Variants, rule.Variants[0])
	if err := rule.Validate(); err == nil || !strings.Contains(err.Error(), "duplicate variant id") {
		t.Fatalf("Validate() error = %v, want duplicate variant ID", err)
	}
}

func TestDecodeRejectsWrongFilenameAndMultipleDocuments(t *testing.T) {
	base := `id: synthetic.one
title: One
description: One synthetic rule.
references: [https://example.test/rules/one]
attack: [{id: T1055, basis: official}]
variants:
  - id: default
    pattern:
      nodes: {source: {kind: process}, target: {kind: process}}
      edges: {edge: {kind: process_injection, from: source, to: target}}
      evidence: {required: [edge]}
`
	if _, err := Decode("other.yaml", []byte(base)); err == nil || !strings.Contains(err.Error(), "file name") {
		t.Fatalf("Decode() wrong filename error = %v", err)
	}
	if _, err := Decode("synthetic.one.yaml", []byte(base+"---\nid: synthetic.two\n")); err == nil || !strings.Contains(err.Error(), "multiple YAML documents") {
		t.Fatalf("Decode() multiple document error = %v", err)
	}
}

func TestDecodeRejectsInvalidReferencesTopologyAndPredicates(t *testing.T) {
	base := `id: synthetic.valid
title: Valid
description: Synthetic rule.
references: [https://example.test/rules/valid]
attack: [{id: T1055, basis: official}]
variants:
  - id: default
    pattern:
      nodes: {source: {kind: process}, target: {kind: process}}
      edges: {edge: {kind: process_injection, from: source, to: target}}
      evidence: {required: [edge]}
`
	tests := []struct {
		name  string
		input string
	}{
		{name: "invalid rule id", input: strings.Replace(base, "synthetic.valid", "Bad ID", 1)},
		{name: "invalid variant id", input: strings.Replace(base, "id: default", "id: Bad ID", 1)},
		{name: "invalid attack id", input: strings.Replace(base, "T1055", "T10", 1)},
		{name: "invalid attack basis", input: strings.Replace(base, "basis: official", "basis: guessed", 1)},
		{name: "invalid reference", input: strings.Replace(base, "https://example.test/rules/valid", "relative/path", 1)},
		{name: "invalid endpoint", input: strings.Replace(base, "to: target", "to: missing", 1)},
		{name: "unknown semantic", input: strings.Replace(base, "kind: process_injection", "kind: process_injection, where: {unknown.semantic: {eq: x}}", 1)},
		{name: "invalid predicate", input: strings.Replace(base, "kind: process_injection", "kind: process_injection, where: {event.category: {regex: '['}}", 1)},
		{name: "unknown predicate operator", input: strings.Replace(base, "kind: process_injection", "kind: process_injection, where: {event.category: {matches: x}}", 1)},
		{name: "invalid join operand", input: strings.Replace(base, "edges: {edge:", "joins: [{equal: [missing.process.id, edge.process.id]}]\n      edges: {edge:", 1)},
		{name: "invalid order edge", input: strings.Replace(base, "edges: {edge:", "order: [{before: [edge, missing]}]\n      edges: {edge:", 1)},
		{name: "invalid evidence role", input: strings.Replace(base, "edges: {edge:", "evidence: {required: [missing]}\n      edges: {edge:", 1)},
		{name: "invalid distinguishes from", input: strings.Replace(base, "variants:", "distinguishes_from: [synthetic.valid]\nvariants:", 1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Decode("synthetic.valid.yaml", []byte(test.input)); err == nil {
				t.Fatalf("Decode() accepted invalid %s", test.name)
			}
		})
	}
}

func TestDecodeRejectsMalformedYAMLAndUnknownFields(t *testing.T) {
	valid := `id: synthetic.valid
title: Valid
description: Synthetic rule.
references: [https://example.test/rules/valid]
attack: [{id: T1055, basis: official}]
variants:
  - id: default
    pattern:
      nodes: {source: {kind: process}, target: {kind: process}}
      edges: {edge: {kind: process_injection, from: source, to: target}}
      evidence: {required: [edge]}
`
	for _, input := range []string{
		strings.Replace(valid, "description:", "unknown: true\ndescription:", 1),
		"id: [malformed\n",
	} {
		if _, err := Decode("synthetic.valid.yaml", []byte(input)); err == nil {
			t.Fatalf("Decode() accepted invalid YAML: %q", input)
		}
	}
}
