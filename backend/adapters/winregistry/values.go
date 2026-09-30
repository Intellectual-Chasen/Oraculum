package winregistry

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// 数値の型の data の byte 数。
const (
	dwordSize = 4
	qwordSize = 8
)

// 値の型。
const (
	regNone             = 0
	regSZ               = 1
	regExpandSZ         = 2
	regBinary           = 3
	regDword            = 4
	regDwordBigEndian   = 5
	regLink             = 6
	regMultiSZ          = 7
	regResourceList     = 8
	regFullResource     = 9
	regResourceRequired = 10
	regQword            = 11
)

var typeNames = map[uint32]string{
	regNone: "REG_NONE", regSZ: "REG_SZ", regExpandSZ: "REG_EXPAND_SZ", regBinary: "REG_BINARY",
	regDword: "REG_DWORD", regDwordBigEndian: "REG_DWORD_BIG_ENDIAN", regLink: "REG_LINK",
	regMultiSZ: "REG_MULTI_SZ", regResourceList: "REG_RESOURCE_LIST",
	regFullResource: "REG_FULL_RESOURCE_DESCRIPTOR", regResourceRequired: "REG_RESOURCE_REQUIREMENTS_LIST",
	regQword: "REG_QWORD",
}

// typeName は値の型の名前である。定義に無い型は 16 進の数である。
func typeName(valueType uint32) string {
	if name, ok := typeNames[valueType]; ok {
		return name
	}
	return fmt.Sprintf("0x%08X", valueType)
}

// valueText は値の data を文字列にする。
//
// 文字列の型は UTF-16LE を読み、末尾の NUL を除く。MULTI_SZ は要素を改行で繋ぐ。
// DWORD と QWORD は符号無しの 10 進と、括弧の中の 16 進である。**文字列にすると byte 列を
// 失う data は 16 進で書く。** 文字列の途中に NUL を持つ data、UTF-16 として読めない data、
// 型の長さと合わない数値、ほかの型である。
func valueText(valueType uint32, data []byte) string {
	switch valueType {
	case regSZ, regExpandSZ, regLink:
		if text, ok := utf16Text(data); ok && !strings.ContainsRune(text, 0) {
			return text
		}
	case regMultiSZ:
		if text, ok := utf16Text(data); ok {
			return strings.ReplaceAll(text, "\x00", "\n")
		}
	case regDword, regDwordBigEndian:
		if len(data) == dwordSize {
			value := binary.LittleEndian.Uint32(data)
			if valueType == regDwordBigEndian {
				value = binary.BigEndian.Uint32(data)
			}
			return fmt.Sprintf("%d (0x%08X)", value, value)
		}
	case regQword:
		if len(data) == qwordSize {
			value := binary.LittleEndian.Uint64(data)
			return fmt.Sprintf("%d (0x%016X)", value, value)
		}
	}
	return hex.EncodeToString(data)
}

// utf16Text は UTF-16LE の data を、末尾の NUL を除いた文字列にする。ok が偽になるのは、
// 奇数の byte 数の最後の byte が 0 でないときと、対にならない surrogate を持つときである。
func utf16Text(data []byte) (string, bool) {
	if len(data)%2 == 1 {
		if data[len(data)-1] != 0 {
			return "", false
		}
		data = data[:len(data)-1]
	}
	units := make([]uint16, len(data)/2)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(data[2*i:])
	}
	for len(units) > 0 && units[len(units)-1] == 0 {
		units = units[:len(units)-1]
	}
	for i := 0; i < len(units); i++ {
		switch {
		case utf16.IsSurrogate(rune(units[i])) && units[i] < 0xDC00 && i+1 < len(units) &&
			units[i+1] >= 0xDC00 && units[i+1] <= 0xDFFF:
			i++
		case utf16.IsSurrogate(rune(units[i])):
			return "", false
		}
	}
	return string(utf16.Decode(units)), true
}

// nameText は key と値の名前を読む。compressed のときは 1 byte 1 文字 (Latin-1)、それ以外は
// UTF-16LE である。
func nameText(data []byte, compressed bool) string {
	if !compressed {
		units := make([]uint16, len(data)/2)
		for i := range units {
			units[i] = binary.LittleEndian.Uint16(data[2*i:])
		}
		return string(utf16.Decode(units))
	}
	if !bytes.ContainsFunc(data, func(r rune) bool { return r >= utf8.RuneSelf }) {
		return string(data)
	}
	runes := make([]rune, len(data))
	for i, b := range data {
		runes[i] = rune(b)
	}
	return string(runes)
}
