package pipeline

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

func TestAccountRelationsGroupsTicketRequestsAndPagesEvidence(t *testing.T) {
	var document strings.Builder
	document.WriteString("<Events>\n")
	for number := 1; number <= 55; number++ {
		document.WriteString(securityEventXML(strconv.Itoa(number), "4769", logonHostA, "2001-02-03T04:05:10.000Z",
			"TargetUserSid", roleSubjectSid, "TargetUserName", "requester", "TargetDomainName", "EXAMPLE",
			"ServiceSid", roleTargetSid, "ServiceName", "service"))
	}
	document.WriteString("</Events>\n")
	graph := NewGraph(windowsEventImportResult(t, document.String()), AllMatchConditions())
	service := accountNodeOfSid(graph, roleTargetSid)
	if service < 0 {
		t.Fatal("service account missing")
	}
	roles := []core.EdgeKind{core.EdgeKindRecordSubjectAccount, core.EdgeKindRecordTargetAccount}
	first := graph.AccountRelations(graph.nodes[service].id, RecordFilter{}, roles, nil, 0, SigmaEvaluation{})
	group := slices.IndexFunc(first.Groups, func(group AccountRelation) bool {
		return group.OriginRole == core.EdgeKindRecordTargetAccount && group.OtherRole == core.EdgeKindRecordSubjectAccount &&
			group.Counterpart.KeyForm == core.NodeKeyFormAccountDomainName &&
			len(group.Counterpart.Identity) > 0 && group.Counterpart.Identity[len(group.Counterpart.Identity)-1].Value == "requester" && group.RecordCount == 55
	})
	if group < 0 {
		t.Fatalf("requester with 55 ticket requests missing from %+v", first.Groups)
	}
	if len(first.Records) != 0 {
		t.Errorf("initial response has %d individual records", len(first.Records))
	}
	key := first.Groups[group].AccountRelationKey
	page := graph.AccountRelations(graph.nodes[service].id, RecordFilter{}, roles, &key, 0, SigmaEvaluation{})
	if page.SelectedRecordCount != 55 || len(page.Records) != 50 || page.NextOffset == nil || *page.NextOffset != 50 {
		t.Fatalf("first page = count %d, records %d, next %v", page.SelectedRecordCount, len(page.Records), page.NextOffset)
	}
	second := graph.AccountRelations(graph.nodes[service].id, RecordFilter{}, roles, &key, 50, SigmaEvaluation{})
	if len(second.Records) != 5 || second.NextOffset != nil {
		t.Errorf("second page = %d records, next %v", len(second.Records), second.NextOffset)
	}
	refs := map[string]bool{}
	for _, record := range append(page.Records, second.Records...) {
		if refs[record.Summary.RecordRef.RecordRawTextRef] {
			t.Errorf("duplicate record %s", record.Summary.RecordRef.RecordRawTextRef)
		}
		refs[record.Summary.RecordRef.RecordRawTextRef] = true
		if record.Summary.EventAction != "4769" || record.OriginEdgeId == "" || record.OtherEdgeId == "" {
			t.Errorf("incomplete ticket evidence: %+v", record)
		}
	}
	filtered := graph.AccountRelations(graph.nodes[service].id, RecordFilter{EventAction: "4624"}, roles, nil, 0, SigmaEvaluation{})
	if len(filtered.Groups) != 0 {
		t.Errorf("unrelated Event ID returned %d groups", len(filtered.Groups))
	}
	withoutRoles := graph.AccountRelations(graph.nodes[service].id, RecordFilter{}, nil, nil, 0, SigmaEvaluation{})
	if len(withoutRoles.Groups) == 0 {
		t.Error("omitted role selection returned no groups")
	}
}

func TestSigmaRecordKeySeparatesSourcesWithTheSameRawReference(t *testing.T) {
	left := core.RecordLocator{SourceId: "source-a", SourceContentSha256: "hash-a", RecordRawTextRef: "same"}
	right := core.RecordLocator{SourceId: "source-b", SourceContentSha256: "hash-b", RecordRawTextRef: "same"}
	if sigmaRecordKey(left) == sigmaRecordKey(right) {
		t.Fatal("Sigma record key merged distinct sources")
	}
}
