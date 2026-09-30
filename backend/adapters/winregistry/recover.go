package winregistry

import (
	"encoding/binary"
	"fmt"
	"strconv"
)

// logEntryHeaderSize は log entry の見出しの byte 数である。dirty page の参照がその後に続く。
const logEntryHeaderSize = 40

// 適用の結果の値。見出しの Recovery の文字列である。
const (
	recoveryNotNeeded = "not_needed"
	recoveryApplied   = "applied"
	// recoveryNotApplied は、主 file が dirty で、適用できる log entry が無い状態である。
	recoveryNotApplied = "not_applied"
)

// logFile は log の file 1 つと、先頭から読めた log entry の並びである。
type logFile struct {
	name string
	// offset は file の先頭の、連結した byte 列の中の位置である。
	offset int64
	data   []byte
	base   baseBlock
	// usable は、base block が新形式の有効な写しで、log entry を読んだかである。
	usable  bool
	entries []logEntry
	// status は見出しに書く file の状態である。
	status string
	// applied は適用した log entry の番号の範囲である。適用しなかった file では空である。
	applied string
}

// logEntry は検査を通った log entry 1 件である。
type logEntry struct {
	offset   int64
	sequence uint32
	binsSize uint32
	pages    []dirtyPage
}

// dirtyPage は log entry が主 file へ書く page 1 つである。
type dirtyPage struct {
	// rel は page の書き込み先の、hive bins data の先頭からの位置である。
	rel int64
	// data は log の file の中の page の byte 列である。
	data []byte
	// fileOffset は page の、log の file の先頭からの位置である。
	fileOffset int64
}

// readLog は log の file 1 つの base block と log entry を読む。
func readLog(name string, offset int64, data []byte) *logFile {
	log := &logFile{name: name, offset: offset, data: data}
	if len(data) == 0 {
		log.status = "empty"
		return log
	}
	base, ok := parseBaseBlock(data)
	switch {
	case !ok:
		log.status = "invalid base block: no regf signature in the first 512 bytes"
		return log
	case !base.checksumValid:
		log.status = "invalid base block: checksum mismatch"
		return log
	case base.fileType == fileTypeLogLegacy || base.fileType == fileTypeLogOld:
		log.status = "legacy format (file type " + strconv.FormatUint(uint64(base.fileType), 10) + ") unsupported"
		return log
	case base.fileType != fileTypeLogNew:
		log.status = "unsupported file type " + strconv.FormatUint(uint64(base.fileType), 10)
		return log
	case base.primarySequence != base.secondarySequence:
		log.status = "invalid base block: sequence numbers differ"
		return log
	}
	log.base, log.usable = base, true
	position := int64(logBaseBlockSize)
	expected := base.primarySequence
	stop := "reached the end of the file"
	for position+logEntryHeaderSize <= int64(len(data)) {
		entry, problem := readLogEntry(data, position, expected)
		if problem != "" {
			stop = "stopped at byte " + strconv.FormatInt(position, 10) + ": " + problem
			break
		}
		log.entries = append(log.entries, entry)
		position += int64(binary.LittleEndian.Uint32(data[position+4:]))
		expected++
	}
	log.status = strconv.Itoa(len(log.entries)) + " entries"
	if len(log.entries) > 0 {
		log.status += ", sequence " + sequenceRange(log.entries[0].sequence, log.entries[len(log.entries)-1].sequence)
	}
	log.status += "; " + stop
	return log
}

// readLogEntry は position の log entry を検査して読む。検査を通らないときは理由を返す。
func readLogEntry(data []byte, position int64, expected uint32) (logEntry, string) {
	le := binary.LittleEndian
	header := data[position:]
	if string(header[:4]) != "HvLE" {
		return logEntry{}, "no HvLE signature"
	}
	size := int64(le.Uint32(header[4:]))
	if size < logBaseBlockSize || size%logBaseBlockSize != 0 || position+size > int64(len(data)) {
		return logEntry{}, "entry size " + strconv.FormatInt(size, 10) + " is not a multiple of 512 within the file"
	}
	entry := header[:size]
	sequence, binsSize, count := le.Uint32(entry[12:]), le.Uint32(entry[16:]), int64(le.Uint32(entry[20:]))
	if binsSize < hiveBinAlignment || binsSize%hiveBinAlignment != 0 {
		return logEntry{}, "hive bins data size " + strconv.FormatUint(uint64(binsSize), 10) + " is not a multiple of 4096"
	}
	if count == 0 || logEntryHeaderSize+count*8 > size {
		return logEntry{}, "dirty page count " + strconv.FormatInt(count, 10) + " does not fit the entry"
	}
	if marvin32(entry[:32], logEntrySeed) != le.Uint64(entry[32:]) {
		return logEntry{}, "Hash-2 mismatch"
	}
	if marvin32(entry[logEntryHeaderSize:], logEntrySeed) != le.Uint64(entry[24:]) {
		return logEntry{}, "Hash-1 mismatch"
	}
	if sequence != expected {
		return logEntry{}, fmt.Sprintf("sequence %d where %d was expected", sequence, expected)
	}
	result := logEntry{offset: position, sequence: sequence, binsSize: binsSize}
	pageData := logEntryHeaderSize + count*8
	for i := range count {
		reference := entry[logEntryHeaderSize+i*8:]
		rel, pageSize := int64(le.Uint32(reference)), int64(le.Uint32(reference[4:]))
		if pageSize == 0 || rel%unitSize != 0 || pageSize%unitSize != 0 ||
			rel+pageSize > int64(binsSize) || pageData+pageSize > size {
			return logEntry{}, "dirty page reference " + strconv.FormatInt(i, 10) + " lies outside the entry or the hive bins data"
		}
		result.pages = append(result.pages, dirtyPage{
			rel: rel, data: entry[pageData : pageData+pageSize], fileOffset: position + pageData,
		})
		pageData += pageSize
	}
	return result, ""
}

// recovery は log の適用の結果である。
type recovery struct {
	state string
	// reason は適用しなかった理由である。state が recoveryNotApplied のときだけ値を持つ。
	reason string
}

// recoverHive は主 file の hive bins data を組み、主 file が dirty のときは log を適用する。
//
// 順序と開始の条件は regf の仕様の新形式の log に従う。主 file の base block が有効なときは、
// 古い番号の log から適用し、続く log の base block の番号が最後に適用した番号の次である
// ときだけ続ける。最初の log の base block の番号は主 file の secondary sequence number
// 以上である。主 file の base block が無効なときは、新しい番号の log 1 つだけを使う。
func recoverHive(primary []byte, base baseBlock, logs []*logFile) (*hive, recovery) {
	binsSize := int64(base.hiveBinsDataSize)
	if !base.checksumValid {
		binsSize = (int64(len(primary)) - baseBlockSize) / hiveBinAlignment * hiveBinAlignment
	}
	available := min(binsSize, int64(len(primary))-baseBlockSize)
	h := &hive{minorVersion: base.minorVersion, rootCellOffset: base.rootCellOffset}
	if !base.dirty() {
		h.bins = primary[baseBlockSize : baseBlockSize+available]
		for _, log := range logs {
			if log.usable || log.status != "empty" {
				log.status += "; not applied: the primary file is not dirty"
			}
		}
		return h, recovery{state: recoveryNotNeeded}
	}
	var usable []*logFile
	for _, log := range logs {
		if log.usable && len(log.entries) > 0 {
			usable = append(usable, log)
		}
	}
	if len(usable) == 2 && sequenceBefore(usable[1].base.primarySequence, usable[0].base.primarySequence) {
		usable[0], usable[1] = usable[1], usable[0]
	}
	var chosen []*logFile
	switch {
	case len(usable) == 0:
	case !base.checksumValid:
		chosen = usable[len(usable)-1:]
	default:
		eligible := func(log *logFile) bool {
			return !sequenceBefore(log.base.primarySequence, base.secondarySequence)
		}
		for i, log := range usable {
			if eligible(log) {
				chosen = usable[i:]
				break
			}
		}
	}
	if len(chosen) == 0 {
		h.bins = primary[baseBlockSize : baseBlockSize+available]
		reason := "no transaction log holds entries that continue the primary file"
		if len(usable) == 0 {
			reason = "no transaction log holds a readable entry"
		}
		for _, log := range usable {
			log.status += "; not applied: its base block sequence precedes the primary file"
		}
		return h, recovery{state: recoveryNotApplied, reason: reason}
	}
	if !base.checksumValid {
		h.minorVersion, h.rootCellOffset = chosen[0].base.minorVersion, chosen[0].base.rootCellOffset
	}
	h.bins = append([]byte(nil), primary[baseBlockSize:baseBlockSize+available]...)
	units := (len(h.bins) + unitSize - 1) / unitSize
	h.unitOrigin, h.unitEntry = make([]int64, units), make([]int32, units)
	for unit := range h.unitOrigin {
		h.unitOrigin[unit] = baseBlockSize + int64(unit)*unitSize
	}
	for i, log := range chosen {
		// 続く log は、最後に適用した番号の次の番号から始まるときだけ適用する。
		if i > 0 && (len(h.entries) == 0 || log.base.primarySequence != h.entries[len(h.entries)-1].sequence+1) {
			break
		}
		h.applyLog(log)
	}
	for _, log := range logs {
		if log.applied != "" {
			log.status += "; applied " + log.applied
		} else if log.usable {
			log.status += "; not applied: its entries do not continue the applied sequence"
		}
	}
	if len(h.entries) == 0 {
		return h, recovery{state: recoveryNotApplied, reason: "no transaction log entry could be applied"}
	}
	return h, recovery{state: recoveryApplied}
}

// applyLog は log の entry を先頭から順に適用する。hive bins data を広げた分が、entry の page が
// 広げた範囲に書く byte 数より大きい entry で止める。
//
// Windows は新しい hbin を dirty page として log に書くため、正しい log の entry は広げた範囲を
// page で埋める。page を持たない大きさを受け入れると、壊れた欄 1 つが数 GB の確保になる。
func (h *hive) applyLog(log *logFile) {
	applied := 0
	for _, entry := range log.entries {
		current, size := int64(len(h.bins)), int64(entry.binsSize)
		var written int64
		for _, page := range entry.pages {
			start, end := max(page.rel, current), min(page.rel+int64(len(page.data)), size)
			written += max(end-start, 0)
		}
		if size-current > (written+hiveBinAlignment-1)/hiveBinAlignment*hiveBinAlignment {
			log.status += "; stopped applying at sequence " + strconv.FormatUint(uint64(entry.sequence), 10) +
				": the entry grows the hive bins data past its pages"
			break
		}
		h.apply(log, entry)
		applied++
	}
	if applied > 0 {
		log.applied = sequenceRange(log.entries[0].sequence, log.entries[applied-1].sequence)
	}
}

// apply は log entry 1 件の dirty page を hive bins data へ書き、hive bins data の大きさを
// entry の値にする。
func (h *hive) apply(log *logFile, entry logEntry) {
	size := int(entry.binsSize)
	units := (size + unitSize - 1) / unitSize
	if size > len(h.bins) {
		h.bins = append(h.bins, make([]byte, size-len(h.bins))...)
		for len(h.unitOrigin) < units {
			h.unitOrigin = append(h.unitOrigin, -1)
			h.unitEntry = append(h.unitEntry, 0)
		}
	}
	h.bins, h.unitOrigin, h.unitEntry = h.bins[:size], h.unitOrigin[:units], h.unitEntry[:units]
	h.entries = append(h.entries, appliedEntry{member: log.name, sequence: entry.sequence})
	index := int32(len(h.entries)) // #nosec G115 -- log entry の件数は file の byte 数 / 512 以下である。
	for _, page := range entry.pages {
		copy(h.bins[page.rel:], page.data)
		for k := int64(0); k < int64(len(page.data)); k += unitSize {
			unit := (page.rel + k) / unitSize
			h.unitOrigin[unit] = log.offset + page.fileOffset + k
			h.unitEntry[unit] = index
		}
	}
}

// sequenceBefore は、32 bit で一巡する番号 a が b より前であるかを返す。
func sequenceBefore(a, b uint32) bool {
	return int32(a-b) < 0 // #nosec G115 -- 差を符号付きで読み、一巡した番号の前後を比べる。
}

// sequenceRange は番号の範囲の文字列である。
func sequenceRange(first, last uint32) string {
	return strconv.FormatUint(uint64(first), 10) + "-" + strconv.FormatUint(uint64(last), 10)
}
