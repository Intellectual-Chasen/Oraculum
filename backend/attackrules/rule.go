package attackrules

import (
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

var (
	ruleIDPattern    = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*$`)
	variantIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	attackIDPattern  = regexp.MustCompile(`^T[0-9]{4}(?:\.[0-9]{3})?$`)
	rolePattern      = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)
	inputPattern     = regexp.MustCompile(`^[a-z][a-z0-9_.-]*$`)
)

const (
	// InputWindowsSysmon は実際に読み取った Sysmon telemetry を表す。
	InputWindowsSysmon = "windows.sysmon"
	// InputWindowsSecurity は実際に読み取った Windows Security telemetry を表す。
	InputWindowsSecurity = "windows.security"
)

var supportedInputs = []string{InputWindowsSecurity, InputWindowsSysmon}

var supportedPredicates = []string{
	"eq", "ieq", "in", "basename_in", "has_token", "prefix", "contains", "regex", "mask_any", "present", "absent",
}

// SupportedInputCapabilities は matcher が読み取れる input capability を返す。
func SupportedInputCapabilities() []string { return slices.Clone(supportedInputs) }

// SupportedValuePredicates は loader と matcher が扱う value predicate を返す。
func SupportedValuePredicates() []string { return slices.Clone(supportedPredicates) }

// Rule は、観測 graph に適用する ATT&CK 候補 rule を表す。
type Rule struct {
	Path              string      `yaml:"-" json:"-"`
	ID                string      `yaml:"id" json:"id"`
	Title             string      `yaml:"title" json:"title"`
	Description       string      `yaml:"description" json:"description"`
	References        []string    `yaml:"references" json:"references"`
	Attack            []AttackRef `yaml:"attack" json:"attack"`
	DistinguishesFrom []string    `yaml:"distinguishes_from" json:"distinguishesFrom"`
	Variants          []Variant   `yaml:"variants" json:"variants"`
}

// AttackRef は ATT&CK Technique または Sub-Technique と対応の根拠を表す。
type AttackRef struct {
	ID    string      `yaml:"id" json:"id"`
	Basis AttackBasis `yaml:"basis" json:"basis"`
}

// AttackBasis は ATT&CK 対応の根拠の区分である。
type AttackBasis string

const (
	// AttackBasisOfficial は公式資料が対応を直接示す根拠である。
	AttackBasisOfficial AttackBasis = "official"
	// AttackBasisInferred は観測形状から対応を推定する根拠である。
	AttackBasisInferred AttackBasis = "inferred"
)

// Variant は platform と観測条件ごとの pattern を表す。
type Variant struct {
	ID        string   `yaml:"id" json:"id"`
	Platforms []string `yaml:"platforms" json:"platforms"`
	Rationale string   `yaml:"rationale" json:"rationale"`
	Pattern   Pattern  `yaml:"pattern" json:"pattern"`
}

// Pattern は graph 上で評価可能な topology 条件を表す。
type Pattern struct {
	Hosts           []string               `yaml:"hosts" json:"hosts"`
	SameHostAllowed [][]string             `yaml:"same_host_allowed" json:"sameHostAllowed"`
	DistinctNodes   [][]string             `yaml:"distinct_nodes" json:"distinctNodes"`
	Inputs          []string               `yaml:"inputs" json:"inputs"`
	Nodes           map[string]PatternNode `yaml:"nodes" json:"nodes"`
	Edges           map[string]PatternEdge `yaml:"edges" json:"edges"`
	Joins           []Join                 `yaml:"joins" json:"joins"`
	Order           []Order                `yaml:"order" json:"order"`
	Evidence        Evidence               `yaml:"evidence" json:"evidence"`
}

// PatternNode は pattern 内の node role と条件を表す。
type PatternNode struct {
	Kind  string                    `yaml:"kind" json:"kind"`
	On    string                    `yaml:"on" json:"on"`
	Where map[string]ValuePredicate `yaml:"where" json:"where"`
}

// PatternEdge は pattern 内の edge role と有向端点を表す。
type PatternEdge struct {
	Kind  string                    `yaml:"kind" json:"kind"`
	From  string                    `yaml:"from" json:"from"`
	To    string                    `yaml:"to" json:"to"`
	Where map[string]ValuePredicate `yaml:"where" json:"where"`
}

// ValuePredicate は semantic value に適用する 1 つの演算子である。
type ValuePredicate struct {
	Eq         *string  `yaml:"eq" json:"eq"`
	IEq        *string  `yaml:"ieq" json:"ieq"`
	In         []string `yaml:"in" json:"in"`
	BasenameIn []string `yaml:"basename_in" json:"basenameIn"`
	HasToken   *string  `yaml:"has_token" json:"hasToken"`
	Prefix     *string  `yaml:"prefix" json:"prefix"`
	Contains   *string  `yaml:"contains" json:"contains"`
	Regex      *string  `yaml:"regex" json:"regex"`
	MaskAny    []string `yaml:"mask_any" json:"maskAny"`
	Present    *bool    `yaml:"present" json:"present"`
	Absent     *bool    `yaml:"absent" json:"absent"`
}

// Join は role の semantic value を比較する条件である。
type Join struct {
	Equal    []string `yaml:"equal" json:"equal"`
	IEq      []string `yaml:"ieq" json:"ieq"`
	HasToken []string `yaml:"has_token" json:"hasToken"`
	Contains []string `yaml:"contains" json:"contains"`
}

// Order は edge evidence の時間順と最大間隔を表す。
type Order struct {
	Before []string `yaml:"before" json:"before"`
	Within *int64   `yaml:"within" json:"within"`
}

// Evidence は成立に必要な edge role と補助 evidence の edge role を表す。
type Evidence struct {
	Required   []string `yaml:"required" json:"required"`
	Supporting []string `yaml:"supporting" json:"supporting"`
}

// Validate は rule の構造、参照先、語彙、predicate を検証する。
func (r Rule) Validate() error {
	if !ruleIDPattern.MatchString(r.ID) {
		return fmt.Errorf("invalid rule id %q", r.ID)
	}
	if strings.TrimSpace(r.Title) == "" || strings.TrimSpace(r.Description) == "" {
		return fmt.Errorf("rule %q: title and description are required", r.ID)
	}
	if len(r.References) == 0 {
		return fmt.Errorf("rule %q: references must not be empty", r.ID)
	}
	for _, reference := range r.References {
		parsed, err := url.ParseRequestURI(reference)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
			return fmt.Errorf("rule %q: invalid reference %q", r.ID, reference)
		}
	}
	if len(r.Attack) == 0 {
		return fmt.Errorf("rule %q: attack must not be empty", r.ID)
	}
	seenAttack := map[string]struct{}{}
	for _, ref := range r.Attack {
		if !attackIDPattern.MatchString(ref.ID) {
			return fmt.Errorf("rule %q: invalid ATT&CK id %q", r.ID, ref.ID)
		}
		if ref.Basis != AttackBasisOfficial && ref.Basis != AttackBasisInferred {
			return fmt.Errorf("rule %q: invalid ATT&CK basis %q", r.ID, ref.Basis)
		}
		if _, duplicate := seenAttack[ref.ID]; duplicate {
			return fmt.Errorf("rule %q: duplicate ATT&CK id %q", r.ID, ref.ID)
		}
		seenAttack[ref.ID] = struct{}{}
	}
	if len(r.Variants) == 0 {
		return fmt.Errorf("rule %q: variants must not be empty", r.ID)
	}
	seenVariants := map[string]struct{}{}
	for _, variant := range r.Variants {
		if !variantIDPattern.MatchString(variant.ID) {
			return fmt.Errorf("rule %q: invalid variant id %q", r.ID, variant.ID)
		}
		if _, duplicate := seenVariants[variant.ID]; duplicate {
			return fmt.Errorf("rule %q: duplicate variant id %q", r.ID, variant.ID)
		}
		seenVariants[variant.ID] = struct{}{}
		if err := variant.Pattern.Validate(); err != nil {
			return fmt.Errorf("rule %q variant %q: %w", r.ID, variant.ID, err)
		}
	}
	for _, other := range r.DistinguishesFrom {
		if !ruleIDPattern.MatchString(other) || other == r.ID {
			return fmt.Errorf("rule %q: invalid distinguishes_from rule id %q", r.ID, other)
		}
	}
	return nil
}

// Validate は pattern 全 field を matcher が評価できる形に限定する。
func (p Pattern) Validate() error {
	nodes := make(map[string]struct{}, len(p.Nodes)+len(p.Hosts))
	for _, host := range p.Hosts {
		if !rolePattern.MatchString(host) {
			return fmt.Errorf("invalid host role %q", host)
		}
		if _, duplicate := nodes[host]; duplicate {
			return fmt.Errorf("duplicate node role %q", host)
		}
		nodes[host] = struct{}{}
	}
	for role, node := range p.Nodes {
		if !rolePattern.MatchString(role) || !core.NodeKind(node.Kind).IsKnown() {
			return fmt.Errorf("invalid node role or kind %q: %q", role, node.Kind)
		}
		if _, duplicate := nodes[role]; duplicate {
			return fmt.Errorf("duplicate node role %q", role)
		}
		nodes[role] = struct{}{}
		if node.On != "" && !slices.Contains(p.Hosts, node.On) {
			return fmt.Errorf("node %q references undefined host %q", role, node.On)
		}
		if err := validateWhere(node.Where); err != nil {
			return fmt.Errorf("node %q: %w", role, err)
		}
	}
	if len(p.Edges) == 0 {
		return fmt.Errorf("pattern edges must not be empty")
	}
	edges := make(map[string]struct{}, len(p.Edges))
	usedNodes := make(map[string]struct{}, len(nodes))
	for role, edge := range p.Edges {
		if !rolePattern.MatchString(role) || !core.EdgeKind(edge.Kind).IsKnown() {
			return fmt.Errorf("invalid edge role or kind %q: %q", role, edge.Kind)
		}
		if _, duplicate := edges[role]; duplicate {
			return fmt.Errorf("duplicate edge role %q", role)
		}
		if _, ok := nodes[edge.From]; !ok {
			return fmt.Errorf("edge %q references undefined source node %q", role, edge.From)
		}
		usedNodes[edge.From] = struct{}{}
		if _, ok := nodes[edge.To]; !ok {
			return fmt.Errorf("edge %q references undefined target node %q", role, edge.To)
		}
		usedNodes[edge.To] = struct{}{}
		if err := validateWhere(edge.Where); err != nil {
			return fmt.Errorf("edge %q: %w", role, err)
		}
		edges[role] = struct{}{}
	}
	for role, node := range p.Nodes {
		if _, used := usedNodes[role]; !used {
			return fmt.Errorf("node role %q is unused", role)
		}
		if node.On != "" {
			usedNodes[node.On] = struct{}{}
		}
	}
	for _, host := range p.Hosts {
		if _, used := usedNodes[host]; !used {
			return fmt.Errorf("host role %q is unused", host)
		}
	}
	for _, pair := range p.DistinctNodes {
		if len(pair) != 2 || pair[0] == pair[1] {
			return fmt.Errorf("distinct_nodes entries require two distinct node roles")
		}
		for _, role := range pair {
			if _, ok := nodes[role]; !ok || slices.Contains(p.Hosts, role) {
				return fmt.Errorf("distinct_nodes references invalid node role %q", role)
			}
		}
	}
	for _, pair := range p.SameHostAllowed {
		if len(pair) != 2 || pair[0] == pair[1] {
			return fmt.Errorf("same_host_allowed entries require two distinct host roles")
		}
		for _, role := range pair {
			if !slices.Contains(p.Hosts, role) {
				return fmt.Errorf("same_host_allowed references undefined host role %q", role)
			}
		}
	}
	for _, input := range p.Inputs {
		if !inputPattern.MatchString(input) || !slices.Contains(supportedInputs, input) {
			return fmt.Errorf("unsupported input capability %q", input)
		}
	}
	if err := uniqueStrings("input", p.Inputs); err != nil {
		return err
	}
	for _, join := range p.Joins {
		operands, err := join.operands()
		if err != nil {
			return err
		}
		for _, operand := range operands {
			role, semantic, ok := splitOperand(operand)
			if !ok || !knownRole(role, nodes, edges) || !core.SemanticKey(semantic).IsKnown() {
				return fmt.Errorf("join references invalid operand %q", operand)
			}
		}
	}
	for _, order := range p.Order {
		if len(order.Before) != 2 || order.Before[0] == order.Before[1] ||
			order.Within != nil && (*order.Within < 0 || *order.Within > int64((1<<63-1)/int64(time.Second))) {
			return fmt.Errorf("order requires two distinct edges and a non-negative within")
		}
		for _, role := range order.Before {
			if _, ok := edges[role]; !ok {
				return fmt.Errorf("order references undefined edge %q", role)
			}
		}
	}
	for _, roles := range [][]string{p.Evidence.Required, p.Evidence.Supporting} {
		if err := uniqueStrings("evidence edge", roles); err != nil {
			return err
		}
		for _, role := range roles {
			if _, ok := edges[role]; !ok {
				return fmt.Errorf("evidence references undefined edge %q", role)
			}
		}
	}
	for _, required := range p.Evidence.Required {
		if slices.Contains(p.Evidence.Supporting, required) {
			return fmt.Errorf("edge %q cannot be both required and supporting evidence", required)
		}
	}
	for role := range edges {
		if !slices.Contains(p.Evidence.Required, role) && !slices.Contains(p.Evidence.Supporting, role) {
			return fmt.Errorf("edge %q has no evidence role", role)
		}
	}
	return nil
}

func validateWhere(where map[string]ValuePredicate) error {
	for semantic, predicate := range where {
		if !core.SemanticKey(semantic).IsKnown() {
			return fmt.Errorf("unknown semantic key %q", semantic)
		}
		if err := predicate.Validate(); err != nil {
			return fmt.Errorf("where[%q]: %w", semantic, err)
		}
	}
	return nil
}

// Validate は matcher が実装する value predicate を 1 つだけ許可する。
func (p ValuePredicate) Validate() error {
	count := 0
	for _, set := range []bool{
		p.Eq != nil, p.IEq != nil, len(p.In) > 0, len(p.BasenameIn) > 0,
		p.HasToken != nil, p.Prefix != nil, p.Contains != nil, p.Regex != nil,
		len(p.MaskAny) > 0, p.Present != nil, p.Absent != nil,
	} {
		if set {
			count++
		}
	}
	if count != 1 {
		return fmt.Errorf("predicate must specify exactly one supported operator")
	}
	if p.Regex != nil {
		if _, err := regexp.Compile(*p.Regex); err != nil {
			return fmt.Errorf("invalid regex: %w", err)
		}
	}
	if p.Present != nil && !*p.Present || p.Absent != nil && !*p.Absent {
		return fmt.Errorf("present and absent must be true when specified")
	}
	if err := uniqueStrings("predicate value", p.In); err != nil {
		return err
	}
	if err := uniqueStrings("predicate basename", p.BasenameIn); err != nil {
		return err
	}
	if err := uniqueStrings("predicate mask", p.MaskAny); err != nil {
		return err
	}
	for _, mask := range p.MaskAny {
		if _, err := strconv.ParseUint(mask, 0, 64); err != nil {
			return fmt.Errorf("invalid mask_any value %q: %w", mask, err)
		}
	}
	return nil
}

func (j Join) operands() ([]string, error) {
	var selected []string
	count := 0
	for _, values := range [][]string{j.Equal, j.IEq, j.HasToken, j.Contains} {
		if len(values) > 0 {
			count++
			selected = values
		}
	}
	if count != 1 || len(selected) != 2 || selected[0] == selected[1] {
		return nil, fmt.Errorf("join must specify exactly one operator with two distinct operands")
	}
	return selected, nil
}

func splitOperand(value string) (string, string, bool) {
	at := strings.IndexByte(value, '.')
	if at <= 0 || at == len(value)-1 {
		return "", "", false
	}
	return value[:at], value[at+1:], true
}

func knownRole(role string, nodes, edges map[string]struct{}) bool {
	if _, ok := nodes[role]; ok {
		return true
	}
	_, ok := edges[role]
	return ok
}

func uniqueStrings(label string, values []string) error {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s must not be empty", label)
		}
		if _, duplicate := seen[value]; duplicate {
			return fmt.Errorf("duplicate %s %q", label, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}
