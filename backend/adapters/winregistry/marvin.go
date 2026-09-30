package winregistry

import (
	"encoding/binary"
	"math"
	"math/bits"
)

// logEntrySeed は log entry の Hash-1 と Hash-2 を求める Marvin32 の seed である。
const logEntrySeed uint64 = 0x82EF4D887A4E55C5

// Marvin32 の 1 語の混合で回す bit 数と、残りの byte に付ける終端の値。
const (
	marvinRotate1   = 20
	marvinRotate2   = 9
	marvinRotate3   = 27
	marvinRotate4   = 19
	marvinFinalMark = 0x80
	// marvinHalfBits は seed と hash を lo と hi に分ける bit 数である。
	marvinHalfBits = 32
)

// marvin32 は data の Marvin32 hash を返す。seed の下位 32 bit が状態の lo、上位が hi である。
func marvin32(data []byte, seed uint64) uint64 {
	lo, hi := uint32(seed&math.MaxUint32), uint32(seed>>marvinHalfBits) // #nosec G115 -- 下位と上位の 32 bit へ分ける。
	mix := func(value uint32) {
		lo += value
		hi ^= lo
		lo = bits.RotateLeft32(lo, marvinRotate1) + hi
		hi = bits.RotateLeft32(hi, marvinRotate2) ^ lo
		lo = bits.RotateLeft32(lo, marvinRotate3) + hi
		hi = bits.RotateLeft32(hi, marvinRotate4)
	}
	for len(data) >= 4 {
		mix(binary.LittleEndian.Uint32(data))
		data = data[4:]
	}
	// 残りの 0 から 3 byte を終端と一緒に 1 語にする。
	final := uint32(marvinFinalMark)
	for i := len(data) - 1; i >= 0; i-- {
		final = final<<8 | uint32(data[i])
	}
	mix(final)
	mix(0)
	return uint64(hi)<<marvinHalfBits | uint64(lo)
}
