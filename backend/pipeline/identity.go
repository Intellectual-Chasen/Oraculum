package pipeline

// RunManifestEntry は解析実行 1 回に入る収集元 1 件の識別である。
// analysisRunRef の材料になる。
type RunManifestEntry struct {
	// SourceId は取り込み 1 件を指す値である。
	SourceId string
	// ParserVersion はその収集元を読んだパーサーのバージョンである。
	ParserVersion string
	// CaseId は収集元に付けた案件である。案件を区別しない取り込みでは空である。
	// 案件は導く関係を変えるので、同じ収集元でも案件が違えば別の解析実行になる。
	CaseId string
}

// IdentityMinter は取り込みの識別子を発行する port である。
//
// 値の生成規則は本 interface の実装が決める。本 interface が定めるのは、取り込みごとに異なる値に
// なることだけである。
//
// core に置かないのは、実装が乱数と hash を必要とし、core が I/O を持てないためである。
type IdentityMinter interface {
	// SourceId は取り込み 1 件を指す値を作る。
	// 同じ originPath と contentSha256 でも、importOrdinal が異なれば異なる値を返す。
	SourceId(originPath, contentSha256 string, importOrdinal int64) (string, error)
	// ParserVersion はパーサーのバージョンを作る。
	// revision が空のとき、パーサーのバージョンを特定できないことを表す値を返す。
	ParserVersion(identity ParserIdentity, revision, settingsDigest string) (string, error)
	// AnalysisRunRef は解析実行 1 回を指す値を作る。
	// manifest の並び順に依らず同じ値を返す。同じ SourceId が 2 件あるときは error を返す。
	AnalysisRunRef(manifest []RunManifestEntry) (string, error)
}

// ImportOrdinalSource は取り込みの通番を発行する port である。
//
// 同じ原資料を 2 回取り込むと 2 つの異なる sourceId になる。本 port はその材料を出す。
// 通番の定義元は保存先が持つが、保存先は未実装である。
type ImportOrdinalSource interface {
	// Next は originPath と contentSha256 の組に対する次の通番を返す。
	Next(originPath, contentSha256 string) (int64, error)
}
