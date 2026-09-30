package pipeline

import (
	"slices"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// AccountHistoryCategory は、アカウントの管理の事象の区分である。
type AccountHistoryCategory string

// AccountHistoryCategory の値。
const (
	AccountHistoryCreated          AccountHistoryCategory = "created"
	AccountHistoryEnabled          AccountHistoryCategory = "enabled"
	AccountHistoryPasswordSet      AccountHistoryCategory = "password_set"
	AccountHistoryChanged          AccountHistoryCategory = "changed"
	AccountHistoryGroupMemberAdded AccountHistoryCategory = "group_member_added"
	AccountHistoryDeleted          AccountHistoryCategory = "deleted"
)

// accountHistoryProvider は、アカウントの管理の事象を書くプロバイダの名前である。
const accountHistoryProvider = "Microsoft-Windows-Security-Auditing"

// accountHistoryCategories は、Security の監査のイベント ID から、Target のアカウントの管理の
// 事象の区分を引く表である。グループへの追加の Target は追加されたメンバーである。
var accountHistoryCategories = map[string]AccountHistoryCategory{
	"4720": AccountHistoryCreated,
	"4722": AccountHistoryEnabled,
	"4724": AccountHistoryPasswordSet,
	"4738": AccountHistoryChanged,
	"4728": AccountHistoryGroupMemberAdded,
	"4732": AccountHistoryGroupMemberAdded,
	"4756": AccountHistoryGroupMemberAdded,
	"4726": AccountHistoryDeleted,
}

// AccountHistoryEntry は、アカウントの管理の事象を記録したレコード 1 件である。
type AccountHistoryEntry struct {
	Category AccountHistoryCategory
	// Record は事象を記録したレコードの位置である。
	Record core.RecordLocator
	// EventTime はレコードの時刻である。時刻を持たないレコードでは nil である。
	EventTime *core.Timestamp
	// GroupName は、グループへの追加でメンバーを追加したグループの名前である。グループの名前を
	// 記録しないレコードと、ほかの区分では nil である。
	GroupName *core.RawAndNormalized
}

// AccountHistory は、アカウントのノード nodeId を Target として記録した、アカウントの管理の
// 事象のレコードを返す。並びは時刻の順で、時刻を持たないレコードは後ろに並ぶ。nodeId の
// ノードが無いときと、アカウントのノードでないときは nil を返す。
func (g Graph) AccountHistory(nodeId string) []AccountHistoryEntry {
	index, found := g.nodeAt[nodeId]
	if !found || g.nodes[index].key.Kind != core.NodeKindAccount {
		return nil
	}
	// レコードのノードからアカウントへの関係は、レコード 1 件につき 1 本である。
	var ats []int
	for _, edge := range g.adjacency[index].incoming {
		if g.edges[edge].kind != core.EdgeKindRecordTargetAccount {
			continue
		}
		for _, at := range g.edges[edge].evidence {
			if _, managed := g.accountHistoryCategoryOf(at); managed {
				ats = append(ats, at)
			}
		}
	}
	slices.SortStableFunc(ats, func(left, right int) int {
		l, r := g.records[left], g.records[right]
		switch {
		case l.hasInstant && r.hasInstant:
			return l.instant.Compare(r.instant)
		case l.hasInstant:
			return -1
		case r.hasInstant:
			return 1
		}
		return left - right
	})
	history := make([]AccountHistoryEntry, 0, len(ats))
	for _, at := range ats {
		category, _ := g.accountHistoryCategoryOf(at)
		record := g.records[at]
		entry := AccountHistoryEntry{Category: category, Record: record.locator, EventTime: record.eventTime}
		if category == AccountHistoryGroupMemberAdded {
			entry.GroupName = g.recordGroupName(at)
		}
		history = append(history, entry)
	}
	return history
}

// accountHistoryCategoryOf は、レコード at が記録したアカウントの管理の事象の区分を返す。ok が
// 偽になるのは、レコードがアカウントの管理の事象でないときである。
func (g Graph) accountHistoryCategoryOf(at int) (AccountHistoryCategory, bool) {
	record := g.records[at]
	if !record.windowsEventKind || record.eventCategory != accountHistoryProvider {
		return "", false
	}
	category, known := accountHistoryCategories[record.eventAction]
	return category, known
}

// recordGroupName は、レコード at が記録したグループの名前を返す。記録しないときは nil である。
func (g Graph) recordGroupName(at int) *core.RawAndNormalized {
	if !g.records[at].hasRecordNode {
		return nil
	}
	for _, attribute := range g.nodes[g.records[at].recordNode].attributes {
		if attribute.field.Semantic == core.SemanticKeyTargetGroupName && attribute.field.Text != nil {
			name := cloneRawAndNormalized(*attribute.field.Text)
			return &name
		}
	}
	return nil
}
