package main

import (
	"math"
	"os"
	"path"
	"runtime/debug"
	"strconv"
	"strings"
)

// memoryLimitShare は、cgroup のメモリの上限のうち Go の runtime に与える割合である。
//
// 残りは runtime の外の確保 (stack、OS の page の端数) に残す。
//
// 既知の制限: 割合を固定の値に置く, runtime の外の確保の大きさは入力と実行環境で変わり、
// repo の test では測れない, 上限の中で OOM kill が起きたとき、または GC が所要の大半を占めたときに見直す
const memoryLimitShare = 0.85

// cgroupRoot は cgroup v2 の階層を置く directory である。
const cgroupRoot = "/sys/fs/cgroup"

// applyDefaultMemoryLimit は、GOMEMLIMIT を与えない起動で、process が属する cgroup の
// メモリの上限から Go の runtime のメモリの上限を決める。決めた上限の byte 数を返す。
// 上限を決めなかった起動は 0 を返す。
//
// **GOMEMLIMIT を与えた起動はその値を使う。** cgroup の上限が無い起動は runtime の既定のまま
// にする。
func applyDefaultMemoryLimit() int64 {
	if _, given := os.LookupEnv("GOMEMLIMIT"); given {
		return 0
	}
	membership, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return 0
	}
	limit, found := cgroupMemoryMax(string(membership), os.ReadFile)
	if !found {
		return 0
	}
	share := int64(float64(limit) * memoryLimitShare)
	debug.SetMemoryLimit(share)
	return share
}

// cgroupMemoryMax は、/proc/self/cgroup の内容 membership が指す cgroup v2 の階層を
// 根まで辿り、memory.max の最も小さい値を返す。found が偽になるのは、cgroup v2 の行が無いとき
// と、どの階層も上限を持たないときである。
func cgroupMemoryMax(membership string, readFile func(string) ([]byte, error)) (int64, bool) {
	var group string
	for line := range strings.Lines(membership) {
		if rest, isV2 := strings.CutPrefix(strings.TrimSpace(line), "0::"); isV2 {
			group = rest
		}
	}
	if group == "" {
		return 0, false
	}
	smallest := int64(math.MaxInt64)
	for dir := path.Clean(group); ; dir = path.Dir(dir) {
		content, err := readFile(path.Join(cgroupRoot, dir, "memory.max"))
		if err == nil {
			if value, parseErr := strconv.ParseInt(strings.TrimSpace(string(content)), 10, 64); parseErr == nil && value > 0 {
				smallest = min(smallest, value)
			}
		}
		if dir == "/" {
			break
		}
	}
	return smallest, smallest != math.MaxInt64
}
