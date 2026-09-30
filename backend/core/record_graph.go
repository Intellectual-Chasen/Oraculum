package core

import (
	"net/netip"
	"regexp"
	"slices"
	"strings"
)

// RecordLink は 1 レコードが作る関係 1 件である。
type RecordLink struct {
	// Kind は関係の種別である。
	Kind EdgeKind
	// Source は関係の起点のノードの識別鍵である。
	Source NodeKey
	// Target は関係の終点のノードの識別鍵である。
	Target NodeKey
}

// ReferencedNode はレコードが別の対象の属性として参照しただけのノードである。
//
// **参照した側のレコードの項目を、そのノードの属性にしない。** 1 レコードの属性の項目は
// レコードが記録した対象を説明する値であり、参照先の対象を説明しない。
type ReferencedNode struct {
	// Key はノードの識別鍵である。
	Key NodeKey
	// LabelSemantic は、参照した側のレコードで参照先の表示名になる語彙の項目である。
	LabelSemantic SemanticKey
}

// RecordNaming は、レコードのノードから記録した対象へ張る、種別を持つ対象を指す関係 1 件である。
// 起点は、この組を使う側が作るレコードのノードである。
type RecordNaming struct {
	// Kind は関係の種別である。
	Kind EdgeKind
	// Target は記録した対象のノードの識別鍵である。
	Target NodeKey
}

// RecordGraph は 1 レコードが指すノードと、レコードが作る関係である。
//
// **入力形式ごとの分岐を持たない。** 材料は語彙の項目 (SemanticKey) だけであり、原資料の
// key の文字列を読まない。
type RecordGraph struct {
	// Nodes はレコードが対象そのものとして指すノードの識別鍵である。同じ鍵を 2 回持たない。
	Nodes []NodeKey
	// ReferencedNodes はレコードが属性として参照するノードである。Nodes と同じ鍵を持たない。
	ReferencedNodes []ReferencedNode
	// Links はレコードが作る関係である。同じ種別と両端の組を 2 回持たない。
	// 両端は Nodes と ReferencedNodes のいずれかにある。
	Links []RecordLink
	// Namings は、役割を付けて記録したアカウントと、メンバーを追加したグループ (groupKeyOf) への
	// 対象を指す関係である。同じ種別と終点の組を
	// 2 回持たない。終点は Nodes と ReferencedNodes のいずれかにある。
	//
	// **終点には EdgeKindRecordNamesObject を張らない。** 同じ対象へ種別の違う関係を
	// 重ねない。1 件のレコードが同じアカウントを 2 つの役割で指すと、2 件を持つ。
	Namings []RecordNaming
}

// recordObjects は 1 レコードが記録した対象を、関係を組む役割ごとに分けた組である。
type recordObjects struct {
	terminal          *NodeKey
	process           *NodeKey
	parentProcess     *NodeKey
	injectionTarget   *NodeKey
	account           *NodeKey
	roleAccounts      []roleAccount
	group             *NodeKey
	files             []NodeKey
	destinationFiles  []NodeKey
	registryValues    []NodeKey
	terminalAddresses []NodeKey
	sourceAddresses   []NodeKey
	destinations      []NodeKey
	carriesHttp       bool
}

// RecordScope は、端末を名乗らないレコードを置く端末の範囲である。
//
// **1 つのログのファイルは 1 台の端末が書いたものとして扱う。** 収集元ごとに、
// その収集元を記録した端末を呼び出し側が与える。
type RecordScope struct {
	// Terminal は、レコードが端末の外部識別子を持たないときに、レコードを置く端末の
	// 識別鍵である。nil はその範囲を持たないことを表す。
	Terminal *NodeKey
	// NamesTerminal が真のとき、端末の範囲で識別する対象を指さないレコードも Terminal を
	// 指す。利用者が端末を指定した収集元で真にする。
	//
	// **偽のときは、端末の範囲で識別する対象を指すレコードだけが端末を指す。**
	// 端末が分からない収集元の全レコードを 1 つのノードへ集めない。
	NamesTerminal bool
	// AccountNamesLocal が真のとき、account.name を端末の範囲で識別する。ログイン名が
	// 収集元を記録した端末のアカウントを指す入力形式で真にする。
	AccountNamesLocal bool
}

// NewRecordGraph は 1 レコードの項目から、レコードが指すノードと作る関係を組む。
//
// 端末の外部識別子を持たないレコードは、端末とプロセスとファイルとレジストリの値の
// ノードを持たない。4 つの識別鍵が terminal.id を含むためである。
// **識別の範囲を確定できない対象を、範囲の無いノードとして作らない。**
func NewRecordGraph(fields []RecordField) RecordGraph {
	return NewRecordGraphInScope(fields, RecordScope{})
}

// NewRecordGraphInScope は NewRecordGraph と同じ組を、scope の端末の範囲で組む。
//
// **レコードが端末の外部識別子を持つときは、scope の端末を使わない。** 収集元が記録した
// 端末を、呼び出し側が与えた範囲で置き換えない。
func NewRecordGraphInScope(fields []RecordField, scope RecordScope) RecordGraph {
	objects := recordObjectsOf(fields, scope)
	graph := RecordGraph{}
	for _, key := range objects.allNodes() {
		graph.addNode(key)
	}
	if objects.parentProcess != nil {
		graph.addReferencedNode(ReferencedNode{
			Key: *objects.parentProcess, LabelSemantic: SemanticKeyParentProcessBinaryPath,
		})
	}
	if objects.injectionTarget != nil {
		graph.addReferencedNode(ReferencedNode{
			Key:           *objects.injectionTarget,
			LabelSemantic: SemanticKeyInjectionTargetProcessBinaryPath,
		})
	}
	for _, role := range objects.roleAccounts {
		graph.addReferencedNode(ReferencedNode{Key: role.key, LabelSemantic: role.label})
		graph.addNaming(RecordNaming{Kind: role.naming, Target: role.key})
	}
	if objects.group != nil {
		graph.addReferencedNode(ReferencedNode{Key: *objects.group, LabelSemantic: SemanticKeyTargetGroupName})
		graph.addNaming(RecordNaming{Kind: EdgeKindRecordNamesObject, Target: *objects.group})
	}
	for _, link := range objects.links() {
		graph.addLink(link)
	}
	return graph
}

// DestinationNodeKeys は 1 レコードが記録した接続先のノードの識別鍵を、走査の順で返す。
// 接続先は IP アドレスとホスト名のノードである。
//
// 関連付けが挙げた候補の関係の起点になるノードを探すために公開している。ノードの識別鍵の
// 組み方を呼び出し側へ写さないよう、NewRecordGraphInScope と同じ経路で組む。
// **ループバックとリンクローカルのアドレスは scope の端末の範囲で組む。** グラフを組んだときと
// 同じ scope を渡さないと、そのアドレスのノードを探せない。
func DestinationNodeKeys(fields []RecordField, scope RecordScope) []NodeKey {
	return recordObjectsOf(fields, scope).destinations
}

// OperatedFileNodeKeys は、1 レコードが file.path で記録したファイルのノードの識別鍵を、
// scope の端末の範囲で組んで返す。file.destination_path のファイルを含まない。
//
// プロセス番号から区間で区切ったプロセスへ、操作したファイルの関係を張る呼び出し側のために
// 公開している。ファイルの鍵の組み方を呼び出し側へ写さないよう、NewRecordGraphInScope と
// 同じ経路で組む。
func OperatedFileNodeKeys(fields []RecordField, scope RecordScope) []NodeKey {
	return recordObjectsOf(fields, scope).files
}

// ExecutableLink は、1 レコードが process.binary_path で記録した実行ファイルのノードから、
// そのレコードのプロセスへの EdgeKindProcessExecutable の関係を組む。ファイルは
// プロセスと同じ端末の範囲に置く。ok が偽になるのは、レコードがプロセスまたは読める
// 実行ファイルの path を持たないときである。
//
// ファイルの鍵は file.path で記録したファイルと同じ形であり、書き込んだファイルと後で
// 起動した実行ファイルが同じノードになる。起動のレコードかを判定するのは呼び出し側である。
func ExecutableLink(fields []RecordField, scope RecordScope) (RecordLink, bool) {
	objects := recordObjectsOf(fields, scope)
	if objects.terminal == nil || objects.process == nil {
		return RecordLink{}, false
	}
	for _, field := range fields {
		if field.Semantic != SemanticKeyProcessBinaryPath || field.Text == nil {
			continue
		}
		value, readable := field.Text.ComparableValue()
		if !readable || value == "" {
			continue
		}
		file := terminalScopedKeys(NodeKindFile, *objects.terminal,
			[]NodeIdentityValue{{Semantic: SemanticKeyFilePath, Value: value}})[0]
		return RecordLink{Kind: EdgeKindProcessExecutable, Source: file, Target: *objects.process}, true
	}
	return RecordLink{}, false
}

// CollectionTerminalNodeKey は、収集の directory の端末のノードの識別鍵を、収集の file の内容の
// 識別から求めた値 digest から組む。ok が偽になるのは、文字列が空のときである。
func CollectionTerminalNodeKey(digest string) (NodeKey, bool) {
	if digest == "" {
		return NodeKey{}, false
	}
	return NodeKey{
		Kind: NodeKindTerminal, Form: NodeKeyFormCollection,
		Values: []NodeIdentityValue{{Value: digest}},
	}, true
}

// RecordingTerminalNodeKey は、収集元を記録した端末のノードの識別鍵を、その収集元の内容の
// 識別から組む。ok が偽になるのは、文字列が空のときである。
//
// **同じ内容の収集元は同じ端末を指す。** 鍵の材料は原資料の文字列だけであり、取り込み
// 1 件を指す sourceId を入れない (NodeKey の doc コメント)。
func RecordingTerminalNodeKey(sourceContentSha256 string) (NodeKey, bool) {
	if sourceContentSha256 == "" {
		return NodeKey{}, false
	}
	return NodeKey{
		Kind: NodeKindTerminal, Form: NodeKeyFormRecordingSource,
		Values: []NodeIdentityValue{{Value: sourceContentSha256}},
	}, true
}

// RecordingHostTerminalNodeKey は、複数台の端末の記録を持ちうる収集元の中で、レコードが
// 名乗ったホスト名 1 つの端末のノードの識別鍵を組む。ok が偽になるのは、どちらかの文字列が
// 空のときである。
//
// 端末の範囲に置く対象の鍵は、端末の鍵の 2 つの値を両方写す (terminalScopedValues)。
func RecordingHostTerminalNodeKey(sourceContentSha256, hostname string) (NodeKey, bool) {
	if sourceContentSha256 == "" || hostname == "" {
		return NodeKey{}, false
	}
	return NodeKey{
		Kind: NodeKindTerminal, Form: NodeKeyFormRecordingSourceHostname,
		Values: []NodeIdentityValue{
			{Value: sourceContentSha256},
			{Semantic: SemanticKeyTerminalHostname, Value: hostname},
		},
	}, true
}

// RecordTerminalNodeKey は、1 レコードを置く端末のノードの識別鍵を返す。
// レコードが端末の外部識別子を持つときはその鍵、持たないときは scope の端末の鍵である。
// ok が偽になるのは、どちらも無いときである。
//
// NewRecordGraphInScope が指す端末と異なり、端末の範囲で識別する対象を指すかを
// 見ない。プロセス番号から区間で区切ったプロセスを組む呼び出し側のために公開している。
func RecordTerminalNodeKey(fields []RecordField, scope RecordScope) (NodeKey, bool) {
	identities := identityValuesOf(fields)
	if key, named := singleKey(NodeKindTerminal, identities[SemanticObjectTerminal]); named {
		return key, true
	}
	if scope.Terminal == nil {
		return NodeKey{}, false
	}
	return *scope.Terminal, true
}

// TerminalNodeKey は端末の外部識別子からノードの識別鍵を組む。
// ok が偽になるのは、文字列が空のときである。
//
// 端末の外部識別子だけを持つ呼び出し側のために公開している。ノードの識別鍵の組み方を
// 呼び出し側へ写さないよう、NewRecordGraph と同じ形の鍵を組む。
func TerminalNodeKey(terminalId string) (NodeKey, bool) {
	if terminalId == "" {
		return NodeKey{}, false
	}
	return NodeKey{
		Kind: NodeKindTerminal, Form: nodeKeyForms[NodeKindTerminal],
		Values: []NodeIdentityValue{
			{Semantic: SemanticKeyTerminalId, Value: terminalId},
		},
	}, true
}

// AccountNodeKey は 1 レコードが記録したアカウントのノードの識別鍵を返す。
// ok が偽になるのは、アカウントの識別鍵に入る値をレコードが 1 つも持たないときである。
//
// 根拠のレコードが記録したアカウントを探す呼び出し側のために公開している。ノードの
// 識別鍵の組み方を呼び出し側へ写さないよう、NewRecordGraph と同じ経路で組む。
//
// account.* のアカウントが無いときは、役割を付けて記録したアカウントを target、subject の
// 順に選ぶ。**target_account.* の項目を 1 つでも持つレコードでは subject へ戻らない。**
// ログオンの失敗のレコードは、Target の SID を持たないときも、ログオンを要求した
// SYSTEM などを Subject に持つ。Subject を返すと、ログオンを試みたアカウントとして別の
// アカウントを示す。
//
// **役割が SID と名前を共に記録したときは、名前のノードの鍵を返す。** SID のノードの表示名は
// 最初に記録された名前であり、改名した後のレコードでは別の名前になる。名前のノードの表示名は
// そのレコードが記録した名前である。SID のノードは、同じレコードのノードから辿れる。
func AccountNodeKey(fields []RecordField, scope RecordScope) (NodeKey, bool) {
	objects := recordObjectsOf(fields, scope)
	if objects.account != nil {
		return *objects.account, true
	}
	for _, role := range accountRoles {
		var chosen *NodeKey
		for _, named := range objects.roleAccounts {
			if named.naming != role.naming {
				continue
			}
			if chosen == nil || named.key.Form == NodeKeyFormAccountDomainName {
				chosen = &named.key
			}
		}
		if chosen != nil {
			return *chosen, true
		}
		if carriesAnyOf(fields, role.sid, role.name, role.domain) {
			return NodeKey{}, false
		}
	}
	return NodeKey{}, false
}

// RoleAccountNodeKeys は、1 レコードが役割を付けて記録したアカウントのノードの識別鍵を、
// target、subject の順で返す。NewRecordGraph と同じ経路で組む。
func RoleAccountNodeKeys(fields []RecordField) []NodeKey {
	accounts := roleAccountsOf(fields)
	keys := make([]NodeKey, 0, len(accounts))
	for _, account := range accounts {
		keys = append(keys, account.key)
	}
	return keys
}

// ProcessNodeKey は 1 レコードが記録したプロセスのノードの識別鍵を返す。
// ok が偽になるのは、端末とプロセスのいずれかの外部識別子を持つ要素がレコードに無いときで
// ある。
func ProcessNodeKey(fields []RecordField) (NodeKey, bool) {
	process := recordObjectsOf(fields, RecordScope{}).process
	if process == nil {
		return NodeKey{}, false
	}
	return *process, true
}

// allNodes はレコードが記録したノードを、走査の順で返す。
func (o recordObjects) allNodes() []NodeKey {
	keys := make([]NodeKey, 0, len(o.files)+len(o.destinationFiles)+len(o.registryValues)+len(o.destinations)+4)
	for _, single := range []*NodeKey{o.terminal, o.process, o.account} {
		if single != nil {
			keys = append(keys, *single)
		}
	}
	keys = append(keys, o.files...)
	keys = append(keys, o.destinationFiles...)
	keys = append(keys, o.registryValues...)
	keys = append(keys, o.terminalAddresses...)
	keys = append(keys, o.sourceAddresses...)
	return append(keys, o.destinations...)
}

// links はレコードが作る関係を組む。
//
// **操作を行った対象を起点にする。** プロセスの欄を持つレコードはプロセスが起点になり、
// プロセスの欄を持たないレコードは端末が起点になる。プロセスの欄を持たないレコードの
// HTTP の要求は、端末の有無に依らず接続元のアドレスが起点になる。
func (o recordObjects) links() []RecordLink {
	links := make([]RecordLink, 0, len(o.files)+len(o.destinationFiles)+len(o.registryValues)+len(o.destinations)+4)
	for _, source := range o.files {
		for _, destination := range o.destinationFiles {
			if sameNodeKey(source, destination) {
				continue
			}
			links = append(links, RecordLink{
				Kind: EdgeKindFileCopy, Source: source, Target: destination,
			})
		}
	}
	if o.terminal != nil {
		links = append(links, pairLinks(EdgeKindTerminalAddress, *o.terminal, o.terminalAddresses)...)
		// **アカウントを記録したレコードの種別で絞らない。** レコードが書いたことをその
		// まま関係にし、操作を行ったアカウントとログインを試みたアカウントの区別は
		// 根拠のイベントの種別に残す。
		if o.account != nil {
			links = append(links, RecordLink{EdgeKindTerminalAccount, *o.terminal, *o.account})
		}
		for _, role := range o.roleAccounts {
			links = append(links, RecordLink{EdgeKindTerminalAccount, *o.terminal, role.key})
		}
		if o.process != nil {
			links = append(links, RecordLink{EdgeKindRanOn, *o.process, *o.terminal})
		}
		// **親を起点にする。** 起動した側から起動された側へ向ける。親の識別子が
		// レコードの記録したプロセスと同じ値である組は関係にしない。同じノードを両端に
		// 置いた関係は、親子の順序を表さない。
		if o.process != nil && o.parentProcess != nil &&
			!sameNodeKey(*o.parentProcess, *o.process) {
			links = append(links,
				RecordLink{EdgeKindProcessParentChild, *o.parentProcess, *o.process})
		}
		// **注入元を起点にする。** レコードが記録したプロセスが注入を行った側であり、
		// 注入先は語彙の injection_target_process.id が指す別のプロセスである。注入先が
		// レコードの記録したプロセスと同じ値である組は関係にしない。
		if o.process != nil && o.injectionTarget != nil &&
			!sameNodeKey(*o.injectionTarget, *o.process) {
			links = append(links,
				RecordLink{EdgeKindProcessInjection, *o.process, *o.injectionTarget})
		}
	}
	if o.process != nil {
		links = append(links, pairLinks(EdgeKindFileOperation, *o.process, o.files)...)
		links = append(links, pairLinks(EdgeKindRegistryOperation, *o.process, o.registryValues)...)
		return append(links, pairLinks(EdgeKindProcessCommunication, *o.process, o.destinations)...)
	}
	// 接続元のアドレスを起点にするのは、HTTP の項目を持つレコードに限る。要求を出した
	// 対象を指す欄が無いまま、2 つのアドレスの間に関係を作らないためである。
	// **端末を持つレコードでも接続元のアドレスを起点にする。** プロセスの欄を持たない
	// レコードの端末は、Proxy のように収集元を記録した端末であり、要求を出したのは接続元の
	// アドレスである。
	if !o.carriesHttp {
		return links
	}
	for _, source := range o.sourceAddresses {
		links = append(links, pairLinks(EdgeKindHttpRequest, source, o.destinations)...)
	}
	return links
}

// pairLinks は 1 つの起点から、終点の集合へ同じ種別の関係を組む。
func pairLinks(kind EdgeKind, source NodeKey, targets []NodeKey) []RecordLink {
	links := make([]RecordLink, 0, len(targets))
	for _, target := range targets {
		links = append(links, RecordLink{Kind: kind, Source: source, Target: target})
	}
	return links
}

// nodeKeyForms はノードの種別ごとの識別鍵の形の名前である。
//
// **表が持つのはノードの種別と鍵の形だけであり、語彙の項目を持たない。**
// 鍵に入る項目は SemanticKey の Role が identity であることで決まる。
//
// 集めた値をどう組むかは 4 つで、組み方を選ぶのは recordObjectsOf である。
// singleKey は対象の値 1 つを鍵にし、terminalScopedKeys は端末の値と対象の値の組を鍵にし、
// perValueKeys は値 1 つにつき 1 つのノードを作り、alternativeKey は 2 つ以上の形から
// レコードが値を持つ最初の形を選ぶ。**アカウントは alternativeKey が形を選ぶため、
// 本表に置かない。**
var nodeKeyForms = map[NodeKind]NodeKeyForm{
	NodeKindTerminal:      NodeKeyFormTerminalId,
	NodeKindProcess:       NodeKeyFormTerminalProcess,
	NodeKindFile:          NodeKeyFormTerminalFilePath,
	NodeKindRegistryValue: NodeKeyFormTerminalRegistryKeyPath,
	NodeKindIp:            NodeKeyFormAddress,
	NodeKindDomain:        NodeKeyFormHostname,
}

// accountKeyForms はアカウントの識別鍵の 2 つの形である。並びが選ぶ順である。
//
// **識別鍵の組み方で語彙の項目を指す箇所の 1 つである** (ほかは accountRoles と
// referencedProcessKey と recordObjectsOf が分ける IP アドレスの項目)。役割が identity である項目を集めただけ
// では、セキュリティ識別子 1 つの形と、ドメインとログイン名の組の形を分けられない。
// 語彙は 2 つの形の選択と、組の側の項目の並びを文で定めており、SemanticKey の Role が
// 表せる範囲の外にある。
//
// **セキュリティ識別子を持つレコードは、セキュリティ識別子で同一性を判定する。**
// 2 つの形の鍵は別の識別鍵である。ドメインとログイン名の組が 1 つの識別子だけと共に
// 記録されたときに、組の鍵を識別子のノードへ寄せるのは、グラフを組む側である。
var accountKeyForms = []struct {
	form      NodeKeyForm
	semantics []SemanticKey
}{
	{NodeKeyFormAccountSid, []SemanticKey{SemanticKeyAccountSid}},
	{NodeKeyFormAccountDomainName,
		[]SemanticKey{SemanticKeyAccountDomain, SemanticKeyAccountName}},
}

// identityValuesOf は 1 レコードの項目から、対象ごとの識別鍵に入る値を集める。
//
// **項目を語彙の役割で選ぶ。** 入力形式ごとの key の文字列も、語彙の項目の一覧も持たない。
// 並びはレコードの項目の並び順で、同じ語彙の項目が同じ値を 2 回記録した組を入れない。
// **語彙の項目をまたいで値をまとめない。** 1 つのアドレスを terminal.ip_address と
// connection.source_address の両方が指すレコードでは、どちらの項目から作る関係も
// 成り立つ。まとめると、後から走査した項目が作る関係が消える。
//
// **値を比べられない項目と、比べる値が空の文字列である項目を飛ばす。** 欄の不在と値の不在を
// 表す状態の項目から鍵を作ると、値の不在どうしが同じノードに集まる。空の文字列の値は
// NodeIdentityValue.Validate を通らず、鍵を組んでも応答に出せないノードになる。
func identityValuesOf(fields []RecordField) map[SemanticObject][]NodeIdentityValue {
	collected := make(map[SemanticObject][]NodeIdentityValue)
	for _, field := range fields {
		if field.Semantic.Role() != SemanticRoleIdentity || field.Text == nil {
			continue
		}
		object := field.Semantic.Object()
		if _, isNode := NodeKindOf(object); !isNode {
			continue
		}
		value, readable := field.Text.ComparableValue()
		named := NodeIdentityValue{Semantic: field.Semantic, Value: value}
		if !readable || value == "" || containsValue(collected[object], named) {
			continue
		}
		collected[object] = append(collected[object], named)
	}
	return collected
}

// recordObjectsOf は 1 レコードの項目を、関係を組む役割ごとの識別鍵へ直す。
//
// **IP アドレスの 3 つの語彙の項目を名前で分ける。** 3 つは同じ 1 つのノードを指すが、
// 関係の起点と終点は項目ごとに異なる。端末に付いたアドレスは端末からの
// terminal_address、接続元のアドレスは要求の起点、接続先のアドレスは要求の終点になる。
// **Role が identity であることは、その値が関係のどちら側に立つかを表さない。**
// 語彙の役割が表せる範囲の外にあるため、ここが項目を名前で探す。
func recordObjectsOf(fields []RecordField, recordScope RecordScope) recordObjects {
	identities := identityValuesOf(fields)
	objects := recordObjects{
		carriesHttp:  carriesObject(fields, SemanticObjectHttp),
		account:      alternativeKey(NodeKindAccount, identities[SemanticObjectAccount]),
		roleAccounts: roleAccountsOf(fields),
	}
	scope, scoped := singleKey(NodeKindTerminal, identities[SemanticObjectTerminal])
	fromRecordScope := false
	if !scoped && recordScope.Terminal != nil {
		scope, scoped, fromRecordScope = *recordScope.Terminal, true, true
	}
	if scoped {
		objects.terminal = &scope
		objects.process = firstKey(terminalScopedKeys(NodeKindProcess, scope,
			identities[SemanticObjectProcess]))
		objects.parentProcess = referencedProcessKey(scope, fields, SemanticKeyParentProcessId)
		objects.injectionTarget = referencedProcessKey(scope, fields,
			SemanticKeyInjectionTargetProcessId)
		objects.files = terminalScopedKeys(NodeKindFile, scope,
			valuesNamedBy(identities[SemanticObjectFile], SemanticKeyFilePath))
		objects.destinationFiles = terminalScopedKeys(NodeKindFile, scope,
			valuesNamedBy(identities[SemanticObjectFile], SemanticKeyFileDestinationPath))
		objects.registryValues = terminalScopedKeys(NodeKindRegistryValue, scope,
			identities[SemanticObjectRegistryValue])
		if objects.account == nil && recordScope.AccountNamesLocal {
			objects.account = terminalAccountKey(scope,
				valuesNamedBy(identities[SemanticObjectAccount], SemanticKeyAccountName))
		}
		if fromRecordScope && !recordScope.NamesTerminal && !objects.namesTerminalScoped(fields) {
			objects = recordObjects{carriesHttp: objects.carriesHttp,
				account:      alternativeKey(NodeKindAccount, identities[SemanticObjectAccount]),
				roleAccounts: objects.roleAccounts}
		}
	}
	// **端末のノードを指さないレコードでも、アドレスは選んだ端末の範囲に置く。** 端末の
	// ノードを外す分岐の後も scope は残る。
	var addressScope *NodeKey
	if scoped {
		addressScope = &scope
	}
	objects.terminalAddresses = addressKeys(addressScope,
		valuesNamedBy(identities[SemanticObjectIp], SemanticKeyTerminalIpAddress))
	objects.sourceAddresses = addressKeys(addressScope,
		valuesNamedBy(identities[SemanticObjectIp], SemanticKeyConnectionSourceAddress))
	objects.destinations = append(
		addressKeys(addressScope,
			valuesNamedBy(identities[SemanticObjectIp], SemanticKeyConnectionDestinationAddress)),
		perValueKeys(NodeKindDomain, identities[SemanticObjectDomain])...)
	objects.group = groupKeyOf(fields, addressScope)
	return objects
}

// builtinGroupSidPrefix は、端末ごとに同じ値を持つ builtin のグループの SID の接頭辞である。
const builtinGroupSidPrefix = "S-1-5-32-"

// groupKeyOf は、レコードがメンバーを追加したグループ (target_group) のノードの識別鍵を返す。
//
// **グループのノードはアカウントの種類である。** ドメインのグループは SID (S-1-5-21-…) の鍵、
// builtin のグループ (SID が S-1-5-32-… か、ドメインが Builtin) は記録した端末の範囲の名前の
// 鍵にする。scope が nil のとき builtin のグループの鍵を組まない。SID の欄を持たないレコードの
// ほかのグループは、ドメインと名前の組の鍵にする。ほかの SID のグループはノードにしない。
//
// 既知の制限: グループをアカウントと別の種類のノードにしない。グループとアカウントは種類で見分け
// られず、表示名とレコードの種別で見分ける, グループのノードを使う調査の実例が 1 件しか無い,
// グループとアカウントを分けて数える要求が出たとき、ノードの種類を足す
func groupKeyOf(fields []RecordField, scope *NodeKey) *NodeKey {
	if key, named := roleSidKey(fields, SemanticKeyTargetGroupSid); named {
		return &key
	}
	sid, hasSid := firstComparable(fields, SemanticKeyTargetGroupSid)
	domain, _ := firstComparable(fields, SemanticKeyTargetGroupDomain)
	if strings.HasPrefix(sid, builtinGroupSidPrefix) || strings.EqualFold(domain, "Builtin") {
		name, named := firstComparable(fields, SemanticKeyTargetGroupName)
		if scope == nil || !named {
			return nil
		}
		return terminalAccountKey(*scope, []NodeIdentityValue{{Semantic: SemanticKeyAccountName, Value: name}})
	}
	if hasSid {
		return nil
	}
	if key, named := roleDomainNameKey(fields, SemanticKeyTargetGroupDomain, SemanticKeyTargetGroupName); named {
		return &key
	}
	return nil
}

// addressKeys は IP アドレスの値 1 つにつき 1 つのノードの識別鍵を組む。
//
// **ループバックとリンクローカルのアドレスは、端末の鍵の値とアドレスの組を鍵にする**
// (NodeKeyFormTerminalAddress)。scope が nil のときは、そのアドレスの鍵を組まない。
// 識別の範囲を確定できない対象を、範囲の無いノードとして作らない。
// ほかのアドレスは perValueKeys と同じ鍵である。
func addressKeys(scope *NodeKey, values []NodeIdentityValue) []NodeKey {
	keys := make([]NodeKey, 0, len(values))
	for _, value := range values {
		if !isTerminalLocalAddress(value.Value) {
			keys = append(keys, perValueKeys(NodeKindIp, []NodeIdentityValue{value})...)
			continue
		}
		if scope == nil {
			continue
		}
		value.Semantic = ""
		keys = append(keys, NodeKey{
			Kind: NodeKindIp, Form: NodeKeyFormTerminalAddress,
			Values: terminalScopedValues(*scope, value),
		})
	}
	return keys
}

// isTerminalLocalAddress は、文字列がループバックかリンクローカルのアドレスであるかを返す。
// アドレスとして読めない文字列は偽である。
func isTerminalLocalAddress(text string) bool {
	address, err := netip.ParseAddr(text)
	if err != nil {
		return false
	}
	address = address.Unmap()
	return address.IsLoopback() || address.IsLinkLocalUnicast()
}

// namesTerminalScoped は、端末の範囲で識別する対象をレコードが指すかを返す。
//
// **プロセス番号も数える。** 番号から区間で区切ったプロセスは、このレコードの組とは別の段階が
// 端末の範囲で組む。
func (o recordObjects) namesTerminalScoped(fields []RecordField) bool {
	if o.process != nil || o.parentProcess != nil || o.injectionTarget != nil ||
		len(o.files) > 0 || len(o.destinationFiles) > 0 || len(o.registryValues) > 0 {
		return true
	}
	if o.account != nil && o.account.Form == NodeKeyFormTerminalAccountName {
		return true
	}
	for _, field := range fields {
		if field.Semantic != SemanticKeyProcessPid || field.Text == nil {
			continue
		}
		if _, readable := field.Text.ComparableValue(); readable {
			return true
		}
	}
	return false
}

// terminalAccountKey は端末の鍵の値とログイン名の組をアカウントの識別鍵にする。
// nil を返すのは、ログイン名を 1 つも持たないときである。
func terminalAccountKey(scope NodeKey, names []NodeIdentityValue) *NodeKey {
	if len(names) == 0 {
		return nil
	}
	return &NodeKey{
		Kind: NodeKindAccount, Form: NodeKeyFormTerminalAccountName,
		Values: terminalScopedValues(scope, names[0]),
	}
}

// singleKey は対象の識別鍵に入る最初の値 1 つを鍵にする。
//
// **入力の集合と記憶領域を共有しない。** 識別鍵はノードが保持し続ける値であり、
// 部分 slice のまま持つと入力の集合の寿命に縛られる。
func singleKey(kind NodeKind, values []NodeIdentityValue) (NodeKey, bool) {
	if len(values) == 0 {
		return NodeKey{}, false
	}
	return NodeKey{
		Kind: kind, Form: nodeKeyForms[kind], Values: []NodeIdentityValue{values[0]},
	}, true
}

// terminalScopedKeys は端末の範囲に置く対象の識別鍵を、値の並び順で組む。
func terminalScopedKeys(
	kind NodeKind, scope NodeKey, values []NodeIdentityValue,
) []NodeKey {
	fixesSemantic := identitySemanticCount(SemanticObject(kind)) == 1
	keys := make([]NodeKey, 0, len(values))
	for _, value := range values {
		if !fixesSemantic {
			value.Semantic = ""
		}
		if kind == NodeKindFile {
			value.Value = FilePathKeyValue(value.Value)
		}
		keys = append(keys, NodeKey{
			Kind: kind, Form: nodeKeyForms[kind],
			Values: terminalScopedValues(scope, value),
		})
	}
	return keys
}

// windowsPathPattern は、ドライブ文字か `\` で始まる Windows の path である。
var windowsPathPattern = regexp.MustCompile(`^(?:[A-Za-z]:\\|\\)`)

// FilePathKeyValue は、ファイルの path をファイルのノードの識別鍵に入れる値にする。
//
// **Windows の path は小文字にそろえる。** Windows のファイルシステムは大文字と小文字を区別せず、
// 同じファイルを収集元ごとに別の大文字と小文字で記録する (小文字で書く記録と大文字で書く記録)。
// ほかの path は区別する。表示名と属性は原資料の文字列のまま持つ。
//
// ponytail: strings.ToLower は Unicode の小文字であり、Windows の大文字の表と一致しない文字がある。
// 一致しない文字を含む path が同じノードにならない実例が出たとき、表を持つ。
func FilePathKeyValue(path string) string {
	if windowsPathPattern.MatchString(path) {
		return strings.ToLower(path)
	}
	return path
}

// perValueKeys は値 1 つにつき 1 つのノードの識別鍵を組む。
//
// **識別鍵に入る語彙の項目が 2 つ以上ある対象では、値を記録した項目を鍵に残さない。**
// IP アドレスは terminal.ip_address と connection.source_address と
// connection.destination_address のいずれが指しても同じ 1 つのノードを指す。
// どの項目が記録したかは根拠のレコードが持つ。
func perValueKeys(kind NodeKind, values []NodeIdentityValue) []NodeKey {
	fixesSemantic := identitySemanticCount(SemanticObject(kind)) == 1
	keys := make([]NodeKey, 0, len(values))
	for _, value := range values {
		if !fixesSemantic {
			value.Semantic = ""
		}
		keys = append(keys, NodeKey{
			Kind: kind, Form: nodeKeyForms[kind], Values: []NodeIdentityValue{value},
		})
	}
	return keys
}

// referencedProcessKey は、レコードが属性として参照した別のプロセスのノードの識別鍵を組む。
// semantic はその識別子を持つ語彙の項目で、parent_process.id と
// injection_target_process.id が該当する。
// nil を返すのは、その項目を持つ要素が無いときと、値を比べられないときである。
//
// **識別鍵の組み方で語彙の項目を指す箇所の 1 つである** (ほかは accountKeyForms と
// accountRoles と recordObjectsOf が分ける IP アドレスの項目)。2 つの項目の役割は
// attribute で、値が指すのは別のプロセスである。役割が identity である項目を集めた
// 結果には入らないため、ここが項目を語彙の項目で探す。
//
// **鍵の項目には process.id を置く。** 同じ 1 つのプロセスを、そのプロセスの起動の
// レコードは process.id で、子の起動のレコードは parent_process.id で、注入の
// レコードは injection_target_process.id で指す。記録した側の語彙の項目を鍵に残すと、
// 同じノードの識別鍵の項目が、先に走査したレコードによって変わる。
func referencedProcessKey(scope NodeKey, fields []RecordField, semantic SemanticKey) *NodeKey {
	for _, field := range fields {
		if field.Semantic != semantic || field.Text == nil {
			continue
		}
		value, readable := field.Text.ComparableValue()
		if !readable {
			continue
		}
		return &NodeKey{
			Kind: NodeKindProcess, Form: nodeKeyForms[NodeKindProcess],
			Values: terminalScopedValues(scope,
				NodeIdentityValue{Semantic: SemanticKeyProcessId, Value: value}),
		}
	}
	return nil
}

// roleAccount は、レコードが役割を付けて記録したアカウント 1 つである。
type roleAccount struct {
	key    NodeKey
	naming EdgeKind
	label  SemanticKey
}

// accountRoles は、役割を付けて記録したアカウントの項目と、レコードから張る関係の種別である。
// 並びは AccountNodeKey が選ぶ順である。
//
// **識別鍵の組み方で語彙の項目を指す箇所の 1 つである。** 役割の項目は attribute であり、
// 役割が identity である項目を集めた結果には入らない。
var accountRoles = []struct {
	sid, name, domain SemanticKey
	naming            EdgeKind
}{
	{SemanticKeyTargetAccountSid, SemanticKeyTargetAccountName, SemanticKeyTargetAccountDomain,
		EdgeKindRecordTargetAccount},
	{SemanticKeySubjectAccountSid, SemanticKeySubjectAccountName, SemanticKeySubjectAccountDomain,
		EdgeKindRecordSubjectAccount},
}

// accountSidPattern は、アカウントのノードを作るセキュリティ識別子の形である。
//
// 既知の制限: 発行元の識別子が 21 のセキュリティ識別子だけからノードを作る。発行元の
// 識別子が 21 のセキュリティ識別子は、ドメインまたは端末ごとに異なる値を含み、端末を
// またいで同じ値が別のアカウントを指さない。NULL SID と SYSTEM などの well-known な
// セキュリティ識別子は端末ごとに同じ値を持ち、Entra ID のアカウントは別の発行元を持つ。
// どちらもノードを作らず、名前はレコードの属性に残る, 範囲の外の値がどれだけの調査に
// 要るかは入力の側に示す値が無く測れない, well-known なセキュリティ識別子または Entra ID の
// アカウントのノードを要求されたとき、端末の範囲の鍵を足す
var accountSidPattern = regexp.MustCompile(`^S-1-5-21(-[0-9]+)+$`)

// roleAccountsOf は、レコードが役割を付けて記録したアカウントを accountRoles の順で返す。
//
// **鍵の項目には account.sid を置く。** 同じアカウントを、あるレコードは account.sid で、
// 別のレコードは subject_account.sid または target_account.sid で指す。記録した側の
// 語彙の項目を鍵に残すと、同じアカウントが役割ごとに別のノードになる。
//
// **役割のセキュリティ識別子の欄を持たないレコードは、ドメインとログイン名の組を鍵にする**
// (NodeKeyFormAccountDomainName、鍵の項目は account.domain と account.name)。識別子を名前へ
// 直して書き出した記録と、識別子の欄を持たない事象が該当する。識別子が NULL SID だけのレコード
// (存在しない名前へのログオンの失敗) も名前を鍵にし、試行された名前のノードとエッジを作る。
// well-known な識別子を持つレコードは名前へ戻らない。
//
// **識別子のノードを作る役割は、ドメインとログイン名の組のノードも識別子の後ろに並べる。**
// 識別子と名前が別のアカウントを指す記録 (偽造したチケットのログオン) でも、記録した名前の
// ノードから辿れる。2 つのノードが同じアカウントかは EdgeKindAccountIdentityMatch が候補にする。
func roleAccountsOf(fields []RecordField) []roleAccount {
	var accounts []roleAccount
	for _, role := range accountRoles {
		key, bySid := roleSidKey(fields, role.sid)
		if bySid {
			accounts = append(accounts, roleAccount{key: key, naming: role.naming, label: role.name})
		}
		if !bySid && carriesAnyOf(fields, role.sid) && !carriesOnlyNullSid(fields, role.sid) {
			continue
		}
		if key, named := roleDomainNameKey(fields, role.domain, role.name); named {
			accounts = append(accounts, roleAccount{key: key, naming: role.naming, label: role.name})
		}
	}
	return accounts
}

// nullSid は、アカウントを指さないことを表すセキュリティ識別子である。
const nullSid = "S-1-0-0"

// carriesOnlyNullSid は、語彙の項目 sid の読める値がすべて NULL SID であるとき真を返す。
func carriesOnlyNullSid(fields []RecordField, sid SemanticKey) bool {
	found := false
	for _, field := range fields {
		if field.Semantic != sid || field.Text == nil {
			continue
		}
		value, readable := field.Text.ComparableValue()
		if !readable || value != nullSid {
			return false
		}
		found = true
	}
	return found
}

// roleSidKey は、語彙の項目 sid の最初のノードを作る識別子を鍵にする。
func roleSidKey(fields []RecordField, sid SemanticKey) (NodeKey, bool) {
	for _, field := range fields {
		if field.Semantic != sid || field.Text == nil {
			continue
		}
		value, readable := field.Text.ComparableValue()
		if !readable {
			continue
		}
		if key, ok := AccountSidNodeKey(value); ok {
			return key, true
		}
	}
	return NodeKey{}, false
}

// AccountSidNodeKey は、セキュリティ識別子 sid のアカウントのノードの識別鍵を返す。ok が
// 偽になるのは、sid がノードを作る形 (accountSidPattern) でないときである。
func AccountSidNodeKey(sid string) (NodeKey, bool) {
	if !accountSidPattern.MatchString(sid) {
		return NodeKey{}, false
	}
	return NodeKey{
		Kind: NodeKindAccount, Form: NodeKeyFormAccountSid,
		Values: []NodeIdentityValue{{Semantic: SemanticKeyAccountSid, Value: sid}},
	}, true
}

// roleDomainNameKey は、語彙の項目 domain と name の最初の値の組を鍵にする。ok が偽になるのは、
// どちらかの値を比べられないときである。
func roleDomainNameKey(fields []RecordField, domain, name SemanticKey) (NodeKey, bool) {
	domainValue, hasDomain := firstComparable(fields, domain)
	nameValue, hasName := firstComparable(fields, name)
	// ドメインの欄を持たず、名前を `ドメイン\名前` の形で書いたレコードは、名前を 2 つに分ける。
	if !hasDomain && hasName && !carriesAnyOf(fields, domain) {
		if before, after, found := strings.Cut(nameValue, `\`); found && before != "" && after != "" {
			domainValue, nameValue, hasDomain = before, after, true
		}
	}
	if !hasDomain || !hasName {
		return NodeKey{}, false
	}
	return NodeKey{
		Kind: NodeKindAccount, Form: NodeKeyFormAccountDomainName,
		Values: []NodeIdentityValue{
			{Semantic: SemanticKeyAccountDomain, Value: domainValue},
			{Semantic: SemanticKeyAccountName, Value: nameValue},
		},
	}, true
}

// firstComparable は、語彙の項目を持つ最初の項目の、空でない比べる値を返す。
func firstComparable(fields []RecordField, semantic SemanticKey) (string, bool) {
	for _, field := range fields {
		if field.Semantic != semantic || field.Text == nil {
			continue
		}
		value, readable := field.Text.ComparableValue()
		return value, readable && value != ""
	}
	return "", false
}

// carriesAnyOf はレコードが、semantics のいずれかの項目を持つかを返す。
func carriesAnyOf(fields []RecordField, semantics ...SemanticKey) bool {
	for _, field := range fields {
		if slices.Contains(semantics, field.Semantic) {
			return true
		}
	}
	return false
}

// alternativeKey は 2 つ以上の形から、レコードが値を持つ最初の形の識別鍵を組む。
func alternativeKey(kind NodeKind, values []NodeIdentityValue) *NodeKey {
	for _, candidate := range accountKeyForms {
		selected := make([]NodeIdentityValue, 0, len(candidate.semantics))
		for _, semantic := range candidate.semantics {
			named := valuesNamedBy(values, semantic)
			if len(named) == 0 {
				break
			}
			selected = append(selected, named[0])
		}
		if len(selected) != len(candidate.semantics) {
			continue
		}
		return &NodeKey{Kind: kind, Form: candidate.form, Values: selected}
	}
	return nil
}

// valuesNamedBy は語彙の項目が記録した値を、並び順で返す。
func valuesNamedBy(values []NodeIdentityValue, semantic SemanticKey) []NodeIdentityValue {
	named := make([]NodeIdentityValue, 0, len(values))
	for _, value := range values {
		if value.Semantic == semantic {
			named = append(named, value)
		}
	}
	return named
}

// firstKey は識別鍵の並びの先頭を返す。要素数 0 では nil を返す。
func firstKey(keys []NodeKey) *NodeKey {
	if len(keys) == 0 {
		return nil
	}
	return &keys[0]
}

// carriesObject はレコードが、その対象に意味を与える項目を持つかを返す。
func carriesObject(fields []RecordField, object SemanticObject) bool {
	for _, field := range fields {
		if field.Semantic.Object() == object {
			return true
		}
	}
	return false
}

func containsValue(values []NodeIdentityValue, named NodeIdentityValue) bool {
	for _, element := range values {
		if element == named {
			return true
		}
	}
	return false
}

// addNode は同じ識別鍵を 2 回持たないようにノードを足す。
func (g *RecordGraph) addNode(key NodeKey) {
	for _, existing := range g.Nodes {
		if sameNodeKey(existing, key) {
			return
		}
	}
	g.Nodes = append(g.Nodes, key)
}

// addReferencedNode は参照だけのノードを足す。対象そのものとして記録したノードと同じ鍵の
// 参照を足さない。同じレコードが記録し、かつ参照した対象は記録した側が表す。
func (g *RecordGraph) addReferencedNode(node ReferencedNode) {
	for _, existing := range g.Nodes {
		if sameNodeKey(existing, node.Key) {
			return
		}
	}
	for _, existing := range g.ReferencedNodes {
		if sameNodeKey(existing.Key, node.Key) {
			return
		}
	}
	g.ReferencedNodes = append(g.ReferencedNodes, node)
}

// addNaming は同じ種別と終点の組を 2 回持たないように対象を指す関係を足す。
func (g *RecordGraph) addNaming(naming RecordNaming) {
	for _, existing := range g.Namings {
		if existing.Kind == naming.Kind && sameNodeKey(existing.Target, naming.Target) {
			return
		}
	}
	g.Namings = append(g.Namings, naming)
}

// NamesWithKind は、key の対象を種別を持つ対象を指す関係で記録したかを返す。
func (g RecordGraph) NamesWithKind(key NodeKey) bool {
	for _, naming := range g.Namings {
		if sameNodeKey(naming.Target, key) {
			return true
		}
	}
	return false
}

// addLink は同じ種別と両端の組を 2 回持たないように関係を足す。
func (g *RecordGraph) addLink(link RecordLink) {
	for _, existing := range g.Links {
		if existing.Kind == link.Kind && sameNodeKey(existing.Source, link.Source) &&
			sameNodeKey(existing.Target, link.Target) {
			return
		}
	}
	g.Links = append(g.Links, link)
}

func sameNodeKey(left, right NodeKey) bool {
	if left.Kind != right.Kind || left.Form != right.Form ||
		len(left.Values) != len(right.Values) {
		return false
	}
	for index, value := range left.Values {
		if right.Values[index] != value {
			return false
		}
	}
	return true
}
