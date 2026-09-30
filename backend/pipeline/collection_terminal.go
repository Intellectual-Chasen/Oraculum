package pipeline

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// collectionLabelDerivation は、収集の端末の表示名の導出の説明である。
const collectionLabelDerivation = "収集の registry が記録した端末の名前。最も新しい時刻の値を使う"

// collectionUnknownDerivation は、名前の分からない収集の端末の表示名の導出の説明である。
const collectionUnknownDerivation = "収集の directory の path。収集の registry が端末の名前を記録していない"

// collectionTerminal は、端末 1 台から集めた収集の directory 1 つの端末である。
//
// **収集の file は 1 台の端末のものとして扱う。** registry と Prefetch の収集元はこの端末に置く。
// イベントログのレコードは、名乗ったホスト名がこの端末の名前の集合にあるときだけこの端末に置き、
// ほかのホスト名は収集ごとのホスト名の端末に置く。転送されたイベントの別の端末を収集の端末に
// まとめない。
type collectionTerminal struct {
	key core.NodeKey
	// digest は収集の file の内容の識別を並べて求めた値である (collectionDigest)。
	digest string
	// names は端末の名前の集合である。鍵は shortHostname の値である。
	names map[string]bool
	// label は registry が記録した名前のうち、最も新しい時刻の名前である。registry が名前を
	// 記録していない収集では nil である。
	label *core.RawAndNormalized
	// unknownLabel は、registry が名前を記録していない収集の端末の表示名である。
	unknownLabel core.RawAndNormalized
	// history は名前ごとの記録である。名前が最初に現れた順に並ぶ。
	history []TerminalName
}

// TerminalName は、収集の端末が名乗った名前 1 つと、その名前を記録したレコードである。
type TerminalName struct {
	// Name は名前を記録したレコードの値である。shortHostname が同じ値が 2 つ以上あるときは、
	// 先に現れた値である。
	Name string
	// First と Last は、名前を記録したレコードの時刻のうち最も早い値と最も遅い値である。時点が
	// 定まる時刻を持つレコードが無い名前では nil である。
	First, Last *core.Timestamp
	// RecordRefs は名前を記録したレコードの位置である。
	RecordRefs []core.RecordLocator
}

// shortHostname は、名前の先頭の DNS の label を小文字にした値である。registry のコンピューター名
// (NetBIOS の名前) とイベントの Computer (DNS の名前) を同じ端末の名前として比べる。
func shortHostname(name string) string {
	label, _, _ := strings.Cut(name, ".")
	return strings.ToLower(label)
}

// collectionDigest は、収集の file の内容の識別を辞書順に並べてつないだ文字列の sha256 である。
//
// **内容だけから求める。** 同じ内容の収集を 2 回取り込んでも同じ端末を指す。
func collectionDigest(contentSha256s []string) string {
	sorted := slices.Sorted(slices.Values(contentSha256s))
	sum := sha256.Sum256([]byte(strings.Join(sorted, "\n")))
	return hex.EncodeToString(sum[:])
}

// namedRecord は、レコード 1 件が記録した端末自身の名前 1 つと、そのレコードである。
type namedRecord struct {
	naming TerminalNaming
	record RecordEntry
}

// collectionTerminalsOf は、取り込み結果の収集の directory ごとに、その収集の端末を組む。鍵は
// 収集の directory の path である。
//
// 名前の集合の起点は registry が記録した名前である (空の Previous を持つ TerminalNaming)。改名の
// 記録は、前後のどちらかの名前が集合にあるとき、もう一方を集合へ足す。足した名前から先の改名も
// 同じ規則で辿る。
func collectionTerminalsOf(result ImportResult) map[string]*collectionTerminal {
	members := make(map[string][]SourcePublication)
	var paths []string
	for _, publication := range result.publications {
		path := result.identities[publication.status.SourceId].CollectionPath
		if path == "" {
			continue
		}
		if _, seen := members[path]; !seen {
			paths = append(paths, path)
		}
		members[path] = append(members[path], publication)
	}
	terminals := make(map[string]*collectionTerminal, len(paths))
	for _, path := range paths {
		var shas []string
		var named []namedRecord
		for _, publication := range members[path] {
			shas = append(shas, result.identities[publication.status.SourceId].ContentSha256)
			for _, naming := range publication.terminalNamings {
				named = append(named, namedRecord{naming: naming.naming, record: publication.records[naming.record]})
			}
		}
		digest := collectionDigest(shas)
		key, built := core.CollectionTerminalNodeKey(digest)
		if !built {
			continue
		}
		unknown, err := core.NewDerivedValue(path+" を記録した端末 (名前不明)", collectionUnknownDerivation)
		if err != nil {
			continue
		}
		terminal := &collectionTerminal{
			key: key, digest: digest, names: collectionNamesOf(named), unknownLabel: unknown,
		}
		terminal.history, terminal.label = collectionHistoryOf(named, terminal.names)
		terminals[path] = terminal
	}
	return terminals
}

// collectionSourceTerminalOf は、収集の directory から取り出した収集元 1 件を置く端末を組む。
// ok が偽になるのは、入力形式が 1 台の端末が書く形式でもホスト名ごとに分ける形式でもないときで
// ある。
//
// 1 台の端末が書く形式 (registry、Prefetch) は収集の端末に置く。ホスト名ごとに分ける形式
// (イベントログ) は、ホスト名の端末の鍵に収集の digest を使い、名前の集合のホスト名だけを
// 収集の端末に置く (sourceTerminals.recordingScope)。別の収集の同じホスト名を 1 つにまとめない。
func collectionSourceTerminalOf(
	collection *collectionTerminal, parser ParserIdentity, hostNamed []namedAssignment,
) (sourceTerminal, bool) {
	scope := core.RecordScope{AccountNamesLocal: parser.AccountNamesLocalToTerminal}
	switch {
	case parser.RecordedByOneTerminal:
		key := collection.key
		scope.Terminal = &key
		label := collection.unknownLabel
		return sourceTerminal{scope: scope, unknownLabel: &label, collection: collection}, true
	case parser.RecordingTerminalPerHostname:
		return sourceTerminal{
			scope: scope, hostScopedContentSha256: collection.digest, namedAssignments: hostNamed,
			collection: collection,
		}, true
	default:
		return sourceTerminal{}, false
	}
}

// collectionNamesOf は、名前を記録したレコードから収集の端末の名前の集合を求める
// (collectionTerminalsOf)。
func collectionNamesOf(named []namedRecord) map[string]bool {
	names := make(map[string]bool)
	for _, entry := range named {
		if entry.naming.Previous == "" {
			names[shortHostname(entry.naming.Name)] = true
		}
	}
	for changed := len(names) > 0; changed; {
		changed = false
		for _, entry := range named {
			if entry.naming.Previous == "" {
				continue
			}
			previous, current := shortHostname(entry.naming.Previous), shortHostname(entry.naming.Name)
			if names[previous] != names[current] {
				names[previous], names[current], changed = true, true, true
			}
		}
	}
	return names
}

// collectionHistoryOf は、集合の名前を記録したレコードから名前ごとの記録を組み、registry が
// 記録した名前のうち最も新しい時刻の名前を表示名にして返す。表示名の候補が無いときは nil である。
func collectionHistoryOf(named []namedRecord, names map[string]bool) ([]TerminalName, *core.RawAndNormalized) {
	var history []TerminalName
	at := make(map[string]int)
	var current string
	var currentAt time.Time
	currentSeen := false
	for _, entry := range named {
		for _, name := range []string{entry.naming.Previous, entry.naming.Name} {
			short := shortHostname(name)
			if name == "" || !names[short] {
				continue
			}
			index, seen := at[short]
			if !seen {
				index = len(history)
				at[short] = index
				history = append(history, TerminalName{Name: name})
			}
			history[index].observe(entry.record)
		}
		if entry.naming.Previous != "" {
			continue
		}
		instant, readable := time.Time{}, false
		if entry.record.ObservedAt != nil {
			instant, readable = entry.record.ObservedAt.Instant()
		}
		if !currentSeen || readable && instant.After(currentAt) {
			current, currentAt, currentSeen = entry.naming.Name, instant, true
		}
	}
	if !currentSeen {
		return history, nil
	}
	label, err := core.NewDerivedValue(current, collectionLabelDerivation)
	if err != nil {
		return history, nil
	}
	return history, &label
}

// observe は、名前を記録したレコード 1 件の位置と時刻を足す。
func (n *TerminalName) observe(record RecordEntry) {
	locator := cloneLocator(record.Locator)
	if !slices.ContainsFunc(n.RecordRefs, func(held core.RecordLocator) bool {
		return recordKeyOf(held) == recordKeyOf(locator)
	}) {
		n.RecordRefs = append(n.RecordRefs, locator)
	}
	if record.ObservedAt == nil {
		return
	}
	at, readable := record.ObservedAt.Instant()
	if !readable {
		return
	}
	if first, _ := instantOf(n.First); n.First == nil || at.Before(first) {
		n.First = cloneTimestampPointer(record.ObservedAt)
	}
	if last, _ := instantOf(n.Last); n.Last == nil || at.After(last) {
		n.Last = cloneTimestampPointer(record.ObservedAt)
	}
}

// instantOf は時刻の時点を返す。nil と時点の定まらない時刻では偽である。
func instantOf(timestamp *core.Timestamp) (time.Time, bool) {
	if timestamp == nil {
		return time.Time{}, false
	}
	return timestamp.Instant()
}

// applyCollectionLabel は、収集の端末のノードに名前ごとの記録を持たせ、registry が記録した名前を
// 表示名にする。表示名を置いたときに真を返す。
//
// **registry の名前はレコードが名乗ったホスト名の表示名を置き換える。** イベントログのレコードは
// 改名の前の名前も名乗るため、先に置かれた表示名は今の端末の名前を表さない。
func (g *Graph) applyCollectionLabel(collection *collectionTerminal) bool {
	if collection == nil {
		return false
	}
	id := nodeIdOf(collection.key)
	at, present := g.nodeAt[id]
	if !present {
		return false
	}
	if len(collection.history) > 0 {
		if g.terminalNames == nil {
			g.terminalNames = make(map[string][]TerminalName)
		}
		g.terminalNames[id] = collection.history
	}
	if collection.label == nil {
		return false
	}
	g.nodes[at].label = cloneRawAndNormalized(*collection.label)
	return true
}

// TerminalNames は、収集の端末のノード nodeId が名乗った名前ごとの記録を、名前が最初に現れた順に
// 返す。収集の端末でないノードと、名前を記録したレコードの無い収集の端末では要素数 0 である。
func (g Graph) TerminalNames(nodeId string) []TerminalName {
	return cloneTerminalNames(g.terminalNames[nodeId])
}

// cloneTerminalNames は名前ごとの記録の複製を返す。
func cloneTerminalNames(names []TerminalName) []TerminalName {
	cloned := make([]TerminalName, len(names))
	for index, name := range names {
		cloned[index] = TerminalName{
			Name: name.Name, First: cloneTimestampPointer(name.First), Last: cloneTimestampPointer(name.Last),
			RecordRefs: slices.Clone(name.RecordRefs),
		}
	}
	return cloned
}
