package markii_test

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/markii"
)

// FuzzTokenizeRecord は任意の byte 列に対する 4 つの性質を確かめる。
//
// 1. panic しない
// 2. 失敗しなかったレコードは、field 列から組み直すと原文に戻る
// 3. 失敗と成功が同時に立たない
// 4. 位置の値がレコードの範囲に収まる
//
// corpus は testdata/fuzz/FuzzTokenizeRecord/ が持つ。crasher を見つけたら、直した後も
// corpus に残す。
func FuzzTokenizeRecord(f *testing.F) {
	for _, seed := range []string{
		fullShapeLine,
		escapedQuoteLine,
		naiveGrepLine,
		absenceLine,
		zeroValueLine,
		duplicateKeyLine,
		quotedSrcIPLine,
		unquotedSrcIPLine,
		// 失敗する形。
		`02/01/2000 10:11:12.181 +0900 cmd="unterminated`,
		`02/01/2000 10:11:12.181 +0900 =net`,
		`02/01/2000 10:11:12.181 +0900 evt`,
		`02/01/2000 10:11:12.181 +0900Xevt=net`,
		`short`,
		``,
		// 多 byte 文字を持つ形。
		`02/01/2000 10:11:12.181 +0900 winTitle="メモ帳" evt=ps`,
		// 改行を 2 つ持つ形。
		orderLineA + "\r\n" + orderLineB,
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, source string) {
		var reader markii.Reader
		reader.Reset(strings.NewReader(source))
		for {
			record, failure, err := reader.Next()
			if errors.Is(err, io.EOF) {
				return
			}
			if err != nil {
				// 読み込みの失敗では診断が付く。
				if failure == nil {
					t.Fatalf("a read failure must carry a diagnosis: %v", err)
				}
				return
			}

			rawText := record.RawText()

			// 4. 位置の値がレコードの範囲に収まる。
			if problem := offsetProblemOf(record); problem != "" {
				t.Fatal(problem)
			}

			if failure != nil {
				// 3. 失敗したレコードは、原文と位置を持つ。
				if failure.LineNumber == nil || *failure.LineNumber != record.LineNumber() {
					t.Fatal("the failure must carry the line number of the record")
				}
				continue
			}

			// 2. 失敗しなかったレコードは組み直すと原文に戻る。
			restored, hasHeader := restoreOf(record)
			if !hasHeader {
				t.Fatalf("a record without a failure must carry its header: %q", rawText)
			}
			if restored != rawText {
				t.Fatalf("restored = %q, want %q", restored, rawText)
			}
		}
	})
}
