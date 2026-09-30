package pipeline

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"
)

// DigestMinter は取り込みと解析実行の識別子を発行する。
// 各要素の byte 数を固定幅 8 byte big-endian の符号なし整数で前置する。
// 区切り byte だけの連結は、要素内の区切り byte により異なる入力が同じ値になるため禁止する。
type DigestMinter struct{}

// SourceId は取得元、内容の識別、1 起点の取り込み通番から識別子を作る。
func (DigestMinter) SourceId(originPath, contentSha256 string, importOrdinal int64) (string, error) {
	if importOrdinal < 1 {
		return "", errors.New("mint source identity: import ordinal must be positive")
	}
	return identityDigest([]string{originPath, contentSha256, strconv.FormatInt(importOrdinal, 10)}), nil
}

// ParserVersion はパーサー、対応仕様、revision、設定の識別を長さ前置でハッシュしたパーサーのバージョンを返す。
// revision が空の場合は unknown をハッシュの材料にし、返却値に unknown: を前置する。
func (DigestMinter) ParserVersion(identity ParserIdentity, revision, settingsDigest string) (string, error) {
	prefix := ""
	// 既知の制限: vcs.revision を読めない環境では別の commit のコードの結果が同じ parserVersion になる, 実運用で発生頻度を測る相手がまだ無く未測定, 保存した結果を parserVersion で突き合わせるときに見直す。
	if revision == "" {
		revision = "unknown"
		prefix = "unknown:"
	}
	return prefix + identityDigest([]string{identity.ParserID, identity.SupportedFormatVersion, revision, settingsDigest}), nil
}

// caseScopedRunMarker は案件を区別する解析実行の材料の先頭に置く要素である。
// 案件を区別しない実行の材料は収集元ごとに 2 要素、区別する実行は 3 要素を並べる。先頭の
// 要素で 2 つの並びを分け、要素数が偶然揃った 2 つの材料を同じ値にしない。
const caseScopedRunMarker = "case-scoped-run"

// AnalysisRunRef は収集元順に揃えた manifest の識別子を返す。空の manifest も識別する。
// 案件を区別しない manifest の値は、案件を導入する前の規則と同じ値である。
func (DigestMinter) AnalysisRunRef(manifest []RunManifestEntry) (string, error) {
	entries := slices.Clone(manifest)
	slices.SortFunc(entries, func(a, b RunManifestEntry) int { return strings.Compare(a.SourceId, b.SourceId) })
	caseScoped := slices.ContainsFunc(entries, func(entry RunManifestEntry) bool { return entry.CaseId != "" })
	parts := make([]string, 0, 1+3*len(entries))
	if caseScoped {
		parts = append(parts, caseScopedRunMarker)
	}
	for i, entry := range entries {
		if i > 0 && entry.SourceId == entries[i-1].SourceId {
			return "", errors.New("mint analysis run reference: duplicate source identity")
		}
		if !caseScoped {
			parts = append(parts, entry.SourceId, entry.ParserVersion)
			continue
		}
		if entry.CaseId == "" {
			return "", errors.New("mint analysis run reference: some sources carry a case and others do not")
		}
		parts = append(parts, entry.SourceId, entry.ParserVersion, entry.CaseId)
	}
	return identityDigest(parts), nil
}

func identityDigest(parts []string) string {
	var input []byte
	for _, part := range parts {
		input = binary.BigEndian.AppendUint64(input, uint64(len(part)))
		input = append(input, part...)
	}
	digest := sha256.Sum256(input)
	return hex.EncodeToString(digest[:])
}

type inMemoryOrdinals struct {
	mu     sync.Mutex
	counts map[[2]string]int64
}

// NewInMemoryOrdinals は取得元と内容の組ごとに 1 起点で数える通番発行器を返す。
//
// 通番は実行の間だけ保持する。調査を開かない起動は、この発行器で実行ごとに通番を振る。
// 調査を開く起動は、調査に保存した通番を Investigation.Ordinals で使う。
func NewInMemoryOrdinals() ImportOrdinalSource {
	return &inMemoryOrdinals{counts: make(map[[2]string]int64)}
}

func (s *inMemoryOrdinals) Next(originPath, contentSha256 string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := [2]string{originPath, contentSha256}
	if s.counts[key] == math.MaxInt64 {
		return 0, errors.New("issue import ordinal: exhausted")
	}
	s.counts[key]++
	return s.counts[key], nil
}
