package winregistry

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	keyNodeHeaderSize   = 76
	keyValueHeaderSize  = 20
	keyCompressedName   = 0x20
	valueCompressedName = 0x1
	// inlineDataFlag は、inlineDataMaxSize byte 以下の data を data の位置の欄に直接置いたことを示す。
	inlineDataFlag    = 0x80000000
	inlineDataMaxSize = 4
	// listHeaderSize は subkey の list の署名と要素の数の byte 数である。
	listHeaderSize = 4
	// bigDataSegmentSize は big data の segment 1 つの最大の byte 数である。
	bigDataSegmentSize = 16344
	// bigDataMinorVersion は big data を使う hive の minor version の下限である。
	bigDataMinorVersion = 4
)

// keyNode は nk cell から読んだ値である。
type keyNode struct {
	cell        cell
	name        string
	lastWritten uint64
	subkeyCount uint32
	subkeyList  uint32
	valueCount  uint32
	valueList   uint32
}

// keyValue は vk cell と data から読んだ値である。
type keyValue struct {
	name      string
	valueType uint32
	data      []byte
	// cells は vk cell と data を持つ cell である。hive bins data の中の位置と大きさを持つ。
	cells []cell
}

// cellProblem は、cell を指す位置 1 つを読めなかったことである。at は失敗の位置として
// 示す cell で、読めなかった cell そのものか、その位置を持っていた cell である。
type cellProblem struct {
	at  cell
	err error
}

func readKeyNode(h *hive, offset uint32) (keyNode, error) {
	c, err := h.cellAt(offset)
	if err != nil {
		return keyNode{}, err
	}
	data := c.data
	if len(data) < keyNodeHeaderSize || string(data[:2]) != "nk" {
		return keyNode{cell: c}, fmt.Errorf("the cell at offset %d holds no key node", offset)
	}
	le := binary.LittleEndian
	nameLength := int(le.Uint16(data[72:]))
	if keyNodeHeaderSize+nameLength > len(data) {
		return keyNode{cell: c}, fmt.Errorf("the key node at offset %d declares a name past its cell", offset)
	}
	return keyNode{
		cell:        c,
		name:        nameText(data[keyNodeHeaderSize:keyNodeHeaderSize+nameLength], le.Uint16(data[2:])&keyCompressedName != 0),
		lastWritten: le.Uint64(data[4:]),
		subkeyCount: le.Uint32(data[20:]),
		subkeyList:  le.Uint32(data[28:]),
		valueCount:  le.Uint32(data[36:]),
		valueList:   le.Uint32(data[40:]),
	}, nil
}

// subkeys は key の subkey の nk cell の位置を、list の並びの順で返す。index root は 1 階層だけ辿る。
func subkeys(h *hive, key keyNode) ([]uint32, *cellProblem) {
	if key.subkeyCount == 0 || key.subkeyList == noCell {
		return nil, nil
	}
	return subkeyList(h, key.subkeyList, key.cell, true)
}

func subkeyList(h *hive, offset uint32, owner cell, allowRoot bool) ([]uint32, *cellProblem) {
	c, err := h.cellAt(offset)
	if err != nil {
		return nil, &cellProblem{at: owner, err: fmt.Errorf("the subkey list: %w", err)}
	}
	data := c.data
	if len(data) < listHeaderSize {
		return nil, &cellProblem{at: c, err: errors.New("the subkey list is shorter than its header")}
	}
	signature, count := string(data[:2]), int(binary.LittleEndian.Uint16(data[2:]))
	width := 4
	switch signature {
	case "lf", "lh":
		width = 8
	case "li":
	case "ri":
		if !allowRoot {
			return nil, &cellProblem{at: c, err: errors.New("an index root points to another index root")}
		}
	default:
		return nil, &cellProblem{at: c, err: fmt.Errorf("the cell at offset %d holds no subkey list", offset)}
	}
	if listHeaderSize+count*width > len(data) {
		return nil, &cellProblem{at: c, err: fmt.Errorf("the subkey list declares %d elements past its cell", count)}
	}
	var offsets []uint32
	for i := range count {
		element := binary.LittleEndian.Uint32(data[listHeaderSize+i*width:])
		if signature != "ri" {
			offsets = append(offsets, element)
			continue
		}
		nested, problem := subkeyList(h, element, c, false)
		if problem != nil {
			return offsets, problem
		}
		offsets = append(offsets, nested...)
	}
	return offsets, nil
}

// values は key の値を list の並びの順で返す。読めなかった値は problems に入り、ほかの値は返る。
func values(h *hive, key keyNode) ([]keyValue, *cell, []cellProblem) {
	if key.valueCount == 0 || key.valueList == noCell {
		return nil, nil, nil
	}
	list, err := h.cellAt(key.valueList)
	if err != nil {
		return nil, nil, []cellProblem{{at: key.cell, err: fmt.Errorf("the value list: %w", err)}}
	}
	count := int64(key.valueCount)
	if count*4 > int64(len(list.data)) {
		return nil, &list, []cellProblem{{at: list, err: fmt.Errorf("the value list holds fewer than %d elements", count)}}
	}
	var result []keyValue
	var problems []cellProblem
	for i := range count {
		value, problem := readKeyValue(h, binary.LittleEndian.Uint32(list.data[i*4:]), list)
		if problem != nil {
			problems = append(problems, *problem)
			continue
		}
		result = append(result, value)
	}
	return result, &list, problems
}

func readKeyValue(h *hive, offset uint32, list cell) (keyValue, *cellProblem) {
	c, err := h.cellAt(offset)
	if err != nil {
		return keyValue{}, &cellProblem{at: list, err: fmt.Errorf("a value: %w", err)}
	}
	data := c.data
	if len(data) < keyValueHeaderSize || string(data[:2]) != "vk" {
		return keyValue{}, &cellProblem{at: c, err: fmt.Errorf("the cell at offset %d holds no key value", offset)}
	}
	le := binary.LittleEndian
	nameLength := int(le.Uint16(data[2:]))
	if keyValueHeaderSize+nameLength > len(data) {
		return keyValue{}, &cellProblem{at: c, err: fmt.Errorf("the key value at offset %d declares a name past its cell", offset)}
	}
	value := keyValue{
		name:      nameText(data[keyValueHeaderSize:keyValueHeaderSize+nameLength], le.Uint16(data[16:])&valueCompressedName != 0),
		valueType: le.Uint32(data[12:]),
		cells:     []cell{c},
	}
	size, dataOffset := le.Uint32(data[4:]), le.Uint32(data[8:])
	switch {
	case size&inlineDataFlag != 0:
		size &^= inlineDataFlag
		if size > inlineDataMaxSize {
			return keyValue{}, &cellProblem{at: c, err: fmt.Errorf("the key value at offset %d declares %d inline bytes", offset, size)}
		}
		value.data = data[8 : 8+size]
	case size == 0:
		value.data = []byte{}
	default:
		cells, content, err := valueData(h, dataOffset, int64(size))
		if err != nil {
			return keyValue{}, &cellProblem{at: c, err: fmt.Errorf("the data of the key value at offset %d: %w", offset, err)}
		}
		value.data, value.cells = content, append(value.cells, cells...)
	}
	return value, nil
}

// valueData は data の cell を読む。big data は segment を並べて繋ぐ。
func valueData(h *hive, offset uint32, size int64) ([]cell, []byte, error) {
	c, err := h.cellAt(offset)
	if err != nil {
		return nil, nil, err
	}
	if h.minorVersion < bigDataMinorVersion || size <= bigDataSegmentSize || len(c.data) < 8 || string(c.data[:2]) != "db" {
		if size > int64(len(c.data)) {
			return nil, nil, fmt.Errorf("the value declares %d bytes past its data cell of %d bytes", size, len(c.data))
		}
		return []cell{c}, c.data[:size], nil
	}
	count := int64(binary.LittleEndian.Uint16(c.data[2:]))
	// 宣言の byte 数で確保する前に、segment の数が持てる byte 数と比べる。
	if size > count*bigDataSegmentSize {
		return nil, nil, fmt.Errorf("the value declares %d bytes, more than %d segments hold", size, count)
	}
	list, err := h.cellAt(binary.LittleEndian.Uint32(c.data[4:]))
	if err != nil {
		return nil, nil, fmt.Errorf("the segment list: %w", err)
	}
	if count*4 > int64(len(list.data)) {
		return nil, nil, fmt.Errorf("the segment list holds fewer than %d elements", count)
	}
	cells := []cell{c, list}
	// segment は重複して読めないため、内容は hive bins data より大きくならない。
	content := make([]byte, 0, min(size, int64(len(h.bins))))
	for i := range count {
		segment, err := h.cellAt(binary.LittleEndian.Uint32(list.data[i*4:]))
		if err != nil {
			return nil, nil, fmt.Errorf("segment %d: %w", i, err)
		}
		cells = append(cells, segment)
		// segment の cell は 8 byte 境界まで埋めた分を持つ。最後以外の segment は最大の byte 数である。
		take := min(int64(len(segment.data)), bigDataSegmentSize, size-int64(len(content)))
		content = append(content, segment.data[:take]...)
	}
	if int64(len(content)) < size {
		return nil, nil, fmt.Errorf("the segments hold %d of the declared %d bytes", len(content), size)
	}
	return cells, content, nil
}
