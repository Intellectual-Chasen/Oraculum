package pipeline

import (
	"maps"
	"slices"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// caseOfSource は収集元に付けた案件を返す。案件を区別しない取り込みの収集元では空の文字列である。
func (r ImportResult) caseOfSource(sourceId string) string {
	identity, found := r.identities[sourceId]
	if !found || identity.CaseId == nil {
		return ""
	}
	return *identity.CaseId
}

// caseScopes は取り込み結果を、案件ごとの取り込み結果に分ける。並びは案件が最初に現れた
// 収集元の入力順である。案件を区別しない取り込みは、元の結果 1 つを返す。
//
// **関係を導く関連付けは、案件ごとの結果に対して走らせる。** 案件が違うレコードを 1 つの関連付けに
// 入れると、別の期間に記録された平常時のプロセスが別の案件の子プロセスの親に選ばれる。
// ノードは案件をまたいで共有し、関係と根拠を案件の中で導く。
//
// 分析者が与えた端末の割当は、期間を読み取った収集元と、端末を与える収集元の両方が
// その案件にあるものだけを残す。
func (r ImportResult) caseScopes() []ImportResult {
	var order []string
	byCase := make(map[string][]SourcePublication)
	for _, publication := range r.publications {
		caseId := r.caseOfSource(publication.status.SourceId)
		if _, seen := byCase[caseId]; !seen {
			order = append(order, caseId)
		}
		byCase[caseId] = append(byCase[caseId], publication)
	}
	if len(order) <= 1 {
		return []ImportResult{r}
	}
	scopes := make([]ImportResult, 0, len(order))
	for _, caseId := range order {
		scope := r
		scope.publications = byCase[caseId]
		scope.analystAssignments = r.assignmentsInCase(r.analystAssignments, caseId)
		scope.importAssignments = r.assignmentsInCase(r.importAssignments, caseId)
		scopes = append(scopes, scope)
	}
	return scopes
}

// HasCase は、その案件を付けた収集元を取り込み結果が持つかを返す。
func (g Graph) HasCase(caseId string) bool {
	for _, carried := range g.caseOfSource {
		if carried == caseId {
			return true
		}
	}
	return false
}

// caseOfRecord は根拠のレコードの位置から、そのレコードの収集元に付けた案件を返す。
// 案件を区別しない取り込みでは空の文字列である。
func (g Graph) caseOfRecord(at int) string {
	return g.caseOfSource[g.records[at].locator.SourceId]
}

// casesOf は根拠のレコードの位置の並びに現れる案件を、最初に現れた順で重複なく返す。
func (g Graph) casesOf(evidence []int) []string {
	var cases []string
	for _, at := range evidence {
		if caseId := g.caseOfRecord(at); !slices.Contains(cases, caseId) {
			cases = append(cases, caseId)
		}
	}
	return cases
}

// evidenceInCase は根拠のレコードの位置の並びから、その案件のレコードだけを並びを保って返す。
func (g Graph) evidenceInCase(evidence []int, caseId string) []int {
	kept := make([]int, 0, len(evidence))
	for _, at := range evidence {
		if g.caseOfRecord(at) == caseId {
			kept = append(kept, at)
		}
	}
	return kept
}

// evidenceByCase は根拠のレコードの件数を案件ごとに数え、案件の識別子の昇順で返す。
// 案件を区別しない取り込みでは nil を返す。
func (g Graph) evidenceByCase(evidence []int) []core.CaseEvidenceCount {
	if len(g.caseOfSource) == 0 {
		return nil
	}
	counts := make(map[string]int64)
	for _, at := range evidence {
		counts[g.caseOfRecord(at)]++
	}
	byCase := make([]core.CaseEvidenceCount, 0, len(counts))
	for _, caseId := range slices.Sorted(maps.Keys(counts)) {
		byCase = append(byCase, core.CaseEvidenceCount{CaseId: caseId, EvidenceCount: counts[caseId]})
	}
	return byCase
}

// assignmentsInCase は、利用者が入力した割当のうち、その案件の収集元だけを指すものを返す。
func (r ImportResult) assignmentsInCase(
	assignments []core.TerminalAssignment, caseId string,
) []core.TerminalAssignment {
	var kept []core.TerminalAssignment
	for _, assignment := range assignments {
		if r.caseOfSource(assignment.SourceId) != caseId {
			continue
		}
		if assignment.AppliesToSourceId != "" && r.caseOfSource(assignment.AppliesToSourceId) != caseId {
			continue
		}
		kept = append(kept, cloneAssignments([]core.TerminalAssignment{assignment})...)
	}
	return kept
}
