package api

import (
	"net/http"
	"slices"
	"strings"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

const attackCandidatesPattern = "GET /api/v0/attack-candidates"

type attackCandidatesHandler struct {
	graph   pipeline.Graph
	ruleSet pipeline.AttackRuleSet
}

type attackCandidatesResponse struct {
	RuleSet      pipeline.AttackRuleSetInfo    `json:"ruleSet"`
	Rules        []attackCandidateRuleItem     `json:"rules"`
	Matches      []attackCandidateMatchItem    `json:"matches"`
	NotEvaluated []attackCandidateNotEvaluated `json:"notEvaluated"`
}

type attackCandidateRuleItem struct {
	ID                string                         `json:"id"`
	Title             string                         `json:"title"`
	Description       string                         `json:"description"`
	References        []string                       `json:"references"`
	Attack            []pipeline.AttackRuleReference `json:"attack"`
	DistinguishesFrom []string                       `json:"distinguishesFrom"`
	Variants          []attackCandidateVariantItem   `json:"variants"`
	MatchCount        int64                          `json:"matchCount"`
}

type attackCandidateVariantItem struct {
	ID         string                    `json:"id"`
	Platforms  []string                  `json:"platforms"`
	Rationale  string                    `json:"rationale"`
	Evaluation attackCandidateEvaluation `json:"evaluation"`
	MatchCount int64                     `json:"matchCount"`
}

type attackCandidateEvaluation struct {
	State  string `json:"state"`
	Reason string `json:"reason,omitempty"`
	Detail string `json:"detail,omitempty"`
}

type attackCandidateMatchItem struct {
	RuleID    string                       `json:"ruleId"`
	VariantID string                       `json:"variantId"`
	MatchID   string                       `json:"matchId"`
	Edges     []attackCandidateMatchedEdge `json:"edges"`
}

type attackCandidateNotEvaluated struct {
	RuleID    string `json:"ruleId"`
	VariantID string `json:"variantId"`
	Reason    string `json:"reason"`
	Detail    string `json:"detail"`
}

type attackCandidateMatchedEdge struct {
	Role                string                     `json:"role"`
	EvidenceRole        string                     `json:"evidenceRole"`
	Kind                core.EdgeKind              `json:"kind"`
	EdgeID              string                     `json:"edgeId"`
	SourceNode          core.GraphNode             `json:"sourceNode"`
	SinkNode            core.GraphNode             `json:"sinkNode"`
	AssignmentBases     []core.EdgeAssignmentBasis `json:"assignmentBases"`
	TerminalAssignments []core.TerminalAssignment  `json:"terminalAssignments,omitempty"`
	Evidence            []core.RecordLocator       `json:"evidence"`
}

func (h attackCandidatesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	request, apiError := parseGraphRequest(r)
	if apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	if apiError := checkKnownSearch(h.graph, request.search); apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}

	rules := slices.Clone(h.ruleSet.Rules)
	slices.SortFunc(rules, func(left, right pipeline.AttackRule) int {
		return strings.Compare(left.ID, right.ID)
	})
	evaluation, err := h.graph.AttackRuleMatches(request.query(), rules)
	if err != nil {
		writeError(w, http.StatusInternalServerError, core.ApiError{
			Code: core.ApiErrorCodeInternalError, Message: "evaluating ATT&CK rules failed",
		})
		return
	}
	response := attackCandidatesResponse{
		RuleSet: h.ruleSet.Info, Rules: make([]attackCandidateRuleItem, 0, len(rules)),
		Matches:      make([]attackCandidateMatchItem, 0, len(evaluation.Matches)),
		NotEvaluated: make([]attackCandidateNotEvaluated, 0, len(evaluation.NotEvaluated)),
	}
	counts := make(map[string]int64)
	for _, match := range evaluation.Matches {
		matchedEdges := make([]attackCandidateMatchedEdge, 0, len(match.Edges))
		for _, edge := range match.Edges {
			evidence := make([]core.RecordLocator, 0, len(edge.Evidence))
			for _, item := range edge.Evidence {
				evidence = append(evidence, item.RecordRef)
			}
			matchedEdges = append(matchedEdges, attackCandidateMatchedEdge{
				Role: edge.Role, EvidenceRole: edge.EvidenceRole, Kind: edge.Edge.Kind, EdgeID: edge.Edge.Id,
				SourceNode: edge.Source, SinkNode: edge.Sink,
				AssignmentBases:     emptyIfNil(edge.AssignmentBases),
				TerminalAssignments: edge.TerminalAssignments, Evidence: evidence,
			})
		}
		response.Matches = append(response.Matches, attackCandidateMatchItem{
			RuleID: match.RuleID, VariantID: match.VariantID, MatchID: match.MatchID, Edges: matchedEdges,
		})
		counts[ruleVariantKey(match.RuleID, match.VariantID)]++
	}
	notEvaluated := make(map[string]pipeline.AttackRuleNotEvaluated, len(evaluation.NotEvaluated))
	for _, item := range evaluation.NotEvaluated {
		key := ruleVariantKey(item.RuleID, item.VariantID)
		notEvaluated[key] = item
		response.NotEvaluated = append(response.NotEvaluated, attackCandidateNotEvaluated{
			RuleID: item.RuleID, VariantID: item.VariantID, Reason: item.Reason, Detail: item.Detail,
		})
	}
	for _, rule := range rules {
		item := attackCandidateRuleItem{
			ID: rule.ID, Title: rule.Title, Description: rule.Description,
			References: emptyIfNil(slices.Clone(rule.References)), Attack: emptyIfNil(slices.Clone(rule.Attack)),
			DistinguishesFrom: emptyIfNil(slices.Clone(rule.DistinguishesFrom)),
			Variants:          make([]attackCandidateVariantItem, 0, len(rule.Variants)),
		}
		variants := slices.Clone(rule.Variants)
		slices.SortFunc(variants, func(left, right pipeline.AttackRuleVariant) int {
			return strings.Compare(left.ID, right.ID)
		})
		for _, variant := range variants {
			key := ruleVariantKey(rule.ID, variant.ID)
			variantItem := attackCandidateVariantItem{
				ID: variant.ID, Platforms: emptyIfNil(slices.Clone(variant.Platforms)), Rationale: variant.Rationale,
				Evaluation: attackCandidateEvaluation{State: "evaluated"}, MatchCount: counts[key],
			}
			if skipped, found := notEvaluated[key]; found {
				variantItem.Evaluation = attackCandidateEvaluation{
					State: "not_evaluated", Reason: skipped.Reason, Detail: skipped.Detail,
				}
			}
			item.MatchCount += variantItem.MatchCount
			item.Variants = append(item.Variants, variantItem)
		}
		response.Rules = append(response.Rules, item)
	}
	writeJSON(w, http.StatusOK, response)
}

func ruleVariantKey(ruleID, variantID string) string { return ruleID + "\x00" + variantID }
