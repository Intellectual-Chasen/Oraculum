package winevent_test

import (
	"bytes"
	"encoding/binary"
	"strings"
	"unicode/utf16"
)

// 本 file は test の中で EVTX の byte 列を組む。原資料の file を写していない。組む構造は
// EVTX の file の見出し、chunk の見出し、レコードの見出しと、
// レコードの本文の BinXML (template の定義と、template へ差し込む値) である。

const (
	synthChunkSize       = 0x10000
	synthChunkHeaderSize = 0x200
	synthFileHeaderSize  = 0x1000
	// BinXML の token。
	tokenEOF              = 0x00
	tokenOpenStart        = 0x01
	tokenOpenStartAttrs   = 0x41
	tokenCloseStart       = 0x02
	tokenCloseEmpty       = 0x03
	tokenCloseElement     = 0x04
	tokenValueText        = 0x05
	tokenAttribute        = 0x06
	tokenTemplateInstance = 0x0c
	tokenSubstitution     = 0x0e
	tokenFragmentHeader   = 0x0f
	// 差し込む値の型。
	typeString   = 0x01
	typeUint16   = 0x06
	typeUint64   = 0x0a
	typeGUID     = 0x0f
	typeFileTime = 0x11
	typeHexInt64 = 0x15
	// typeStringArray は文字列の配列の型である。要素を NUL で区切る。
	typeStringArray = 0x81
)

// synthData は `<Data Name="...">` 1 つである。guid を持つ値は GUID の型で差し込む。list を持つ
// 値は文字列の配列の型で差し込む。name が空の値は Name の属性を持たない `<Data>` である。
type synthData struct {
	name, value string
	guid        []byte
	list        []string
}

// synthEvent はレコード 1 件の値である。
type synthEvent struct {
	// headerID はレコードの見出しの番号、recordID は System の EventRecordID である。
	headerID, recordID uint64
	// writtenAt はレコードの見出しの書き込み時刻、created は TimeCreated の SystemTime で
	// ある。どちらも FILETIME である。
	writtenAt, created uint64
	provider           string
	eventID            uint16
	keywords           uint64
	channel, computer  string
	data               []synthData
	// activityID を持つイベントは `<Correlation ActivityID="..."/>` を GUID の型で持つ。
	activityID []byte
	// providerAttribute はプロバイダの名前を置く属性の名前である。空は Name である。
	providerAttribute string
	// userDataItem を持つイベントは、名前空間を宣言した UserData の要素 1 つにこの値を持つ。
	userDataItem string
}

// synthChunk は chunk 1 つを組む。buf は chunk の先頭からの byte 列である。BinXML の名前は
// chunk の中の位置で参照されるため、位置を chunk の先頭から数える。
type synthChunk struct {
	buf       bytes.Buffer
	firstID   uint64
	lastID    uint64
	templates uint32
}

func newSynthChunk() *synthChunk {
	c := &synthChunk{}
	c.buf.Write(make([]byte, synthChunkHeaderSize))
	return c
}

func (c *synthChunk) u8(v uint8)   { c.buf.WriteByte(v) }
func (c *synthChunk) u16(v uint16) { _ = binary.Write(&c.buf, binary.LittleEndian, v) }
func (c *synthChunk) u32(v uint32) { _ = binary.Write(&c.buf, binary.LittleEndian, v) }
func (c *synthChunk) u64(v uint64) { _ = binary.Write(&c.buf, binary.LittleEndian, v) }

func utf16le(text string) []byte {
	var out bytes.Buffer
	for _, unit := range utf16.Encode([]rune(text)) {
		_ = binary.Write(&out, binary.LittleEndian, unit)
	}
	return out.Bytes()
}

// name は名前をその場に書く。名前の位置は、位置の欄の直後である。
func (c *synthChunk) name(text string) {
	c.u32(uint32(c.buf.Len() + 4))
	c.u32(0) // 次の名前の位置
	c.u16(0) // hash
	c.u16(uint16(len([]rune(text))))
	c.buf.Write(utf16le(text))
	c.u16(0)
}

// open は template の中の開始タグを書く。
func (c *synthChunk) open(element string, attributes bool) {
	if attributes {
		c.u8(tokenOpenStartAttrs)
	} else {
		c.u8(tokenOpenStart)
	}
	c.u16(0) // dependency id
	c.u32(0) // 要素の長さ。ライブラリは読まない。
	c.name(element)
	if attributes {
		c.u32(0) // 属性の列の長さ。ライブラリは読まない。
	}
}

func (c *synthChunk) literal(text string) {
	c.u8(tokenValueText)
	c.u8(typeString)
	c.u16(uint16(len([]rune(text))))
	c.buf.Write(utf16le(text))
}

func (c *synthChunk) substitution(index uint16, valueType uint8) {
	c.u8(tokenSubstitution)
	c.u16(index)
	c.u8(valueType)
}

// textElement は `<element>%index%</element>` を書く。
func (c *synthChunk) textElement(element string, index uint16, valueType uint8) {
	c.open(element, false)
	c.u8(tokenCloseStart)
	c.substitution(index, valueType)
	c.u8(tokenCloseElement)
}

// attributeElement は `<element attribute="%index%"/>` を書く。
func (c *synthChunk) attributeElement(element, attribute string, index uint16, valueType uint8) {
	c.open(element, true)
	c.u8(tokenAttribute)
	c.name(attribute)
	c.substitution(index, valueType)
	c.u8(tokenCloseEmpty)
}

// template は event の形の template の本文を書く。差し込む値の番号は synthArguments の並びである。
func (c *synthChunk) template(event synthEvent) {
	c.u8(tokenFragmentHeader)
	c.buf.Write([]byte{1, 1, 0})
	c.open("Event", false)
	c.u8(tokenCloseStart)
	c.open("System", false)
	c.u8(tokenCloseStart)
	providerAttribute := event.providerAttribute
	if providerAttribute == "" {
		providerAttribute = "Name"
	}
	c.attributeElement("Provider", providerAttribute, 0, typeString)
	c.textElement("EventID", 1, typeUint16)
	c.textElement("Keywords", 2, typeHexInt64)
	c.attributeElement("TimeCreated", "SystemTime", 3, typeFileTime)
	c.textElement("EventRecordID", 4, typeUint64)
	c.textElement("Channel", 5, typeString)
	c.textElement("Computer", 6, typeString)
	if event.activityID != nil {
		c.attributeElement("Correlation", "ActivityID", uint16(7+len(event.data)), typeGUID)
	}
	c.u8(tokenCloseElement)
	if len(event.data) > 0 {
		c.open("EventData", false)
		c.u8(tokenCloseStart)
		for i, data := range event.data {
			c.open("Data", data.name != "")
			if data.name != "" {
				c.u8(tokenAttribute)
				c.name("Name")
				c.literal(data.name)
			}
			c.u8(tokenCloseStart)
			c.substitution(uint16(7+i), dataType(data))
			c.u8(tokenCloseElement)
		}
		c.u8(tokenCloseElement)
	}
	if event.userDataItem != "" {
		// `<UserData><Payload xmlns:ns1="urn:example:events"><Item>...</Item></Payload></UserData>`
		c.open("UserData", false)
		c.u8(tokenCloseStart)
		c.open("Payload", true)
		c.u8(tokenAttribute)
		c.name("xmlns:ns1")
		c.literal("urn:example:events")
		c.u8(tokenCloseStart)
		c.open("Item", false)
		c.u8(tokenCloseStart)
		c.literal(event.userDataItem)
		c.u8(tokenCloseElement)
		c.u8(tokenCloseElement)
		c.u8(tokenCloseElement)
	}
	c.u8(tokenCloseElement)
	c.u8(tokenEOF)
}

// synthArgument は template へ差し込む値 1 つの型と byte 列である。
type synthArgument struct {
	valueType uint8
	value     []byte
}

func synthArguments(event synthEvent) []synthArgument {
	le := binary.LittleEndian
	arguments := []synthArgument{
		{typeString, utf16le(event.provider)},
		{typeUint16, le.AppendUint16(nil, event.eventID)},
		{typeHexInt64, le.AppendUint64(nil, event.keywords)},
		{typeFileTime, le.AppendUint64(nil, event.created)},
		{typeUint64, le.AppendUint64(nil, event.recordID)},
		{typeString, utf16le(event.channel)},
		{typeString, utf16le(event.computer)},
	}
	for _, data := range event.data {
		value := utf16le(data.value)
		if data.guid != nil {
			value = data.guid
		}
		if data.list != nil {
			value = utf16le(strings.Join(data.list, "\x00"))
		}
		arguments = append(arguments, synthArgument{dataType(data), value})
	}
	if event.activityID != nil {
		arguments = append(arguments, synthArgument{typeGUID, event.activityID})
	}
	return arguments
}

func dataType(data synthData) uint8 {
	if data.guid != nil {
		return typeGUID
	}
	if data.list != nil {
		return typeStringArray
	}
	return typeString
}

// record はレコード 1 件を書き、chunk の中の開始位置と大きさを返す。template は
// レコードごとに別の番号で定義する。
func (c *synthChunk) record(event synthEvent) (int, int) {
	start := c.buf.Len()
	if c.firstID == 0 {
		c.firstID = event.headerID
	}
	c.lastID = event.headerID
	c.buf.WriteString("**\x00\x00")
	sizeAt := c.buf.Len()
	c.u32(0)
	c.u64(event.headerID)
	c.u64(event.writtenAt)
	c.u8(tokenFragmentHeader)
	c.buf.Write([]byte{1, 1, 0})
	c.u8(tokenTemplateInstance)
	c.u8(1)
	c.templates++
	c.u32(c.templates)
	c.u32(0) // template の定義の位置
	c.u32(0) // 次の template の位置
	c.buf.Write(make([]byte, 16))
	bodyLengthAt := c.buf.Len()
	c.u32(0)
	bodyStart := c.buf.Len()
	c.template(event)
	binary.LittleEndian.PutUint32(c.buf.Bytes()[bodyLengthAt:], uint32(c.buf.Len()-bodyStart))
	arguments := synthArguments(event)
	c.u32(uint32(len(arguments)))
	for _, argument := range arguments {
		c.u16(uint16(len(argument.value)))
		c.u16(uint16(argument.valueType))
	}
	for _, argument := range arguments {
		c.buf.Write(argument.value)
	}
	c.u8(tokenEOF)
	size := c.buf.Len() - start + 4
	c.u32(uint32(size))
	binary.LittleEndian.PutUint32(c.buf.Bytes()[sizeAt:], uint32(size))
	return start, size
}

// bytes は chunk の見出しを埋め、chunk の大きさまで 0 で埋めた byte 列を返す。
func (c *synthChunk) bytes() []byte {
	out := make([]byte, synthChunkSize)
	copy(out, c.buf.Bytes())
	copy(out, "ElfChnk\x00")
	le := binary.LittleEndian
	le.PutUint64(out[8:], c.firstID)
	le.PutUint64(out[16:], c.lastID)
	le.PutUint64(out[24:], c.firstID)
	le.PutUint64(out[32:], c.lastID)
	le.PutUint32(out[40:], 128)
	le.PutUint32(out[48:], uint32(c.buf.Len())) // 空き領域の始まり
	return out
}

// synthFileHeader は file の見出しの byte 列を返す。
func synthFileHeader(nextRecordID uint64, chunks int, flags uint32) []byte {
	out := make([]byte, synthFileHeaderSize)
	le := binary.LittleEndian
	copy(out, "ElfFile\x00")
	le.PutUint64(out[8:], 0)
	le.PutUint64(out[16:], uint64(max(chunks-1, 0)))
	le.PutUint64(out[24:], nextRecordID)
	le.PutUint32(out[32:], 128)
	le.PutUint16(out[36:], 1)
	le.PutUint16(out[38:], 3)
	le.PutUint16(out[40:], synthFileHeaderSize)
	le.PutUint16(out[42:], uint16(chunks))
	le.PutUint32(out[120:], flags)
	return out
}
