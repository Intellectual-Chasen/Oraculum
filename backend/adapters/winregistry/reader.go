package winregistry

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

var errNotReset = errors.New("registry hive: call Reset with an input first")

// Member は収集元を構成する file 1 つの、連結した byte 列の中の範囲である。
type Member struct {
	// Name は file の名前である。
	Name string
	// Offset は file の先頭の、連結した byte 列の中の位置である。
	Offset int64
	// Size は file の byte 数である。
	Size int64
}

// Record は key 1 つを読んだ結果、または読めなかった cell 1 つである。
type Record struct {
	// RawText は位置が指す cell の byte を大文字の 16 進で書いた文字列である。
	RawText string
	// ByteOffset と ByteLength は、cell の先頭の、連結した byte 列の中の連続した範囲である。
	ByteOffset int64
	ByteLength int64
	// ObservedAt は key の最終更新の時刻である。値が 0 の key と失敗では nil である。タスクの
	// 登録を記録した TaskCache の key では、DynamicInfo の値が持つ登録の時刻である。
	ObservedAt *core.Timestamp
	// Fields は key の項目である。失敗では nil である。
	Fields []core.RecordField
	// TerminalCandidates は、key が記録した端末の名前である。
	TerminalCandidates []string
	// TerminalNames は、key が記録した、hive を置いた端末自身の名前である。コンピューター名
	// (ComputerName) と TCP/IP のホスト名 (Hostname) の値を、値の並びの順に持つ。
	TerminalNames []string
}

// computerNameKey は、端末の名前を値 ComputerName に持つ key の path である。
var computerNameKey = regexp.MustCompile(`(?i)^\\ControlSet\d{3}\\Control\\ComputerName\\ComputerName$`)

// tcpipParametersKey は、端末の TCP/IP のホスト名を値 Hostname に持つ key の path である。
var tcpipParametersKey = regexp.MustCompile(`(?i)^\\ControlSet\d{3}\\Services\\Tcpip\\Parameters$`)

// Reader は hive の key を、root から深さ優先で 1 件ずつ返す。
type Reader struct {
	members  []Member
	content  []byte
	readErr  error
	wasReset bool
	started  bool
	hive     *hive
	stack    []pendingKey
	pending  []pendingItem
	header   []core.RecordField
}

type pendingKey struct {
	offset uint32
	// parent は親の key の path である。root では空である。
	parent string
	// owner は key の位置を持っていた cell である。root では大きさ 0 である。
	owner cell
}

type pendingItem struct {
	record  Record
	failure *core.ImportFailure
}

// SetMembers は連結した byte 列を構成する file の範囲を与える。先頭が主 file、その後が log
// である。Reset の前に呼ぶ。与えないときは、入力の全体を主 file として読む。
func (r *Reader) SetMembers(members []Member) {
	r.members = slices.Clone(members)
}

// Reset は入力を差し替え、走査を先頭に戻す。入力全体を読み切ってから走査する。
func (r *Reader) Reset(input io.Reader) {
	members := r.members
	*r = Reader{members: members}
	if input == nil {
		return
	}
	r.wasReset = true
	r.content, r.readErr = io.ReadAll(input)
}

// SourceHeader は主 file の base block の値と、log の適用の結果を返す。走査が末尾に達した後に呼ぶ。
func (r *Reader) SourceHeader() []core.RecordField {
	return slices.Clone(r.header)
}

// Next は次の key 1 件を返す。返り値の組み合わせは次の 3 通りである。
//
//   - 読めた key: レコードを返し、失敗と error は nil である。
//   - 読めなかった cell: 位置と原文を持つレコードと失敗を返し、error は nil である。
//   - 読み取りの失敗: 失敗と error を返し、走査は止まる。
func (r *Reader) Next() (Record, *core.ImportFailure, error) {
	if !r.wasReset {
		return Record{}, nil, fmt.Errorf("reading registry hive source: %w", errNotReset)
	}
	if r.readErr != nil {
		err := r.readErr
		r.readErr, r.started, r.stack = nil, true, nil
		return Record{}, failureAt(core.FailureStageRead, 0, "a readable registry hive", err.Error()),
			fmt.Errorf("reading registry hive source: %w", err)
	}
	if !r.started {
		r.started = true
		r.start()
	}
	for len(r.pending) == 0 && len(r.stack) > 0 {
		key := r.stack[len(r.stack)-1]
		r.stack = r.stack[:len(r.stack)-1]
		r.visit(key)
	}
	if len(r.pending) == 0 {
		return Record{}, nil, io.EOF
	}
	item := r.pending[0]
	r.pending = r.pending[1:]
	return item.record, item.failure, nil
}

// start は主 file の base block を読み、log を適用し、root の key を積む。
func (r *Reader) start() {
	members := r.members
	if len(members) == 0 {
		members = []Member{{Offset: 0, Size: int64(len(r.content))}}
	}
	primary := r.content[members[0].Offset : members[0].Offset+members[0].Size]
	base, ok := parseBaseBlock(primary)
	switch {
	case !ok || len(primary) < baseBlockSize:
		r.failBaseBlock(primary, "a primary file starting with a 4096-byte regf base block")
		return
	case base.fileType != fileTypePrimary:
		r.failBaseBlock(primary, "a primary file (file type 0), found file type "+strconv.FormatUint(uint64(base.fileType), 10))
		return
	}
	var logs []*logFile
	for _, member := range members[1:] {
		logs = append(logs, readLog(member.Name, member.Offset, r.content[member.Offset:member.Offset+member.Size]))
	}
	h, result := recoverHive(primary, base, logs)
	r.hive = h
	r.header = headerFields(base, h, result, members, logs)
	if result.state == recoveryNotApplied {
		r.failBaseBlock(primary, "a dirty hive recovered from its transaction logs; "+result.reason+
			"; the keys that follow are those of the primary file alone")
	}
	r.stack = []pendingKey{{offset: h.rootCellOffset}}
}

func (r *Reader) failBaseBlock(primary []byte, expected string) {
	block := primary[:min(len(primary), baseBlockSize)]
	record := Record{RawText: strings.ToUpper(hex.EncodeToString(block)), ByteLength: int64(len(block))}
	r.pending = append(r.pending, pendingItem{
		record:  record,
		failure: failureAt(core.FailureStageTokenize, 0, expected, "the base block does not meet the expectation"),
	})
}

// visit は key 1 つを読み、レコードと失敗を積み、subkey を積む。
func (r *Reader) visit(key pendingKey) {
	node, err := readKeyNode(r.hive, key.offset)
	if err != nil {
		at := node.cell
		if at.size == 0 {
			at = key.owner
		}
		r.fail(at, err)
		return
	}
	// root の key の名前は hive を読み込んだ位置で決まり、path に入れない。
	path := `\`
	switch key.parent {
	case "":
	case `\`:
		path += node.name
	default:
		path = key.parent + `\` + node.name
	}
	children, listProblem := subkeys(r.hive, node)
	keyValues, valueList, problems := values(r.hive, node)
	r.pending = append(r.pending, pendingItem{record: r.keyRecord(path, node, keyValues, valueList, len(problems))})
	if listProblem != nil {
		problems = append(problems, *listProblem)
	}
	for _, problem := range problems {
		r.fail(problem.at, problem.err)
	}
	for _, child := range slices.Backward(children) {
		r.stack = append(r.stack, pendingKey{offset: child, parent: path, owner: node.cell})
	}
}

// fail は at の cell を位置とする失敗を積む。at の大きさが 0 のときは base block を位置にする。
func (r *Reader) fail(at cell, err error) {
	record := Record{ByteOffset: 0, ByteLength: baseBlockSize}
	if at.size > 0 {
		spans := r.hive.spans(at.rel, at.size)
		record.ByteOffset, record.ByteLength = spans[0].offset, spans[0].length
		record.RawText = strings.ToUpper(hex.EncodeToString(r.hive.bins[at.rel : at.rel+at.size]))
		if len(spans) > 1 {
			// 位置は先頭の範囲だけを指す。原文の cell の残りの byte がある範囲を文面に残す。
			err = fmt.Errorf("%w (the cell bytes lie at %s)", err, spansText(spans))
		}
	} else {
		record.RawText = strings.ToUpper(hex.EncodeToString(r.content[:min(len(r.content), baseBlockSize)]))
	}
	r.pending = append(r.pending, pendingItem{
		record: record,
		failure: failureAt(core.FailureStageTokenize, record.ByteOffset,
			"registry cells reachable from the root key", err.Error()),
	})
}

// keyRecord は key 1 つのレコードを組む。
func (r *Reader) keyRecord(path string, node keyNode, keyValues []keyValue, valueList *cell, unreadable int) Record {
	h := r.hive
	spans := h.spans(node.cell.rel, node.cell.size)
	record := Record{
		RawText:    strings.ToUpper(hex.EncodeToString(h.bins[node.cell.rel : node.cell.rel+node.cell.size])),
		ByteOffset: spans[0].offset, ByteLength: spans[0].length,
	}
	fields := []core.RecordField{textField(fieldKeyPath, path)}
	if node.lastWritten != 0 {
		if field, timestamp, ok := fileTimeField(fieldLastWrittenTime, node.lastWritten); ok {
			fields = append(fields, field)
			record.ObservedAt = &timestamp
		} else {
			// 時刻として書けない値も、原資料の文字列の欄として残す。
			fields = append(fields, textField(fieldLastWrittenTime, strconv.FormatUint(node.lastWritten, 10)))
		}
	}
	if len(spans) > 1 {
		fields = append(fields, textField(fieldCellSegments, spansText(spans)))
	}
	entries := map[int32]struct{}{}
	h.addEntries(node.cell.rel, node.cell.size, entries)
	if valueList != nil {
		h.addEntries(valueList.rel, valueList.size, entries)
	}
	used := map[string]struct{}{}
	for _, value := range keyValues {
		// 同じ名前の値には、まだ使っていない `#<番号>` を付ける。
		name := value.name
		for count := 2; ; count++ {
			if _, taken := used[name]; !taken {
				break
			}
			name = value.name + "#" + strconv.Itoa(count)
		}
		used[name] = struct{}{}
		var cellSpans []span
		for _, c := range value.cells {
			cellSpans = append(cellSpans, h.spans(c.rel, c.size)...)
			h.addEntries(c.rel, c.size, entries)
		}
		valueField, mapped := semanticValueField(path, valueFieldName(name), value, name == value.name)
		if !mapped {
			valueField = textField(valueFieldName(name), valueText(value.valueType, value.data))
		}
		fields = append(fields,
			valueField,
			textField(fieldValueType+"["+name+"]", typeName(value.valueType)),
			textField(fieldValueCell+"["+name+"]", spansText(cellSpans)),
		)
		if value.valueType != regSZ && value.valueType != regExpandSZ {
			continue
		}
		computerName := value.name == "ComputerName" && computerNameKey.MatchString(path)
		if !computerName && (value.name != "Hostname" || !tcpipParametersKey.MatchString(path)) {
			continue
		}
		if text, ok := utf16Text(value.data); ok && text != "" {
			record.TerminalNames = append(record.TerminalNames, text)
			if computerName {
				record.TerminalCandidates = append(record.TerminalCandidates, text)
			}
		}
	}
	taskFields, registered := taskCacheFields(path, keyValues)
	fields = append(fields, taskFields...)
	if registered != nil {
		record.ObservedAt = registered
	}
	if unreadable > 0 {
		fields = append(fields, textField(fieldUnreadableValueCount, strconv.Itoa(unreadable)))
	}
	if len(entries) > 0 {
		fields = append(fields, textField(fieldAppliedLogEntry, appliedEntriesText(h, entries)))
	}
	record.Fields = fields
	return record
}

// valueFieldName は値の項目の名前である。名前の無い値 (既定の値) は `Value` である。
func valueFieldName(name string) string {
	if name == "" {
		return fieldValue
	}
	return fieldValue + "." + name
}

// spansText は範囲を `<位置>+<byte 数>` の並びの文字列にする。位置は連結した byte 列の中である。
func spansText(spans []span) string {
	parts := make([]string, len(spans))
	for i, s := range spans {
		parts[i] = strconv.FormatInt(s.offset, 10) + "+" + strconv.FormatInt(s.length, 10)
	}
	return strings.Join(parts, " ")
}

func appliedEntriesText(h *hive, entries map[int32]struct{}) string {
	indexes := make([]int32, 0, len(entries))
	for index := range entries {
		indexes = append(indexes, index)
	}
	slices.Sort(indexes)
	parts := make([]string, len(indexes))
	for i, index := range indexes {
		entry := h.entries[index]
		parts[i] = entry.member + " sequence " + strconv.FormatUint(uint64(entry.sequence), 10)
	}
	return strings.Join(parts, ", ")
}
