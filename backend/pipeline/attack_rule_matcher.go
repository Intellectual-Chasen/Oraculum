package pipeline

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/Intellectual-Chasen/Oraculum/backend/attackrules"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// AttackRuleSet は API と起動処理が共有する strict loader の結果である。
type AttackRuleSet = attackrules.Set

// AttackRuleSetInfo は runtime rule source の provenance である。
type AttackRuleSetInfo = attackrules.SetInfo

// AttackRule は API が公開する rule metadata である。
type AttackRule = attackrules.Rule

// AttackRuleReference は ATT&CK technique reference と basis である。
type AttackRuleReference = attackrules.AttackRef

// AttackRuleVariant は rule の platform-specific executable pattern である。
type AttackRuleVariant = attackrules.Variant

// AttackRuleEvaluation は rule の評価結果を保持する。
type AttackRuleEvaluation struct {
	Matches      []AttackRuleMatch
	NotEvaluated []AttackRuleNotEvaluated
}

// AttackRuleMatch は rule variant に結び付く 1 topology binding である。
type AttackRuleMatch struct {
	RuleID    string
	VariantID string
	MatchID   string
	Edges     []AttackRuleMatchedEdge
}

// AttackRuleMatchedEdge は match 内の role と有向 edge と evidence である。
type AttackRuleMatchedEdge struct {
	Role                string
	EvidenceRole        string
	Edge                core.GraphEdge
	Source              core.GraphNode
	Sink                core.GraphNode
	AssignmentBases     []core.EdgeAssignmentBasis
	TerminalAssignments []core.TerminalAssignment
	Evidence            []core.GraphEvidence
}

// AttackRuleNotEvaluated は必須 input が無く評価しなかった variant である。
type AttackRuleNotEvaluated struct {
	RuleID    string
	VariantID string
	Reason    string
	Detail    string
}

type attackRuleBinding struct {
	nodes map[string]int
	edges map[string]int
}

type attackRuleEdgeCandidate struct {
	at   int
	view core.GraphEdge
}

// attackRuleRegexCache は variant の評価中に使う正規表現を保持する。
type attackRuleRegexCache map[string]*regexp.Regexp

func (cache attackRuleRegexCache) compiled(pattern string) *regexp.Regexp {
	if compiled := cache[pattern]; compiled != nil {
		return compiled
	}
	compiled := regexp.MustCompile(pattern) // Rule.Validate checked the expression before matching.
	cache[pattern] = compiled
	return compiled
}

func indexAttackRuleCandidatesByKind(candidates []attackRuleEdgeCandidate) map[core.EdgeKind][]attackRuleEdgeCandidate {
	indexed := make(map[core.EdgeKind][]attackRuleEdgeCandidate)
	for _, candidate := range candidates {
		indexed[candidate.view.Kind] = append(indexed[candidate.view.Kind], candidate)
	}
	return indexed
}

// AttackRuleMatches は query 範囲内の graph topology を rule ごとに評価する。
func (g Graph) AttackRuleMatches(query GraphQuery, rules []attackrules.Rule) (AttackRuleEvaluation, error) {
	// Candidate output is a complete topology result, independent of the graph display cap.
	query.NodeLimit = 0
	visible := g.Query(query)
	observedInputs := g.ruleInputCapabilitiesInQuery(query)
	candidates := make([]attackRuleEdgeCandidate, 0, len(visible.Edges))
	for _, edge := range visible.Edges {
		at, found := g.edgeIndexOf(edge.Id)
		if found {
			candidates = append(candidates, attackRuleEdgeCandidate{at: at, view: edge})
		}
	}

	ordered := slices.Clone(rules)
	slices.SortFunc(ordered, func(left, right attackrules.Rule) int {
		return strings.Compare(left.ID, right.ID)
	})
	evaluation := AttackRuleEvaluation{
		Matches:      make([]AttackRuleMatch, 0),
		NotEvaluated: make([]AttackRuleNotEvaluated, 0),
	}
	for _, rule := range ordered {
		if err := rule.Validate(); err != nil {
			return AttackRuleEvaluation{}, fmt.Errorf("validating ATT&CK rule %q before matching: %w", rule.ID, err)
		}
		variants := slices.Clone(rule.Variants)
		slices.SortFunc(variants, func(left, right attackrules.Variant) int {
			return strings.Compare(left.ID, right.ID)
		})
		for _, variant := range variants {
			missing := missingRuleInputs(variant.Pattern.Inputs, observedInputs)
			if len(missing) > 0 {
				evaluation.NotEvaluated = append(evaluation.NotEvaluated, AttackRuleNotEvaluated{
					RuleID: rule.ID, VariantID: variant.ID, Reason: "missing_input",
					Detail: "required observed input capability is absent: " + strings.Join(missing, ", "),
				})
				continue
			}
			evaluation.Matches = append(evaluation.Matches,
				g.matchAttackRuleVariant(query, rule.ID, variant, candidates)...)
		}
	}
	slices.SortFunc(evaluation.Matches, compareAttackRuleMatches)
	slices.SortFunc(evaluation.NotEvaluated, func(left, right AttackRuleNotEvaluated) int {
		if result := strings.Compare(left.RuleID, right.RuleID); result != 0 {
			return result
		}
		return strings.Compare(left.VariantID, right.VariantID)
	})
	return evaluation, nil
}

func (g Graph) matchAttackRuleVariant(
	query GraphQuery, ruleID string, variant attackrules.Variant, candidates []attackRuleEdgeCandidate,
) []AttackRuleMatch {
	pattern := variant.Pattern
	edgeRoles := make([]string, 0, len(pattern.Edges))
	for role := range pattern.Edges {
		edgeRoles = append(edgeRoles, role)
	}
	slices.Sort(edgeRoles)
	candidatesByKind := indexAttackRuleCandidatesByKind(candidates)
	regexes := make(attackRuleRegexCache)
	binding := attackRuleBinding{nodes: make(map[string]int), edges: make(map[string]int)}
	matches := make([]AttackRuleMatch, 0)
	var bind func(int)
	bind = func(index int) {
		if index == len(edgeRoles) {
			if !g.ruleDistinctNodesMatch(pattern, binding) || !g.ruleJoinsMatch(pattern, binding, query, regexes) ||
				!g.ruleOrderMatches(pattern, binding, query, regexes) {
				return
			}
			matchedEdges := make([]AttackRuleMatchedEdge, 0, len(edgeRoles))
			for _, role := range edgeRoles {
				matched, ok := g.ruleMatchedEdge(pattern, role, binding.edges[role], query, regexes)
				if !ok {
					return
				}
				matchedEdges = append(matchedEdges, matched)
			}
			matches = append(matches, AttackRuleMatch{
				RuleID: ruleID, VariantID: variant.ID, MatchID: attackRuleMatchID(ruleID, variant.ID, binding, g),
				Edges: matchedEdges,
			})
			return
		}
		role := edgeRoles[index]
		patternEdge := pattern.Edges[role]
		for _, candidate := range candidatesByKind[core.EdgeKind(patternEdge.Kind)] {
			if _, reused := binding.edgesByIndex(candidate.at); reused {
				continue
			}
			matchedEvidence := g.ruleEdgeEvidence(candidate.at, patternEdge.Where, query, regexes)
			if len(patternEdge.Where) > 0 && len(matchedEvidence) == 0 {
				continue
			}
			added, ok := g.bindRuleEdge(pattern, binding, patternEdge, candidate.at, query, regexes)
			if !ok {
				continue
			}
			binding.edges[role] = candidate.at
			bind(index + 1)
			delete(binding.edges, role)
			for _, name := range added {
				delete(binding.nodes, name)
			}
		}
	}
	bind(0)
	return matches
}

func (b attackRuleBinding) edgesByIndex(at int) (string, bool) {
	for role, bound := range b.edges {
		if bound == at {
			return role, true
		}
	}
	return "", false
}

func (g Graph) bindRuleEdge(
	pattern attackrules.Pattern, binding attackRuleBinding, edge attackrules.PatternEdge, edgeAt int, query GraphQuery,
	regexes attackRuleRegexCache,
) ([]string, bool) {
	internal := g.edges[edgeAt]
	added := make([]string, 0, 2)
	for _, endpoint := range []struct {
		role string
		at   int
	}{{edge.From, internal.source}, {edge.To, internal.target}} {
		if current, bound := binding.nodes[endpoint.role]; bound {
			if current != endpoint.at {
				for _, role := range added {
					delete(binding.nodes, role)
				}
				return nil, false
			}
			continue
		}
		if !g.ruleNodeMatches(pattern, endpoint.role, endpoint.at, binding, query, regexes) {
			for _, role := range added {
				delete(binding.nodes, role)
			}
			return nil, false
		}
		binding.nodes[endpoint.role] = endpoint.at
		added = append(added, endpoint.role)
	}
	for role, node := range pattern.Nodes {
		nodeAt, bound := binding.nodes[role]
		if !bound || node.On == "" {
			continue
		}
		hostAt, found := g.ruleHostForNode(nodeAt)
		if !found {
			for _, name := range added {
				delete(binding.nodes, name)
			}
			return nil, false
		}
		if current, exists := binding.nodes[node.On]; exists {
			if current != hostAt {
				for _, name := range added {
					delete(binding.nodes, name)
				}
				return nil, false
			}
			continue
		}
		binding.nodes[node.On] = hostAt
		added = append(added, node.On)
	}
	if !g.ruleHostsAllowed(pattern, binding.nodes) || !g.ruleNodeHostScopesMatch(pattern, binding.nodes) {
		for _, role := range added {
			delete(binding.nodes, role)
		}
		return nil, false
	}
	return added, true
}

func (g Graph) ruleHostForNode(nodeAt int) (int, bool) {
	found := -1
	for hostAt, host := range g.nodes {
		if host.key.Kind != core.NodeKindTerminal || !g.ruleNodeOnHost(nodeAt, hostAt) {
			continue
		}
		if found >= 0 && found != hostAt {
			return 0, false
		}
		found = hostAt
	}
	return found, found >= 0
}

func (g Graph) ruleNodeMatches(
	pattern attackrules.Pattern, role string, nodeAt int, binding attackRuleBinding, query GraphQuery,
	regexes attackRuleRegexCache,
) bool {
	if slices.Contains(pattern.Hosts, role) {
		return g.nodes[nodeAt].key.Kind == core.NodeKindTerminal
	}
	node, declared := pattern.Nodes[role]
	if !declared || g.nodes[nodeAt].key.Kind != core.NodeKind(node.Kind) ||
		!g.ruleNodeWhereMatches(nodeAt, node.Where, query, regexes) {
		return false
	}
	if node.On != "" {
		if hostAt, bound := binding.nodes[node.On]; bound && !g.ruleNodeOnHost(nodeAt, hostAt) {
			return false
		}
	}
	return true
}

func (g Graph) ruleNodeHostScopesMatch(pattern attackrules.Pattern, bindings map[string]int) bool {
	for role, node := range pattern.Nodes {
		if node.On == "" {
			continue
		}
		nodeAt, hasNode := bindings[role]
		hostAt, hasHost := bindings[node.On]
		if hasNode && hasHost && !g.ruleNodeOnHost(nodeAt, hostAt) {
			return false
		}
	}
	return true
}

func (g Graph) ruleHostsAllowed(pattern attackrules.Pattern, bindings map[string]int) bool {
	allowed := make(map[string]struct{}, len(pattern.SameHostAllowed))
	for _, pair := range pattern.SameHostAllowed {
		allowed[canonicalRolePair(pair[0], pair[1])] = struct{}{}
	}
	for index, leftRole := range pattern.Hosts {
		left, leftBound := bindings[leftRole]
		if !leftBound {
			continue
		}
		for _, rightRole := range pattern.Hosts[index+1:] {
			right, rightBound := bindings[rightRole]
			if !rightBound || left != right {
				continue
			}
			if _, ok := allowed[canonicalRolePair(leftRole, rightRole)]; !ok {
				return false
			}
		}
	}
	return true
}

func canonicalRolePair(left, right string) string {
	if right < left {
		left, right = right, left
	}
	return left + "\x00" + right
}

func (g Graph) ruleNodeOnHost(nodeAt, hostAt int) bool {
	if nodeAt == hostAt || g.nodes[hostAt].key.Kind != core.NodeKindTerminal {
		return nodeAt == hostAt
	}
	for _, edgeAt := range g.adjacency[nodeAt].outgoing {
		edge := g.edges[edgeAt]
		if edge.target == hostAt && edge.kind == core.EdgeKindRanOn {
			return true
		}
	}
	for _, edgeAt := range g.adjacency[nodeAt].incoming {
		edge := g.edges[edgeAt]
		if edge.source == hostAt && (edge.kind == core.EdgeKindTerminalAddress || edge.kind == core.EdgeKindTerminalAccount) {
			return true
		}
	}
	nodeValues, hostValues := g.nodes[nodeAt].key.Values, g.nodes[hostAt].key.Values
	if len(nodeValues) < len(hostValues) || len(hostValues) == 0 {
		return false
	}
	for index, hostValue := range hostValues {
		if nodeValues[index].Value != hostValue.Value || nodeValues[index].Semantic != hostValue.Semantic {
			return false
		}
	}
	return true
}

func (g Graph) ruleDistinctNodesMatch(pattern attackrules.Pattern, binding attackRuleBinding) bool {
	for _, pair := range pattern.DistinctNodes {
		left, hasLeft := binding.nodes[pair[0]]
		right, hasRight := binding.nodes[pair[1]]
		if !hasLeft || !hasRight || left == right {
			return false
		}
	}
	return true
}

func (g Graph) ruleNodeWhereMatches(nodeAt int, where map[string]attackrules.ValuePredicate, query GraphQuery, regexes attackRuleRegexCache) bool {
	for name, predicate := range where {
		values, present := g.ruleNodeValues(nodeAt, core.SemanticKey(name), query)
		if !matchRulePredicate(predicate, values, present, regexes) {
			return false
		}
	}
	return true
}

func (g Graph) ruleNodeValues(nodeAt int, semantic core.SemanticKey, query GraphQuery) ([]string, bool) {
	node := g.nodes[nodeAt]
	values := make([]string, 0)
	present := false
	for _, identity := range node.key.Values {
		if identity.Semantic == semantic {
			present = true
			values = append(values, identity.Value)
		}
	}
	for _, attribute := range node.attributes {
		if attribute.field.Semantic != semantic || len(g.filteredEvidence(attribute.evidence, query)) == 0 {
			continue
		}
		present = true
		if value, readable := comparableFieldValue(*attribute.field); readable {
			values = append(values, value)
		}
	}
	if semantic.Role() == core.SemanticRoleIdentity {
		if kind, isNode := core.NodeKindOf(semantic.Object()); isNode && kind == node.key.Kind {
			for _, identity := range node.key.Values {
				if identity.Semantic == "" {
					present = true
					values = append(values, identity.Value)
				}
			}
		}
	}
	slices.Sort(values)
	return slices.Compact(values), present
}

func (g Graph) ruleEdgeEvidence(
	edgeAt int, where map[string]attackrules.ValuePredicate, query GraphQuery, regexes attackRuleRegexCache,
) []int {
	evidence := g.filteredEvidence(g.edges[edgeAt].evidence, query)
	if len(where) == 0 {
		return evidence
	}
	matched := make([]int, 0, len(evidence))
	for _, recordAt := range evidence {
		if g.ruleRecordWhereMatches(recordAt, where, regexes) {
			matched = append(matched, recordAt)
		}
	}
	return matched
}

func (g Graph) ruleRecordWhereMatches(recordAt int, where map[string]attackrules.ValuePredicate, regexes attackRuleRegexCache) bool {
	if recordAt < 0 || recordAt >= len(g.records) || !g.records[recordAt].hasRecordNode {
		return false
	}
	return g.ruleNodeWhereMatches(g.records[recordAt].recordNode, where, GraphQuery{}, regexes)
}

func (g Graph) ruleMatchedEdge(
	pattern attackrules.Pattern, role string, edgeAt int, query GraphQuery, regexes attackRuleRegexCache,
) (AttackRuleMatchedEdge, bool) {
	where := pattern.Edges[role].Where
	evidence := g.ruleEdgeEvidence(edgeAt, where, query, regexes)
	if slices.Contains(pattern.Evidence.Required, role) && len(evidence) == 0 {
		return AttackRuleMatchedEdge{}, false
	}
	internal := g.edges[edgeAt]
	evidenceRole := "supporting"
	if slices.Contains(pattern.Evidence.Required, role) {
		evidenceRole = "required"
	}
	return AttackRuleMatchedEdge{
		Role: role, EvidenceRole: evidenceRole,
		Edge: g.responseEdge(internal, evidence), Source: g.graphNode(internal.source), Sink: g.graphNode(internal.target),
		AssignmentBases:     cloneAssignmentBases(internal.assignmentBasisList()),
		TerminalAssignments: cloneAssignments(internal.terminalAssignmentList()),
		Evidence:            g.evidenceItems(evidence),
	}, true
}

func (g Graph) ruleJoinsMatch(pattern attackrules.Pattern, binding attackRuleBinding, query GraphQuery, regexes attackRuleRegexCache) bool {
	for _, join := range pattern.Joins {
		operands := ruleJoinOperands(join)
		left, leftPresent := g.ruleOperandValues(operands[0], binding, pattern, query, regexes)
		right, rightPresent := g.ruleOperandValues(operands[1], binding, pattern, query, regexes)
		if !leftPresent || !rightPresent {
			return false
		}
		matched := false
		for _, a := range left {
			for _, b := range right {
				if ruleJoinMatches(join, a, b) {
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func ruleJoinOperands(join attackrules.Join) []string {
	for _, operands := range [][]string{join.Equal, join.IEq, join.HasToken, join.Contains} {
		if len(operands) != 0 {
			return operands
		}
	}
	return nil
}

func ruleJoinMatches(join attackrules.Join, left, right string) bool {
	switch {
	case len(join.Equal) > 0:
		return left == right
	case len(join.IEq) > 0:
		return strings.EqualFold(left, right)
	case len(join.HasToken) > 0:
		return hasRuleToken(left, right)
	case len(join.Contains) > 0:
		return strings.Contains(left, right)
	default:
		return false
	}
}

func (g Graph) ruleOperandValues(
	operand string, binding attackRuleBinding, pattern attackrules.Pattern, query GraphQuery, regexes attackRuleRegexCache,
) ([]string, bool) {
	role, semantic, ok := splitRuleOperand(operand)
	if !ok {
		return nil, false
	}
	if nodeAt, found := binding.nodes[role]; found {
		return g.ruleNodeValues(nodeAt, semantic, query)
	}
	edgeAt, found := binding.edges[role]
	if !found {
		return nil, false
	}
	values := make([]string, 0)
	present := false
	for _, recordAt := range g.ruleEdgeEvidence(edgeAt, pattern.Edges[role].Where, query, regexes) {
		if !g.records[recordAt].hasRecordNode {
			continue
		}
		one, isPresent := g.ruleNodeValues(g.records[recordAt].recordNode, semantic, query)
		present = present || isPresent
		values = append(values, one...)
	}
	slices.Sort(values)
	return slices.Compact(values), present
}

func splitRuleOperand(value string) (string, core.SemanticKey, bool) {
	at := strings.IndexByte(value, '.')
	if at <= 0 || at == len(value)-1 {
		return "", "", false
	}
	return value[:at], core.SemanticKey(value[at+1:]), true
}

func (g Graph) ruleOrderMatches(pattern attackrules.Pattern, binding attackRuleBinding, query GraphQuery, regexes attackRuleRegexCache) bool {
	for _, order := range pattern.Order {
		beforeAt, hasBefore := binding.edges[order.Before[0]]
		afterAt, hasAfter := binding.edges[order.Before[1]]
		if !hasBefore || !hasAfter {
			return false
		}
		beforeTimes := g.ruleEdgeTimes(beforeAt, pattern.Edges[order.Before[0]].Where, query, regexes)
		afterTimes := g.ruleEdgeTimes(afterAt, pattern.Edges[order.Before[1]].Where, query, regexes)
		matched := false
		for _, before := range beforeTimes {
			for _, after := range afterTimes {
				if !after.After(before) || order.Within != nil &&
					after.Sub(before) > time.Duration(*order.Within)*time.Second {
					continue
				}
				matched = true
				break
			}
			if matched {
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func (g Graph) ruleEdgeTimes(edgeAt int, where map[string]attackrules.ValuePredicate, query GraphQuery, regexes attackRuleRegexCache) []time.Time {
	times := make([]time.Time, 0)
	for _, recordAt := range g.ruleEdgeEvidence(edgeAt, where, query, regexes) {
		if g.records[recordAt].hasInstant {
			times = append(times, g.records[recordAt].instant)
		}
	}
	return times
}

func matchRulePredicate(predicate attackrules.ValuePredicate, values []string, present bool, regexes attackRuleRegexCache) bool {
	if predicate.Present != nil {
		return present
	}
	if predicate.Absent != nil {
		return !present
	}
	for _, value := range values {
		switch {
		case predicate.Eq != nil && value == *predicate.Eq:
			return true
		case predicate.IEq != nil && strings.EqualFold(value, *predicate.IEq):
			return true
		case len(predicate.In) > 0 && slices.Contains(predicate.In, value):
			return true
		case len(predicate.BasenameIn) > 0 && basenameInRule(value, predicate.BasenameIn):
			return true
		case predicate.HasToken != nil && hasRuleToken(value, *predicate.HasToken):
			return true
		case predicate.Prefix != nil && strings.HasPrefix(value, *predicate.Prefix):
			return true
		case predicate.Contains != nil && strings.Contains(value, *predicate.Contains):
			return true
		case predicate.Regex != nil && regexes.compiled(*predicate.Regex).MatchString(value):
			return true
		case len(predicate.MaskAny) > 0 && maskAnyRule(value, predicate.MaskAny):
			return true
		}
	}
	return false
}

func basenameInRule(value string, expected []string) bool {
	base := filepath.Base(strings.ReplaceAll(value, "\\", "/"))
	return slices.ContainsFunc(expected, func(item string) bool { return strings.EqualFold(base, item) })
}

func hasRuleToken(value, expected string) bool {
	tokens := strings.FieldsFunc(value, func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune(`"'(),;=|&`, r)
	})
	return slices.ContainsFunc(tokens, func(token string) bool { return strings.EqualFold(token, expected) })
}

func maskAnyRule(value string, masks []string) bool {
	number, err := strconv.ParseUint(value, 0, 64)
	if err != nil {
		return false
	}
	for _, maskText := range masks {
		mask, err := strconv.ParseUint(maskText, 0, 64)
		if err == nil && number&mask != 0 {
			return true
		}
	}
	return false
}

func attackRuleMatchID(ruleID, variantID string, binding attackRuleBinding, graph Graph) string {
	var identity strings.Builder
	writeRuleMatchPart(&identity, ruleID)
	writeRuleMatchPart(&identity, variantID)
	nodeRoles := make([]string, 0, len(binding.nodes))
	for role := range binding.nodes {
		nodeRoles = append(nodeRoles, role)
	}
	slices.Sort(nodeRoles)
	for _, role := range nodeRoles {
		writeRuleMatchPart(&identity, role)
		writeRuleMatchPart(&identity, graph.nodes[binding.nodes[role]].id)
	}
	edgeRoles := make([]string, 0, len(binding.edges))
	for role := range binding.edges {
		edgeRoles = append(edgeRoles, role)
	}
	slices.Sort(edgeRoles)
	for _, role := range edgeRoles {
		writeRuleMatchPart(&identity, role)
		writeRuleMatchPart(&identity, graph.edges[binding.edges[role]].id)
	}
	digest := sha256.Sum256([]byte(identity.String()))
	return "acm:" + hex.EncodeToString(digest[:])
}

func writeRuleMatchPart(builder *strings.Builder, value string) {
	_, _ = fmt.Fprintf(builder, "%d:", len(value))
	builder.WriteString(value)
}

func compareAttackRuleMatches(left, right AttackRuleMatch) int {
	if order := strings.Compare(left.RuleID, right.RuleID); order != 0 {
		return order
	}
	if order := strings.Compare(left.VariantID, right.VariantID); order != 0 {
		return order
	}
	return strings.Compare(left.MatchID, right.MatchID)
}

func missingRuleInputs(required []string, observed map[string]struct{}) []string {
	missing := make([]string, 0)
	for _, input := range required {
		if _, present := observed[input]; !present {
			missing = append(missing, input)
		}
	}
	slices.Sort(missing)
	return slices.Compact(missing)
}

func (g Graph) ruleInputCapabilitiesInQuery(query GraphQuery) map[string]struct{} {
	query = g.withResolvedFieldNames(query)
	matchedNodes, _, reachableEdges := g.reach(query)
	records := make(map[int]struct{})
	for _, node := range matchedNodes {
		for _, at := range g.nodes[node].evidence {
			records[at] = struct{}{}
		}
	}
	for _, edge := range reachableEdges {
		for _, at := range g.edges[edge].evidence {
			records[at] = struct{}{}
		}
	}
	observed := make(map[string]struct{})
	for at := range records {
		if !g.recordMatches(at, query.RecordFilter) {
			continue
		}
		if capability := g.records[at].inputCapability; capability != "" {
			observed[capability] = struct{}{}
		}
	}
	return observed
}

func observeInputCapability(fields []core.RecordField) string {
	channel := firstComparableSemantic(fields, core.SemanticKeyWindowsEventChannel)
	provider := firstComparableSemantic(fields, core.SemanticKeyWindowsEventProvider)
	channelLower, providerLower := strings.ToLower(channel), strings.ToLower(provider)
	capability := ""
	switch {
	case strings.Contains(channelLower, "sysmon/operational") || strings.Contains(providerLower, "microsoft-windows-sysmon"):
		capability = attackrules.InputWindowsSysmon
	case strings.EqualFold(channel, "Security"), strings.EqualFold(channel, "セキュリティ"),
		strings.EqualFold(provider, "Microsoft-Windows-Security-Auditing"):
		capability = attackrules.InputWindowsSecurity
	}
	return capability
}

func firstComparableSemantic(fields []core.RecordField, semantic core.SemanticKey) string {
	for _, field := range fields {
		if field.Semantic == semantic {
			if value, readable := comparableFieldValue(field); readable {
				return value
			}
		}
	}
	return ""
}
