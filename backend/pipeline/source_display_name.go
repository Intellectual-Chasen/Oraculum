package pipeline

import (
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// distinguisherSeparator は、区別する文字列の要素をつなぐ文字列である。図の表示名は最後の `/` の
// 後だけを描くため、`/` と `\` を使わない。
const distinguisherSeparator = ", "

// sharedNameCount は、区別する文字列を足す、同じ名前の plan の最少の件数である。
const sharedNameCount = 2

// DistinguishFileNames は、同じ FileName の plan が 2 件以上あるとき、その FileName に区別する
// 文字列を括弧で足した写しを返す。FileName が 1 件だけの plan は変えない。
//
// 区別する文字列は、取り込み元の親 directory のうち、同じ名前の plan の間で違う階層である。前と
// 後ろの共通の階層を外す。親 directory が同じ plan には、入力形式、案件、plan の位置 (1 起点) の
// 順に、違いが出る値を足す。結果に同じ FileName の plan を渡しても、名前は変わらない。
//
// FileName は表示に使う値であり、収集元の識別と原資料を開く処理は OriginPath と内容の sha256 を
// 使う。
//
// 既知の制限: 区別した表示名が、別の場所の実在の file 名と同じ文字列になることを防がない,
// 取り込みの外にある file 名を知る手段を持たない, 括弧を含む file 名の組を取り込む要求が出たとき
func DistinguishFileNames(plans []SourcePlan) []SourcePlan {
	distinguished := slices.Clone(plans)
	groups := make(map[string][]int)
	var names []string
	for index, plan := range plans {
		if _, seen := groups[plan.FileName]; !seen {
			names = append(names, plan.FileName)
		}
		groups[plan.FileName] = append(groups[plan.FileName], index)
	}
	for _, name := range names {
		members := groups[name]
		if len(members) < sharedNameCount {
			continue
		}
		labels := differingDirectories(plans, members)
		for _, qualifier := range []func(SourcePlan, int) string{
			func(plan SourcePlan, _ int) string { return string(plan.FormatKey) },
			func(plan SourcePlan, _ int) string {
				if plan.CaseId == nil {
					return ""
				}
				return *plan.CaseId
			},
			func(_ SourcePlan, index int) string { return strconv.Itoa(index + 1) },
		} {
			qualifyDuplicates(labels, members, func(index int) string { return qualifier(plans[index], index) })
		}
		for at, index := range members {
			distinguished[index].FileName = name + " (" + labels[at] + ")"
		}
	}
	return distinguished
}

// differingDirectories は、members の plan の親 directory から、前と後ろの共通の階層を外した階層を
// つないだ文字列を返す。残る階層が無い plan は "." である。
func differingDirectories(plans []SourcePlan, members []int) []string {
	parts := make([][]string, len(members))
	shortest := -1
	for at, index := range members {
		directory := filepath.ToSlash(filepath.Dir(plans[index].OriginPath))
		if directory != "." {
			parts[at] = strings.Split(directory, "/")
		}
		if shortest < 0 || len(parts[at]) < shortest {
			shortest = len(parts[at])
		}
	}
	prefix := 0
	for prefix < shortest && allEqual(parts, func(elements []string) string { return elements[prefix] }) {
		prefix++
	}
	suffix := 0
	for prefix+suffix < shortest &&
		allEqual(parts, func(elements []string) string { return elements[len(elements)-1-suffix] }) {
		suffix++
	}
	labels := make([]string, len(members))
	for at, elements := range parts {
		middle := elements[prefix : len(elements)-suffix]
		labels[at] = "."
		if len(middle) > 0 {
			labels[at] = strings.Join(middle, distinguisherSeparator)
		}
	}
	return labels
}

// qualifyDuplicates は、同じ文字列を持つ labels の組に、組の中で違いが出るときだけ qualifier の
// 値を足す。
func qualifyDuplicates(labels []string, members []int, qualifier func(int) string) {
	groups := make(map[string][]int)
	for at, label := range labels {
		groups[label] = append(groups[label], at)
	}
	for _, positions := range groups {
		if len(positions) < sharedNameCount {
			continue
		}
		values := make([][]string, len(positions))
		for i, at := range positions {
			values[i] = []string{qualifier(members[at])}
		}
		if allEqual(values, func(elements []string) string { return elements[0] }) {
			continue
		}
		for i, at := range positions {
			switch {
			case values[i][0] == "":
			case labels[at] == ".":
				// 親 directory が同じ組は、親の階層を持たない。違いの出る値だけを出す。
				labels[at] = values[i][0]
			default:
				labels[at] += distinguisherSeparator + values[i][0]
			}
		}
	}
}

// allEqual は、すべての要素から読んだ文字列が同じであるかを返す。
func allEqual(values [][]string, read func([]string) string) bool {
	for _, value := range values[1:] {
		if read(value) != read(values[0]) {
			return false
		}
	}
	return true
}
