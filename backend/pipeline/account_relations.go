package pipeline

import (
	"cmp"
	"slices"
	"strings"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// AccountRelationKey は起点と相手を同じレコードが名指した役割の組である。
// 相手は表示名でなくノード ID で区別する。
type AccountRelationKey struct {
	CounterpartId string        `json:"counterpartId"`
	OriginRole    core.EdgeKind `json:"originRole"`
	OtherRole     core.EdgeKind `json:"otherRole"`
}

// AccountRelationEvent は候補に含まれる事象の種別と件数である。
type AccountRelationEvent struct {
	Category string `json:"category"`
	Action   string `json:"action"`
	Count    int    `json:"count"`
}

// AccountRelationSource は候補に含まれる収集元と件数である。
type AccountRelationSource struct {
	SourceId string `json:"sourceId"`
	Count    int    `json:"count"`
}

// AccountRelation は同じ役割の組で結ばれたレコードをまとめた候補である。
type AccountRelation struct {
	AccountRelationKey
	Counterpart      core.GraphNode          `json:"counterpart"`
	RecordCount      int                     `json:"recordCount"`
	FirstTime        *core.Timestamp         `json:"firstTime,omitempty"`
	LastTime         *core.Timestamp         `json:"lastTime,omitempty"`
	UnknownTimeCount int                     `json:"unknownTimeCount"`
	Events           []AccountRelationEvent  `json:"events"`
	Sources          []AccountRelationSource `json:"sources"`
	SigmaRulePaths   []string                `json:"sigmaRulePaths"`
}

// AccountRelationRecord は開いた候補の 1 レコードと、それを名指した 2 本のエッジである。
type AccountRelationRecord struct {
	NodeId         string             `json:"nodeId"`
	OriginEdgeId   string             `json:"originEdgeId"`
	OtherEdgeId    string             `json:"otherEdgeId"`
	Summary        core.RecordSummary `json:"summary"`
	SigmaRulePaths []string           `json:"sigmaRulePaths"`
}

// AccountRelations は起点アカウントと相手候補、選択候補の根拠を持つ。
type AccountRelations struct {
	Origin                    core.GraphNode          `json:"origin"`
	Groups                    []AccountRelation       `json:"groups"`
	Records                   []AccountRelationRecord `json:"records,omitempty"`
	SelectedRecordCount       int                     `json:"selectedRecordCount"`
	NextOffset                *int                    `json:"nextOffset,omitempty"`
	PeriodUnjudgedRecordCount int                     `json:"periodUnjudgedRecordCount"`
}

const accountRelationPageSize = 50

type relationGroup struct {
	value       AccountRelation
	records     map[int]AccountRelationRecord
	first, last int
}

// AccountRelations はレコードを介して 2 ホップの相手アカウントを集計する。
// Sigma の一致は補足情報であり、候補の採否には使わない。
func (g Graph) AccountRelations(originId string, filter RecordFilter, roles []core.EdgeKind, selected *AccountRelationKey, offset int, sigma SigmaEvaluation) AccountRelations {
	filter.Validate()
	originAt := g.nodeAt[originId]
	response := AccountRelations{Origin: g.graphNode(originAt), Groups: []AccountRelation{}}
	allowed := func(kind core.EdgeKind) bool { return len(roles) == 0 || slices.Contains(roles, kind) }
	sigmaByRef := make(map[string][]string)
	for _, match := range sigma.Matches {
		ref := sigmaRecordKey(match.Record)
		if !slices.Contains(sigmaByRef[ref], match.RulePath) {
			sigmaByRef[ref] = append(sigmaByRef[ref], match.RulePath)
		}
	}
	groups := make(map[AccountRelationKey]*relationGroup)
	periodless := filter.withoutPeriod()
	unknown := make(map[int]bool)
	for _, originEdgeAt := range g.adjacency[originAt].incoming {
		originEdge := g.edges[originEdgeAt]
		if !accountRole(originEdge.kind) || !allowed(originEdge.kind) {
			continue
		}
		for _, recordAt := range originEdge.evidence {
			if !g.recordMatches(recordAt, periodless) {
				continue
			}
			record := g.records[recordAt]
			if !record.hasRecordNode {
				continue
			}
			passes := g.recordMatches(recordAt, filter)
			for _, otherEdgeAt := range g.adjacency[record.recordNode].outgoing {
				otherEdge := g.edges[otherEdgeAt]
				if !accountRole(otherEdge.kind) || !allowed(otherEdge.kind) || otherEdge.target == originAt ||
					g.nodes[otherEdge.target].key.Kind != core.NodeKindAccount || !slices.Contains(otherEdge.evidence, recordAt) {
					continue
				}
				if !passes {
					if (filter.TimeFrom != nil || filter.TimeTo != nil) && !record.hasInstant {
						unknown[recordAt] = true
					}
					continue
				}
				key := AccountRelationKey{g.nodes[otherEdge.target].id, originEdge.kind, otherEdge.kind}
				group := groups[key]
				if group == nil {
					group = &relationGroup{value: AccountRelation{AccountRelationKey: key, Counterpart: g.graphNode(otherEdge.target), Events: []AccountRelationEvent{}, Sources: []AccountRelationSource{}, SigmaRulePaths: []string{}}, records: make(map[int]AccountRelationRecord), first: -1, last: -1}
					groups[key] = group
				}
				if _, found := group.records[recordAt]; found {
					continue
				}
				paths := append([]string{}, sigmaByRef[sigmaRecordKey(record.locator)]...)
				group.records[recordAt] = AccountRelationRecord{NodeId: g.nodes[record.recordNode].id, OriginEdgeId: originEdge.id, OtherEdgeId: otherEdge.id, Summary: *g.recordSummaryOf(record.recordNode), SigmaRulePaths: paths}
				group.value.RecordCount++
				if !record.hasInstant {
					group.value.UnknownTimeCount++
				} else {
					if group.first < 0 || record.instant.Before(g.records[group.first].instant) {
						group.first = recordAt
					}
					if group.last < 0 || record.instant.After(g.records[group.last].instant) {
						group.last = recordAt
					}
				}
				addRelationEvent(&group.value.Events, record.eventCategory, record.eventAction)
				addRelationSource(&group.value.Sources, record.locator.SourceId)
				for _, path := range sigmaByRef[sigmaRecordKey(record.locator)] {
					if !slices.Contains(group.value.SigmaRulePaths, path) {
						group.value.SigmaRulePaths = append(group.value.SigmaRulePaths, path)
					}
				}
			}
		}
	}
	response.PeriodUnjudgedRecordCount = len(unknown)
	keys := make([]AccountRelationKey, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, func(a, b AccountRelationKey) int {
		if v := cmp.Compare(a.CounterpartId, b.CounterpartId); v != 0 {
			return v
		}
		if v := cmp.Compare(a.OriginRole, b.OriginRole); v != 0 {
			return v
		}
		return cmp.Compare(a.OtherRole, b.OtherRole)
	})
	for _, key := range keys {
		group := groups[key]
		if group.first >= 0 {
			group.value.FirstTime = cloneTimestampPointer(g.records[group.first].eventTime)
		}
		if group.last >= 0 {
			group.value.LastTime = cloneTimestampPointer(g.records[group.last].eventTime)
		}
		slices.SortFunc(group.value.Events, func(a, b AccountRelationEvent) int {
			if v := cmp.Compare(a.Category, b.Category); v != 0 {
				return v
			}
			return cmp.Compare(a.Action, b.Action)
		})
		slices.SortFunc(group.value.Sources, func(a, b AccountRelationSource) int { return cmp.Compare(a.SourceId, b.SourceId) })
		slices.Sort(group.value.SigmaRulePaths)
		response.Groups = append(response.Groups, group.value)
		if selected != nil && key == *selected {
			response.SelectedRecordCount = len(group.records)
			ats := make([]int, 0, len(group.records))
			for at := range group.records {
				ats = append(ats, at)
			}
			slices.SortFunc(ats, func(a, b int) int { return cmp.Compare(a, b) })
			if offset > len(ats) {
				offset = len(ats)
			}
			end := min(offset+accountRelationPageSize, len(ats))
			for _, at := range ats[offset:end] {
				response.Records = append(response.Records, group.records[at])
			}
			if end < len(ats) {
				response.NextOffset = &end
			}
		}
	}
	return response
}

// sigmaRecordKey は同じ RecordRawTextRef を持つ別収集元を混同しない識別子である。
func sigmaRecordKey(locator core.RecordLocator) string {
	return strings.Join([]string{locator.SourceId, locator.SourceContentSha256, locator.RecordRawTextRef}, "\x00")
}

func accountRole(kind core.EdgeKind) bool {
	return kind == core.EdgeKindRecordSubjectAccount || kind == core.EdgeKindRecordTargetAccount || kind == core.EdgeKindRecordNamesObject
}

func addRelationEvent(events *[]AccountRelationEvent, category, action string) {
	for i := range *events {
		if (*events)[i].Category == category && (*events)[i].Action == action {
			(*events)[i].Count++
			return
		}
	}
	*events = append(*events, AccountRelationEvent{category, action, 1})
}

func addRelationSource(sources *[]AccountRelationSource, id string) {
	for i := range *sources {
		if (*sources)[i].SourceId == id {
			(*sources)[i].Count++
			return
		}
	}
	*sources = append(*sources, AccountRelationSource{id, 1})
}
