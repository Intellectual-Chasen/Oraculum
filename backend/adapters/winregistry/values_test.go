package winregistry

import "testing"

func TestValueTextKeepsBytesThatTextWouldLose(t *testing.T) {
	for _, tc := range []struct {
		name      string
		valueType uint32
		data      []byte
		want      string
	}{
		{"string", regSZ, utf16Bytes("abc"), "abc"},
		{"string without NUL", regSZ, []byte{'a', 0, 'b', 0}, "ab"},
		{"odd byte NUL", regSZ, []byte{'a', 0, 0}, "a"},
		{"odd byte data", regSZ, []byte{'a', 0, 'b'}, "610062"},
		{"NUL inside", regSZ, []byte{'a', 0, 0, 0, 'b', 0}, "610000006200"},
		{"lone surrogate", regSZ, []byte{0x00, 0xD8, 'a', 0}, "00d86100"},
		{"surrogate pair", regSZ, []byte{0x3D, 0xD8, 0x00, 0xDE}, "\U0001F600"},
		{"multi", regMultiSZ, append(append(utf16Bytes("a"), utf16Bytes("")...), utf16Bytes("b")...), "a\n\nb"},
		{"dword", regDword, []byte{0x5A, 0, 0, 0}, "90 (0x0000005A)"},
		{"dword big endian", regDwordBigEndian, []byte{0, 0, 0, 0x5A}, "90 (0x0000005A)"},
		{"dword wrong length", regDword, []byte{0x5A, 0}, "5a00"},
		{"qword", regQword, []byte{0, 0, 0, 0, 0x5A, 0, 0, 0}, "386547056640 (0x0000005A00000000)"},
		{"binary", regBinary, []byte{0xAB}, "ab"},
		{"unknown type", 0x20, []byte{0xAB}, "ab"},
	} {
		if got := valueText(tc.valueType, tc.data); got != tc.want {
			t.Errorf("%s: valueText = %q, want %q", tc.name, got, tc.want)
		}
	}
	if got := typeName(0x20); got != "0x00000020" {
		t.Errorf("typeName = %q", got)
	}
}

func TestNameTextReadsLatin1AndUTF16(t *testing.T) {
	if got := nameText([]byte{'A', 0xE9}, true); got != "Aé" {
		t.Errorf("compressed name = %q", got)
	}
	if got := nameText([]byte{'A', 0, 0xE9, 0}, false); got != "Aé" {
		t.Errorf("UTF-16 name = %q", got)
	}
}

func TestMarvin32MatchesKnownHashes(t *testing.T) {
	sequence := func(n int) []byte {
		data := make([]byte, n)
		for i := range data {
			data[i] = byte(i % 40)
		}
		return data
	}
	for _, tc := range []struct {
		data []byte
		want uint64
	}{
		{nil, 0xB39EFCA403966E08},
		{[]byte("abcd"), 0x20094FB11A6CD086},
		{sequence(32), 0xC64FEEE425C2E24C},
		{sequence(120), 0x68A485E9386E9123},
	} {
		if got := marvin32(tc.data, logEntrySeed); got != tc.want {
			t.Errorf("marvin32(%d bytes) = %#016x, want %#016x", len(tc.data), got, tc.want)
		}
	}
}
