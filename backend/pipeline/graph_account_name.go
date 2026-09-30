package pipeline

import (
	"slices"
	"strings"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// accountNameOfNode は、アカウントのノード 1 つの、同じアカウントとしてまとめる鍵か、鍵を
// 持たない理由である (core.GraphNode.AccountName と AccountNameWithheld)。
type accountNameOfNode struct {
	key      *core.AccountNameKey
	withheld core.AccountNameWithheldReason
}

// sidRecordedName は、SID と共に記録した名前 1 組である。domains は、レコードが記録した
// ドメインを端末の別名に広げ、小文字にして昇順に並べた集合である。
type sidRecordedName struct {
	caseId  string
	domains string
	name    string
}

// indexAccountNames は、アカウントのノードごとに、同じアカウントとしてまとめる鍵を決める。
// 結果は g.accountNames に置く。
//
// **SID と共に記録した名前は、グラフ全体の全レコードから 1 回だけ集める。** 時刻を持たない
// レコードと、別の案件のレコードも数える。名前はドメインを domainsOf で端末の別名に広げた集合で
// 比べ、集合が同じなら 1 組と数える。1 組だけの SID に鍵を付ける。
//
// 名前のノードは、自身の識別鍵のドメインを、根拠のレコードの収集元ごとに domainsOf で広げた
// 集合の和から鍵を作る。
//
// ドメインの短い名前と FQDN、UPN の形の名前は mergedAccountName で揃える。鍵のドメインは、
// 揃えた集合のうち最小の文字列である。**揃える規則は鍵だけに当て、同じアカウントの候補の
// エッジ (addAccountIdentityEdges) の規則を変えない。**
//
// **案件をまたいでまとめない。** 鍵は案件を含み、根拠のレコードが 2 つ以上の案件にある
// ノードには鍵を付けない。
func (g *Graph) indexAccountNames(
	result ImportResult, domainsOf func(record RecordEntry, domain string) []string,
) {
	sidNames := make(map[int]map[sidRecordedName]struct{})
	for _, publication := range result.publications {
		if publication.status.PublicationState == core.PublicationStateWithheld {
			continue
		}
		for _, record := range publication.records {
			at, indexed := g.recordAtLocator(record.Locator)
			if !indexed {
				continue
			}
			fields := graphFieldsOf(record)
			for _, items := range accountRoleItems {
				sidNode, domain, name, found := g.accountSidObservationOf(fields, items)
				if !found {
					continue
				}
				domains, login := mergedAccountName(domainsOf(record, domain), name)
				if sidNames[sidNode] == nil {
					sidNames[sidNode] = make(map[sidRecordedName]struct{})
				}
				sidNames[sidNode][sidRecordedName{
					caseId: g.caseOfRecord(at), domains: strings.Join(domains, "\x00"), name: login,
				}] = struct{}{}
			}
		}
	}
	g.accountNames = make(map[int]accountNameOfNode)
	counts := make(map[core.AccountNameKey]int64)
	for index, node := range g.nodes {
		if node.key.Kind != core.NodeKindAccount {
			continue
		}
		var named accountNameOfNode
		switch node.key.Form {
		case core.NodeKeyFormAccountSid:
			named = sidAccountName(sidNames[index])
		case core.NodeKeyFormAccountDomainName:
			named = g.domainNameAccountName(node, domainsOf)
		default:
			continue
		}
		if named.key != nil {
			counts[*named.key]++
		}
		g.accountNames[index] = named
	}
	for index, named := range g.accountNames {
		if named.key != nil {
			named.key.NodeCount = counts[*named.key]
			g.accountNames[index] = named
		}
	}
}

// sidAccountName は、SID と共に記録した名前の集合から、SID のノードの鍵か理由を決める。
func sidAccountName(names map[sidRecordedName]struct{}) accountNameOfNode {
	if len(names) == 0 {
		return accountNameOfNode{withheld: core.AccountNameWithheldNoNameRecorded}
	}
	cases := make(map[string]struct{})
	var only sidRecordedName
	for recorded := range names {
		cases[recorded.caseId] = struct{}{}
		only = recorded
	}
	if len(cases) > 1 {
		return accountNameOfNode{withheld: core.AccountNameWithheldMultipleCases}
	}
	if len(names) > 1 {
		return accountNameOfNode{withheld: core.AccountNameWithheldMultipleNames}
	}
	least, _, _ := strings.Cut(only.domains, "\x00")
	return accountNameOfNode{key: &core.AccountNameKey{CaseId: only.caseId, Name: accountNameKey(least, only.name)}}
}

// domainNameAccountName は、名前のノードの鍵か理由を決める。domainsOf は indexAccountNames の
// 引数である。
func (g *Graph) domainNameAccountName(
	node graphNode, domainsOf func(record RecordEntry, domain string) []string,
) accountNameOfNode {
	cases := g.casesOf(node.evidence)
	if len(cases) > 1 {
		return accountNameOfNode{withheld: core.AccountNameWithheldMultipleCases}
	}
	caseId := ""
	if len(cases) == 1 {
		caseId = cases[0]
	}
	domain, name := node.key.Values[0].Value, node.key.Values[1].Value
	domains := []string{domain}
	// 広げる名前は収集元だけで決まるため、収集元ごとに 1 回だけ広げる。
	expanded := make(map[string]struct{})
	for _, at := range node.evidence {
		locator := g.records[at].locator
		if _, seen := expanded[locator.SourceId]; seen {
			continue
		}
		expanded[locator.SourceId] = struct{}{}
		domains = append(domains, domainsOf(RecordEntry{Locator: locator}, domain)...)
	}
	merged, login := mergedAccountName(domains, name)
	return accountNameOfNode{key: &core.AccountNameKey{CaseId: caseId, Name: accountNameKey(merged[0], login)}}
}

// mergedAccountName は、アカウントを記録したドメインの並びとログイン名を、まとめる鍵を比べる形に
// 揃える。返すドメインは、重複を除いて昇順に並べた集合であり、要素数は 1 以上である。
//
//   - ドメインは、小文字にした最初のラベル (最初の `.` の前) に揃える。ドメインの短い名前と、その
//     名前で始まる FQDN (`corp` と `CORP.LOCAL`) が同じ値になる。
//   - ログイン名は小文字にする。UPN の形 (`user@CORP.LOCAL`) の名前は、`@` の後が揃えたドメインの
//     どれかと等しいときだけ、`@` の前をログイン名として読む。
//
// ponytail: 最初のラベルで揃えるため、最初のラベルが同じ 2 つの FQDN (`corp.local` と `corp.example`)
// も同じドメインになる。区別が要る資料が現れたら、短い名前と FQDN の組だけを結ぶ規則に替える。
func mergedAccountName(domains []string, name string) ([]string, string) {
	merged := make([]string, 0, len(domains))
	for _, domain := range domains {
		merged = append(merged, firstDomainLabel(domain))
	}
	slices.Sort(merged)
	merged = slices.Compact(merged)
	login := strings.ToLower(name)
	if at := strings.LastIndex(login, "@"); at > 0 && slices.Contains(merged, firstDomainLabel(login[at+1:])) {
		login = login[:at]
	}
	return merged, login
}

// firstDomainLabel は、ドメインの名前を小文字にした最初のラベルを返す。
func firstDomainLabel(domain string) string {
	label, _, _ := strings.Cut(strings.ToLower(domain), ".")
	return label
}

// accountNameFields は、ノードの位置から、応答に載せるまとめる鍵と理由を返す。
func (g Graph) accountNameFields(index int) (*core.AccountNameKey, core.AccountNameWithheldReason) {
	named, found := g.accountNames[index]
	if !found {
		return nil, ""
	}
	if named.key == nil {
		return nil, named.withheld
	}
	key := *named.key
	return &key, ""
}
