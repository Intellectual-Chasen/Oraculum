package pipeline

import (
	"net/netip"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// argumentDerivation は、コマンド行の引数の文字列から指された対象を導いた導き方である。
// 画面が表示名の導き方として利用者に示す。
const argumentDerivation = "コマンド行の引数"

// maxArgumentObjects は、1 本のコマンド行から採る対象の数の上限である。
//
// 既知の制限: 上限を超えた対象を捨てる, スクリプトの本文を持つ復号したコマンド行は本文の中の
// 文字列の path を大量に指しうる。シェルが実行したスクリプトの本文 (process.shell_command) も
// 同じである, 上限で捨てた
// 件数を出す要求が出たとき、件数を数える
const maxArgumentObjects = 16

// uncHostPattern は、UNC の path の host を取り出す。前置は行頭・空白・引用符・`=`・`(`・`>`・
// `,` であり、host は次の `\`・`/`・`@`・空白・引用符・`|<>&;,)` の前までである。
var uncHostPattern = regexp.MustCompile(`(?:^|[\s"'=(>,])\\\\([^\\/@\s"'|<>&;,)]*)`)

// urlHostPattern は、`http://` と `https://` の URL の host を取り出す。userinfo (`user:pass@`) を
// 読み飛ばす。host は `[` と `]` で囲んだ IPv6 のアドレスか、次の `/`・`:`・`?`・`#`・`@`・`\`・
// 空白・引用符・`|<>&;,)` の前までの文字列である。
var urlHostPattern = regexp.MustCompile(
	`(?i)\bhttps?://(?:[^/?#@\\\s"'|<>&;,)]*@)?(\[[0-9a-f:.]+\]|[^\[/:?#@\\\s"'|<>&;,)]+)(?:[/:?#\s"'|<>&;,)\\]|$)`)

// quotedPathPattern は、引用符で囲んだドライブ文字の path を取り出す。
var quotedPathPattern = regexp.MustCompile(`["']([A-Za-z]:\\[^"']*)["']`)

// barePathPattern は、引用符で囲まないドライブ文字の path を取り出す。前置は uncHostPattern と
// 同じであり、path は空白・引用符・`|<>&;,)` の前までである。
var barePathPattern = regexp.MustCompile(`(?:^|[\s"'=(>,])([A-Za-z]:\\[^\s"'|<>&;,)]*)`)

// tokenSeparators は、先頭の文字列の終わりを決める文字である。スクリプトの本文は行ごとに
// コマンドを書くため、改行も文字列を区切る。
const tokenSeparators = " \t\r\n"

// argumentNamedObjects は、コマンド行の引数が指す UNC の host と `http(s)://` の URL の host
// と、ドライブ文字の path を、現れた順に重複を除いて返す。hosts は UNC の host の後ろに URL の
// host を並べる。
//
// **先頭の文字列 (実行ファイル) の path を採らない。** 実行ファイルはプロセスのノードが指す。
// 先頭の文字列の UNC の host は、共有から実行したプロセスの所在として採る。
//
// 引用符で囲んだ path の中に ` -`・` /`・` &` があるときは、`cmd /c "C:\x.exe -a"` の形の
// コマンドとみなし、最初の空白までを path にする。`\` で終わる path (ディレクトリ) と、
// `*` か `?` を含む path を捨てる。UNC の host の `?` と `.` (`\\?\` と `\\.\`) と空の
// host を捨て、末尾の `.` を外す。
//
// Windows の引数の分割の規則で文字列を分けず、コマンド行の全体を走査する。`cmd /c` と
// `powershell -c` は内側のコマンドを 1 つの引数に包み、分けると中の path を見逃す。
//
// 既知の制限: 環境変数・相対 path・`C:/x`・`-OutFile:C:\x`・引用符で囲まない
// `C:\Program Files\…` の空白の後ろを見逃し、コピー元と読むだけのファイルも指す対象に入る,
// 引数の文法はツールごとに異なり、文字列だけからは path の役割を決められない,
// 引数の文法をツールごとに読む要求が出たとき
func argumentNamedObjects(commandLine string) (hosts, paths []string) {
	body := strings.TrimLeft(commandLine, tokenSeparators)
	head := leadingToken(body)
	for _, match := range uncHostPattern.FindAllStringSubmatch(body, -1) {
		host := strings.TrimRight(match[1], ".")
		if host == "" || host == "?" {
			continue
		}
		if !slices.Contains(hosts, host) {
			hosts = append(hosts, host)
		}
	}
	for _, match := range urlHostPattern.FindAllStringSubmatch(body, -1) {
		host := strings.TrimRight(strings.TrimSuffix(strings.TrimPrefix(match[1], "["), "]"), ".")
		if host != "" && !slices.Contains(hosts, host) {
			hosts = append(hosts, host)
		}
	}
	rest := body[len(head):]
	addPath := func(path string) {
		// 引用符の中の path の後ろの空白は、引用符の閉じ方の違いで付いた文字列であり path に含めない。
		path = strings.TrimRight(path, tokenSeparators)
		if path == "" || strings.HasSuffix(path, `\`) || strings.ContainsAny(path, "*?") ||
			slices.Contains(paths, path) {
			return
		}
		paths = append(paths, path)
	}
	// quotedStarts は、引用符で囲んだ path の始まりの位置である。同じ位置から始まる囲まない
	// path は、囲んだ path の空白の前までであるため採らない。
	quotedStarts := make(map[int]struct{})
	for _, match := range quotedPathPattern.FindAllStringSubmatchIndex(rest, -1) {
		quotedStarts[match[2]] = struct{}{}
		path := rest[match[2]:match[3]]
		if strings.Contains(path, " -") || strings.Contains(path, " /") || strings.Contains(path, " &") {
			path, _, _ = strings.Cut(path, " ")
		}
		addPath(path)
	}
	for _, match := range barePathPattern.FindAllStringSubmatchIndex(rest, -1) {
		if _, quoted := quotedStarts[match[2]]; !quoted {
			addPath(rest[match[2]:match[3]])
		}
	}
	return hosts, paths
}

// leadingToken はコマンド行の先頭の文字列を返す。引用符で始まるなら閉じる引用符まで、そうで
// なければ最初の空白までである。
func leadingToken(commandLine string) string {
	if strings.HasPrefix(commandLine, `"`) {
		if end := strings.Index(commandLine[1:], `"`); end >= 0 {
			return commandLine[:end+2]
		}
		return commandLine
	}
	if end := strings.IndexAny(commandLine, tokenSeparators); end >= 0 {
		return commandLine[:end]
	}
	return commandLine
}

// addArgumentNamedObjectEdges は、コマンド行の引数が UNC の path と URL で指すアドレスまたは
// ホスト名とファイルへ、候補の関係を張る。
//
// **起点は、そのコマンド行で生成されたプロセスである。** レコードが生成を記録したプロセスの
// ノードを持たないときは、レコードのノードを起点にする。コマンド行は、レコードのノードの
// 属性から読む。
//
// **ファイルは、そのレコードを置いた端末の範囲で組む。** 端末に置かないレコードからは
// ファイルの候補を作らない。無いノードは参照だけのノードとして足し、そのレコードを根拠に
// する。既にあるノードの状態と根拠と表示名を変えない。
//
// **指すホスト名を、同じ案件の分析者の割当がそのホスト名を記録した端末へも結ぶ。**
// レコードの時刻が割当の期間の中にあり、端末のノードがグラフにあるときだけ結ぶ。関係は
// 割当を持ち (edgeBasis.terminalAssignments)、根拠のレコードは引数のレコードである。
// namedByCase は案件から、その案件の namedAssignmentsOf の割当を探す。
func (g *Graph) addArgumentNamedObjectEdges(namedByCase map[string][]namedAssignment) {
	createdBy := make(map[int]int)
	for index, node := range g.nodes {
		if node.key.Kind != core.NodeKindProcess {
			continue
		}
		for _, recordAt := range node.creationRecords {
			if _, found := createdBy[recordAt]; !found {
				createdBy[recordAt] = index
			}
		}
	}
	for recordAt, record := range g.records {
		if !record.hasRecordNode {
			continue
		}
		source, created := createdBy[recordAt]
		if !created {
			source = record.recordNode
		}
		var terminal *core.NodeKey
		if at, placed := g.nodeAt[record.placedTerminalNodeId]; placed && record.placedTerminalNodeId != "" {
			key := g.nodes[at].key
			terminal = &key
		}
		targets, hostnames := g.argumentTargetsOf(record.recordNode, terminal)
		for _, target := range targets {
			at := g.ensureEdge(core.EdgeKindArgumentNamesObject, core.RelationStateCandidate, source, target)
			g.addEdgeEvidence(at, recordAt)
			if node := &g.nodes[target]; node.observation == core.NodeObservationReferenced &&
				!slices.Contains(node.evidence, recordAt) {
				node.evidence = append(node.evidence, recordAt)
			}
		}
		assignments := namedByCase[g.caseOfRecord(recordAt)]
		if len(assignments) == 0 {
			continue
		}
		for _, hostname := range hostnames {
			g.addArgumentNamedTerminalEdges(source, recordAt, hostname, assignments)
		}
	}
}

// addArgumentNamedTerminalEdges は、引数が指すホスト名 1 つを記録した割当の端末へ、起点
// source から候補の関係を張る。
func (g *Graph) addArgumentNamedTerminalEdges(
	source, recordAt int, hostname string, assignments []namedAssignment,
) {
	record := g.records[recordAt]
	if !record.hasInstant {
		return
	}
	for _, named := range assignments {
		if !named.covers(hostname, record.instant) {
			continue
		}
		terminalAt, present := g.nodeAt[nodeIdOf(named.terminal)]
		if !present {
			continue
		}
		assignment := named.assignment
		at := g.ensureEdge(core.EdgeKindArgumentNamesObject, core.RelationStateCandidate, source, terminalAt)
		g.addEdgeEvidence(at, recordAt)
		held := g.edges[at].ensureBasis()
		if !slices.ContainsFunc(held.terminalAssignments, func(carried core.TerminalAssignment) bool {
			return reflect.DeepEqual(carried, assignment)
		}) {
			held.terminalAssignments = append(held.terminalAssignments, assignment)
		}
	}
}

// argumentSemantics は、指す対象を読むコマンドの文字列の語彙の項目である。シェルが実行した
// コマンドは、スクリプトの本文を 1 件に持つことがあり、改行は空白と同じく文字列を区切る。
var argumentSemantics = []core.SemanticKey{
	core.SemanticKeyProcessCommandLine,
	core.SemanticKeyProcessDecodedCommandLine,
	core.SemanticKeyProcessShellCommand,
}

// argumentTargetsOf は、レコードのノードのコマンド行の属性が指す対象のノードの位置と、
// 指すホスト名 (IP アドレスとして読めない host) を返す。無いノードは参照だけのノードとして
// 足す。
func (g *Graph) argumentTargetsOf(recordNode int, terminal *core.NodeKey) ([]int, []string) {
	var targets []int
	var hostnames []string
	for _, attribute := range g.nodes[recordNode].attributes {
		if !slices.Contains(argumentSemantics, attribute.field.Semantic) {
			continue
		}
		if attribute.field.Text == nil {
			continue
		}
		commandLine, readable := attribute.field.Text.ComparableValue()
		if !readable {
			continue
		}
		hosts, paths := argumentNamedObjects(commandLine)
		var keys []core.NodeKey
		// 端末のノードが無いレコードでは、ループバックとリンクローカルのアドレスの鍵を組まない。
		hostScope := core.RecordScope{Terminal: terminal}
		for _, host := range hosts {
			field := hostField(host)
			if field.Semantic == core.SemanticKeyConnectionDestinationHostname && !slices.Contains(hostnames, host) {
				hostnames = append(hostnames, host)
			}
			keys = append(keys, core.DestinationNodeKeys([]core.RecordField{field}, hostScope)...)
		}
		if terminal != nil {
			scope := core.RecordScope{Terminal: terminal, NamesTerminal: true}
			for _, path := range paths {
				keys = append(keys, core.OperatedFileNodeKeys([]core.RecordField{argumentField(core.SemanticKeyFilePath, path)}, scope)...)
			}
		}
		for _, key := range keys {
			if len(targets) >= maxArgumentObjects {
				return targets, hostnames
			}
			target := g.ensureNode(key, core.NodeObservationReferenced)
			if label, err := core.NewDerivedValue(key.Values[len(key.Values)-1].Value, argumentDerivation); err == nil {
				g.nodes[target].applyLabel(label)
			}
			if !slices.Contains(targets, target) {
				targets = append(targets, target)
			}
		}
	}
	return targets, hostnames
}

// hostField は、UNC の host の文字列を接続先の項目にする。IP アドレスとして読める文字列は
// アドレス、読めない文字列はホスト名の項目である。IPv6 の形で書いた IPv4 は、ドット 10 進の
// 文字列で比べる。それ以外のアドレスは原資料の文字列のまま比べる。
func hostField(host string) core.RecordField {
	address, err := netip.ParseAddr(host)
	if err != nil {
		return argumentField(core.SemanticKeyConnectionDestinationHostname, host)
	}
	if address.Is4In6() {
		value, _ := core.NewNormalizedValue(core.ValueStatePresent, host, address.Unmap().String(), argumentDerivation)
		field, _ := core.NewTextField("argument", core.SemanticKeyConnectionDestinationAddress, value)
		return field
	}
	return argumentField(core.SemanticKeyConnectionDestinationAddress, host)
}

// argumentField は引数から採った文字列を 1 つの項目にする。
func argumentField(semantic core.SemanticKey, text string) core.RecordField {
	// 名前は非空、値は原資料の文字列を持つ present、semantic は語彙の項目であるため検査が失敗しない。
	value, _ := core.NewRawValue(core.ValueStatePresent, text)
	field, _ := core.NewTextField("argument", semantic, value)
	return field
}
