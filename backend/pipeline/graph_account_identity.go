package pipeline

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// accountSidObservation は、セキュリティ識別子とドメインとログイン名を共に記録したレコード
// 1 件である。
type accountSidObservation struct {
	instant time.Time
	sidNode int
	at      int
}

// accountRoleItems は、役割を付けて記録したアカウントの 3 つの語彙の項目である。
var accountRoleItems = [][3]core.SemanticKey{
	{core.SemanticKeyTargetAccountSid, core.SemanticKeyTargetAccountDomain, core.SemanticKeyTargetAccountName},
	{core.SemanticKeySubjectAccountSid, core.SemanticKeySubjectAccountDomain, core.SemanticKeySubjectAccountName},
}

// accountNameKey は、ドメインとログイン名を大文字と小文字を区別せずに比べる鍵である。
func accountNameKey(domain, name string) string {
	return strings.ToLower(domain) + `\` + strings.ToLower(name)
}

// sameDomainNames は、record が domain と共に記録したアカウントを、同じアカウントとして指す
// ドメインの名前を返す。
//
// record の収集元が収集の端末に置かれ、domain がその端末の今または過去の名前であるときは、
// domain と端末の名前の集合の全ての名前を返す。ローカルアカウントは改名の前のホスト名も
// ドメインに書く。ほかのときは domain 自身だけを返す。
func (t sourceTerminals) sameDomainNames(record RecordEntry, domain string) []string {
	collection := t[record.Locator.SourceId].collection
	if collection == nil || !collection.names[shortHostname(domain)] {
		return []string{domain}
	}
	names := []string{domain}
	for name := range collection.names {
		names = append(names, name)
	}
	return names
}

// addAccountIdentityEdges は、ドメインとログイン名で識別したアカウントのノードから、同じ
// アカウントの候補であるセキュリティ識別子のノードへのエッジを足す。result は案件 1 つである。
//
// **名前を記録したレコードの時刻の直前と直後に、同じドメインとログイン名を識別子と共に記録した
// レコードが同じ識別子を指すときだけ結ぶ。** 片側だけに記録があるときは、その識別子に結ぶ。
// 直前と直後が別の識別子を指すレコードは、アカウントの削除と作り直しの間にあり、結ばない。
// 名前は大文字と小文字を区別せずに比べ、ドメインの短い名前と FQDN は別の名前である。
//
// **名前のノードを識別子のノードへまとめない。** 1 つの識別子を改名の前後の名前と共に記録した
// アカウントでは、名前ごとのノードが記録した名前を表示名に持ち、候補のエッジの根拠がどのレコードの
// 名前と識別子を結んだかを持つ。
//
// domainsOf は、レコードが識別子と共に記録したドメインを、同じアカウントを指すドメインの名前の
// 並びへ広げる。ホスト名を変えた端末のローカルアカウントは、変える前のホスト名もドメインに書く。
func (g *Graph) addAccountIdentityEdges(
	result ImportResult, domainsOf func(record RecordEntry, domain string) []string,
) {
	observed, named := g.accountIdentityObservations(result, domainsOf)
	evidenceOf := make(map[int]map[int]struct{})
	paired := make(map[[2]int]struct{})
	for _, record := range named {
		sidNode, basis, found := sidAtInstant(observed[record.key], g.records[record.at].instant)
		if !found {
			continue
		}
		edge := g.ensureEdge(core.EdgeKindAccountIdentityMatch, core.RelationStateCandidate, record.node, sidNode)
		// 1 本のエッジが名前のレコードを多数持つため、根拠の重複を表で除く。addEdgeEvidence の
		// 線形の探索を件数の 2 乗の回数で繰り返さない。
		if evidenceOf[edge] == nil {
			// 別の案件の走査が同じエッジに足した根拠も数える。
			evidenceOf[edge] = make(map[int]struct{})
			for _, at := range g.edges[edge].evidence {
				evidenceOf[edge][at] = struct{}{}
			}
		}
		// 2 つの役割が同じ名前を記録したレコードは named に 2 回入る。組は 1 回だけ足す。
		// 根拠の表では判定しない。名前のレコードが別の名前のレコードの SID の記録として先に
		// 根拠へ入っていることがある。
		pairedKey := [2]int{edge, record.at}
		if _, repeated := paired[pairedKey]; !repeated {
			paired[pairedKey] = struct{}{}
			for _, sidAt := range basis {
				g.addRecordPair(edge, pairRuleAccountIdentity, record.at, sidAt)
			}
		}
		for _, at := range append(basis, record.at) {
			if _, held := evidenceOf[edge][at]; !held {
				evidenceOf[edge][at] = struct{}{}
				g.edges[edge].evidence = append(g.edges[edge].evidence, at)
			}
		}
	}
}

// accountNamedRecord は、名前だけでアカウントを記録したレコード 1 件と、その名前のノードと、
// ドメインとログイン名の鍵 (accountNameKey) である。
type accountNamedRecord struct {
	node, at int
	key      string
}

// accountIdentityObservations は、result のレコードから、ドメインとログイン名の鍵ごとに識別子と
// 共に記録した観測を時刻の順に並べた表と、名前だけでアカウントを記録したレコードを返す。
// domainsOf は addAccountIdentityEdges の引数である。
func (g *Graph) accountIdentityObservations(
	result ImportResult, domainsOf func(record RecordEntry, domain string) []string,
) (map[string][]accountSidObservation, []accountNamedRecord) {
	observed := make(map[string][]accountSidObservation)
	var named []accountNamedRecord
	for _, publication := range result.publications {
		if publication.status.PublicationState == core.PublicationStateWithheld {
			continue
		}
		for _, record := range publication.records {
			at, indexed := g.recordAtLocator(record.Locator)
			if !indexed || !g.records[at].hasInstant {
				continue
			}
			fields := graphFieldsOf(record)
			for _, items := range accountRoleItems {
				sidNode, domain, name, found := g.accountSidObservationOf(fields, items)
				if !found {
					continue
				}
				keys := make(map[string]struct{})
				for _, named := range domainsOf(record, domain) {
					keys[accountNameKey(named, name)] = struct{}{}
				}
				for key := range keys {
					observed[key] = append(observed[key], accountSidObservation{g.records[at].instant, sidNode, at})
				}
			}
			for _, key := range core.RoleAccountNodeKeys(rolesWithoutSid(fields)) {
				if key.Form != core.NodeKeyFormAccountDomainName {
					continue
				}
				if node, present := g.nodeAt[nodeIdOf(key)]; present {
					named = append(named, accountNamedRecord{node, at, accountNameKey(key.Values[0].Value, key.Values[1].Value)})
				}
			}
		}
	}
	for _, observations := range observed {
		slices.SortStableFunc(observations, func(left, right accountSidObservation) int {
			return left.instant.Compare(right.instant)
		})
	}
	return observed, named
}

// rolesWithoutSid は、識別子の欄を持つ役割の 3 つの項目を除いた項目を返す。識別子の欄を持つ
// 役割の名前のノードは、識別子と並べて記録しただけであり、名前だけを記録したレコードに数えない。
func rolesWithoutSid(fields []core.RecordField) []core.RecordField {
	var dropped []core.SemanticKey
	for _, items := range accountRoleItems {
		if len(fieldsWithSemantic(fields, items[0])) > 0 {
			dropped = append(dropped, items[:]...)
		}
	}
	if len(dropped) == 0 {
		return fields
	}
	return slices.DeleteFunc(slices.Clone(fields), func(field core.RecordField) bool {
		return slices.Contains(dropped, field.Semantic)
	})
}

// accountSidObservationOf は、1 つの役割の識別子のノードの位置と、ドメインとログイン名を
// 返す。ok が偽になるのは、3 つの値のいずれかを比べられないときと、識別子のノードが無いとき
// である。
func (g *Graph) accountSidObservationOf(
	fields []core.RecordField, items [3]core.SemanticKey,
) (node int, domain, name string, ok bool) {
	sid, hasSid := comparableOfSemantic(fields, items[0])
	domain, hasDomain := comparableOfSemantic(fields, items[1])
	name, hasName := comparableOfSemantic(fields, items[2])
	if !hasSid || !hasDomain || !hasName || domain == "" || name == "" {
		return 0, "", "", false
	}
	node, present := g.nodeAt[nodeIdOf(core.NodeKey{
		Kind: core.NodeKindAccount, Form: core.NodeKeyFormAccountSid,
		Values: []core.NodeIdentityValue{{Semantic: core.SemanticKeyAccountSid, Value: sid}},
	})]
	return node, domain, name, present
}

// sidAtInstant は、時刻の順に並んだ observations から、instant の時点の識別子のノードと、
// 決めた根拠のレコードの位置を返す。直前 (同じ時刻を含む) と直後の記録が別の識別子を指す
// ときと、記録が無いときは ok が偽である。
func sidAtInstant(observations []accountSidObservation, instant time.Time) (int, []int, bool) {
	after, _ := slices.BinarySearchFunc(observations, instant, func(observation accountSidObservation, target time.Time) int {
		return cmp.Compare(observation.instant.UnixNano(), target.UnixNano()+1)
	})
	var basis []int
	sidNode := -1
	if after > 0 {
		sidNode = observations[after-1].sidNode
		basis = append(basis, observations[after-1].at)
	}
	if after < len(observations) {
		if sidNode >= 0 && observations[after].sidNode != sidNode {
			return 0, nil, false
		}
		sidNode = observations[after].sidNode
		basis = append(basis, observations[after].at)
	}
	return sidNode, basis, sidNode >= 0
}
