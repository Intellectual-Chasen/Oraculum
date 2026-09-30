// in-package test: URL の path に分けて送られた番号付きの断片をつなぎ、復号する条件を確かめる。
package pipeline

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// syntheticZip は 2 つの file を持つ ZIP の byte 列を返す。
func syntheticZip(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, content := range map[string]string{"a.txt": "first synthetic file", "b.txt": "second synthetic file"} {
		file, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte(strings.Repeat(content, 8))); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

// fragmentLine は、接続元 IP から files.example.test へ path を要求した Squid の 1 行である。
func fragmentLine(path string) string {
	return fragmentLineWithStatus(path, "200")
}

// fragmentLineWithStatus は、HTTP の状態が status の fragmentLine である。
func fragmentLineWithStatus(path, status string) string {
	return `192.0.2.10 - - [03/Feb/2001:04:10:00 +0000] "GET http://files.example.test/` + path +
		` HTTP/1.1" ` + status + ` 3 "-" "agent" TCP_MISS:HIER_DIRECT` + "\n"
}

// 同じ番号の行が複数あるとき、状態が成功の行をつなぐ。成功の行が無い番号は欠けになり、
// 状態が成功でない行はつながない。
func TestUrlFragmentJoinAdoptsOnlyTheSuccessfulLines(t *testing.T) {
	// "QUJD" と "REVG" をつないだ文字列は "ABCDEF" の base64url である。
	join := joinOfDocument(t, fragmentLineWithStatus("0/QUJD", "403")+fragmentLine("0/QUJD")+
		fragmentLineWithStatus("1/WFla", "404")+fragmentLine("1/REVG"))
	if len(join.Segments) != 1 {
		t.Fatalf("segments = %d, want 1", len(join.Segments))
	}
	segment := join.Segments[0]
	if segment.Decoded == nil {
		t.Fatalf("the segment is not decoded: %q (%+v)", segment.DecodeFailure, segment)
	}
	sum := sha256.Sum256([]byte("ABCDEF"))
	if segment.Decoded.Sha256 != hex.EncodeToString(sum[:]) {
		t.Errorf("decoded sha256 = %s, want the one of the successful lines", segment.Decoded.Sha256)
	}
	// 断片は番号の順、同じ番号の中では根拠の並びである。成功の行は 2 番目と 4 番目である。
	for index, fragment := range segment.Fragments {
		if fragment.Adopted != (index%2 == 1) {
			t.Errorf("fragment %d (number %d) adopted = %v", index, fragment.Number, fragment.Adopted)
		}
	}
	if segment.ConflictingDuplicateCount != 0 {
		t.Errorf("a line that is not adopted counts as a conflicting duplicate: %+v", segment)
	}

	withoutSuccess := joinOfDocument(t, fragmentLine("0/QUJD")+fragmentLineWithStatus("1/REVG", "403"))
	if got := withoutSuccess.Segments[0]; got.MissingNumberCount != 1 ||
		got.DecodeFailure != core.UrlFragmentDecodeFailureMissingNumbers || got.Fragments[1].Adopted {
		t.Errorf("a number without a successful line = %+v, want a missing number", got)
	}
	// 状態の欄が値の不在の文字列を持つ行はつながない。
	absentStatus := joinOfDocument(t, fragmentLine("0/QUJD")+fragmentLineWithStatus("1/REVG", "-"))
	if got := absentStatus.Segments[0]; got.MissingNumberCount != 1 || got.Fragments[1].Adopted ||
		got.Fragments[1].HttpStatusCode != "-" {
		t.Errorf("a line with the absent status = %+v, want a missing number", got)
	}
}

// numberedLines は文字列を size ごとに分け、番号を付けた行にする。repeat の番号は 2 回送る。
func numberedLines(token string, size int, repeat map[int]bool) []string {
	var lines []string
	for number := 0; number*size < len(token); number++ {
		part := token[number*size : min(len(token), (number+1)*size)]
		line := fragmentLine(strconv.Itoa(number) + "/" + part)
		lines = append(lines, line)
		if repeat[number] {
			lines = append(lines, line)
		}
	}
	return lines
}

// nativeLine は、組み込みの squid の並びで、接続元 IP から files.example.test へ
// path を要求した 1 行である。改行を持たない。
func nativeLine(second int, status, path string) string {
	return strconv.Itoa(978307200+second) + ".000      1 192.0.2.10 TCP_MISS/" + status +
		" 3 GET http://files.example.test/" + path + " - HIER_DIRECT/192.0.2.20 text/html"
}

// Squid の 1 行の上限で切れた行は、取り込めなかったレコードとして記録し、読めた欄を持つ
// 要求の記録としても取り込む。切れた行の番号は、その行の位置と一緒に送信の回の欠けに出る。
func TestTruncatedSquidLineIsKeptAsARequestAndCountsAsAMissingNumber(t *testing.T) {
	const lineLimit = 8191
	head := nativeLine(1, "400", "1/")
	head = head[:strings.Index(head, "/1/")+3]
	truncated := head + strings.Repeat("A", lineLimit-len(head))
	document := nativeLine(0, "200", "0/QUJD") + "\n" + truncated + "\n" + nativeLine(2, "200", "2/REVG") + "\n"

	source := scanIndexSource(t, NewTestSquidLogFormatParser("squid"), "access.log", SquidLogFormatKey, document)
	if len(source.Records) != 3 || len(source.Failures) != 1 {
		t.Fatalf("records = %d failures = %d, want 3 and 1", len(source.Records), len(source.Failures))
	}
	failure := source.Failures[0].Failure
	firstLineBytes := int64(len(nativeLine(0, "200", "0/QUJD")) + 1)
	if !failure.RecordTruncated || failure.RecordRef == nil || *failure.RecordRef.LineNumber != 2 ||
		*failure.RecordRef.ByteOffset != firstLineBytes || *failure.RecordRef.ByteLength != lineLimit {
		t.Errorf("failure = %+v ref %+v, want a truncated line 2 with its byte range", failure, failure.RecordRef)
	}
	if succeeded, _ := source.Counts.Count(core.ImportCategorySucceeded); succeeded != 2 {
		t.Errorf("succeeded = %d, want the truncated line counted as failed", succeeded)
	}
	var target *core.RecordField
	for index, field := range source.Records[1].Semantics.Fields {
		if field.Semantic == core.SemanticKeyHttpRequestUrl {
			target = &source.Records[1].Semantics.Fields[index]
		}
	}
	if target == nil || target.Text.ValueState != core.ValueStateTruncated {
		t.Fatalf("the url of the truncated line = %+v, want a truncated value", target)
	}

	result, err := newImportResult([]scannedSource{source},
		[]core.ImportStatus{settleStatus(t, source, "source-access.log")}, "run", indexRawTextRef)
	if err != nil {
		t.Fatal(err)
	}
	if status := result.publications[0].status.PublicationState; status != core.PublicationStatePublishedPartial {
		t.Fatalf("publication = %q, want published_partial", status)
	}
	graph := NewGraph(result, AllMatchConditions())
	edges := edgesOfKind(graph, core.EdgeKindHttpRequest)
	if len(edges) != 1 {
		t.Fatalf("http_request edges = %d, want 1", len(edges))
	}
	join, _ := graph.UrlFragmentJoin(edges[0].id, EdgeEvidenceFilter{})
	if err := join.Validate(); err != nil {
		t.Fatal(err)
	}
	segment := join.Segments[0]
	cut := segment.Fragments[1]
	if len(join.Segments) != 1 || segment.MissingNumberCount != 1 || !cut.Truncated || cut.Adopted ||
		cut.HttpStatusCode != "400" || *cut.RecordRef.LineNumber != 2 {
		t.Errorf("segment = %+v, want the truncated line 2 as the missing number 1", segment)
	}
}

func joinOfDocument(t *testing.T, document string) core.UrlFragmentJoin {
	t.Helper()
	graph := NewGraph(sessionSourcesResult(t, sessionSource{
		name: "access.log", format: SquidFormatKey, document: document,
	}), AllMatchConditions())
	edges := edgesOfKind(graph, core.EdgeKindHttpRequest)
	if len(edges) != 1 {
		t.Fatalf("http_request edges = %d, want 1", len(edges))
	}
	join, found := graph.UrlFragmentJoin(edges[0].id, EdgeEvidenceFilter{})
	if !found {
		t.Fatal("the edge is not found")
	}
	if err := join.Validate(); err != nil {
		t.Fatalf("the join violates the contract: %v", err)
	}
	return join
}

// 番号が小さくなった点で送信を区切り、最後の回を番号の順につないで ZIP として読む。
// 同じ番号の再送は同じ回に残り、番号を持たない行はつながない。
func TestUrlFragmentJoinDecodesTheLastTransferAsZip(t *testing.T) {
	archive := syntheticZip(t)
	token := base64.RawURLEncoding.EncodeToString(archive)
	var document strings.Builder
	// 1 回目: 小さい断片で途中まで送る。
	for _, line := range numberedLines(token[:len(token)/2], 16, nil)[:4] {
		document.WriteString(line)
	}
	document.WriteString(fragmentLine("notnumbered"))
	// 2 回目: 0 から最後まで送り、番号 1 を再送する。
	for _, line := range numberedLines(token, 40, map[int]bool{1: true}) {
		document.WriteString(line)
	}

	join := joinOfDocument(t, document.String())
	if join.UnnumberedRecordCount != 1 {
		t.Errorf("unnumbered = %d, want 1", join.UnnumberedRecordCount)
	}
	if len(join.Segments) != 2 {
		t.Fatalf("segments = %d, want 2", len(join.Segments))
	}
	last := join.Segments[1]
	if last.DuplicateCount != 1 || last.ConflictingDuplicateCount != 0 || last.MissingNumberCount != 0 {
		t.Errorf("counts = %+v", last)
	}
	if last.Decoded == nil {
		t.Fatalf("the last segment is not decoded: %q", last.DecodeFailure)
	}
	sum := sha256.Sum256(archive)
	if last.Encoding != core.UrlFragmentEncodingBase64Url || last.Decoded.Sha256 != hex.EncodeToString(sum[:]) ||
		last.Decoded.ByteCount != int64(len(archive)) || last.Decoded.ContentType != "application/zip" {
		t.Errorf("decoded = %+v encoding %q", last.Decoded, last.Encoding)
	}
	listing := last.Decoded.Zip
	if listing == nil || !listing.Readable || listing.Integrity != core.ZipIntegrityPassed || listing.EntryCount != 2 {
		t.Fatalf("zip = %+v", listing)
	}
	names := []string{listing.Entries[0].Name, listing.Entries[1].Name}
	slices.Sort(names)
	if names[0] != "a.txt" || names[1] != "b.txt" {
		t.Errorf("entry names = %v", names)
	}
	notAdopted := 0
	for index, fragment := range last.Fragments {
		if index > 0 && fragment.Number < last.Fragments[index-1].Number {
			t.Fatalf("fragments are not in the number order: %+v", last.Fragments)
		}
		if !fragment.Adopted {
			notAdopted++
		}
		if fragment.RecordRef.LineNumber == nil {
			t.Fatal("a fragment carries no line number")
		}
	}
	if notAdopted != 1 {
		t.Errorf("not adopted = %d, want 1", notAdopted)
	}
	// 途中で切れた 1 回目は ZIP として読めないか、復号しない。
	first := join.Segments[0]
	if first.Decoded != nil && first.Decoded.Zip != nil && first.Decoded.Zip.Integrity == core.ZipIntegrityPassed {
		t.Errorf("the truncated segment passes the zip check: %+v", first.Decoded)
	}
}

// 欠番と、同じ番号で文字列の違う行と、前の番号の再送は、復号しないか回を区切る。
func TestUrlFragmentJoinWithholdsTheDecodingItCannotOrder(t *testing.T) {
	for name, test := range map[string]struct {
		lines    []string
		segments int
		failure  core.UrlFragmentDecodeFailure
	}{
		"欠番": {
			lines:    []string{fragmentLine("0/QUJD"), fragmentLine("2/REVG")},
			segments: 1, failure: core.UrlFragmentDecodeFailureMissingNumbers,
		},
		"文字列の違う重複": {
			lines:    []string{fragmentLine("0/QUJD"), fragmentLine("0/REVG")},
			segments: 1, failure: core.UrlFragmentDecodeFailureConflictingDuplicates,
		},
		"両方の字母": {
			lines:    []string{fragmentLine("0/QU-D"), fragmentLine("1/RE+G")},
			segments: 1, failure: core.UrlFragmentDecodeFailureAlphabetUndetermined,
		},
		"途中の埋め草": {
			lines:    []string{fragmentLine("0/QQ=="), fragmentLine("1/QQ==")},
			segments: 1, failure: core.UrlFragmentDecodeFailureAlphabetUndetermined,
		},
		"前の番号の再送は回を区切る": {
			lines:    []string{fragmentLine("0/QUJD"), fragmentLine("1/REVG"), fragmentLine("0/QUJD")},
			segments: 2,
		},
	} {
		t.Run(name, func(t *testing.T) {
			join := joinOfDocument(t, strings.Join(test.lines, ""))
			if len(join.Segments) != test.segments {
				t.Fatalf("segments = %d, want %d", len(join.Segments), test.segments)
			}
			if got := join.Segments[0].DecodeFailure; got != test.failure {
				t.Errorf("failure = %q, want %q", got, test.failure)
			}
		})
	}
}

func TestNumberedFragmentOfReadsTheFirstPathElement(t *testing.T) {
	field := func(raw string) *core.RecordField {
		value := presentText(raw)
		return &core.RecordField{Name: "requestTarget", Text: &value}
	}
	for raw, want := range map[string]struct {
		number   int64
		token    string
		numbered bool
	}{
		"http://files.example.test/12/abc":       {12, "abc", true},
		"/007/abc":                               {7, "abc", true},
		"http://files.example.test/3/ab/cd?x":    {3, "ab/cd", true},
		"http://files.example.test/abc":          {numbered: false},
		"http://files.example.test/x1/abc":       {numbered: false},
		"http://files.example.test/+1/abc":       {numbered: false},
		"http://files.example.test/1/":           {numbered: false},
		"http://files.example.test/4294967296/a": {numbered: false},
	} {
		fragment, numbered := numberedFragmentOf(field(raw))
		if numbered != want.numbered || numbered && (fragment.number != want.number || fragment.token != want.token) {
			t.Errorf("%q = %+v %v, want %+v", raw, fragment, numbered, want)
		}
	}
	if _, numbered := numberedFragmentOf(nil); numbered {
		t.Error("a record without the url is numbered")
	}
	// 途中で切れた URL は、番号の後ろの `/` まで残っていれば、文字列が空でも番号を読む。
	truncated := func(raw string) *core.RecordField {
		value := core.RawAndNormalized{RawText: &raw, ValueState: core.ValueStateTruncated}
		return &core.RecordField{Name: "requestTarget", Text: &value}
	}
	if fragment, numbered := numberedFragmentOf(truncated("http://files.example.test/5/")); !numbered ||
		fragment.number != 5 || !fragment.truncated || fragment.joinable() {
		t.Errorf("a truncated url = %+v %v, want the number 5 that is not joinable", fragment, numbered)
	}
	// 百分率符号化の途中で切れた文字列も番号を読む。
	for _, raw := range []string{"http://files.example.test/6/AB%2", "http://files.example.test/6/AB%"} {
		if fragment, numbered := numberedFragmentOf(truncated(raw)); !numbered || fragment.number != 6 {
			t.Errorf("%q = %+v %v, want the number 6", raw, fragment, numbered)
		}
	}
	if _, numbered := numberedFragmentOf(truncated("http://files.example.test/5")); numbered {
		t.Error("a url truncated inside the number is numbered")
	}
}

func TestFragmentAlphabetOf(t *testing.T) {
	for text, want := range map[string]struct {
		encoding core.UrlFragmentEncoding
		known    bool
	}{
		"ab-_":  {core.UrlFragmentEncodingBase64Url, true},
		"ab+/":  {core.UrlFragmentEncodingBase64, true},
		"abc9":  {core.UrlFragmentEncodingBase64Url, true},
		"a-+b":  {known: false},
		"a%2Fb": {known: false},
	} {
		encoding, known := fragmentAlphabetOf(text)
		if encoding != want.encoding || known != want.known {
			t.Errorf("%q = %q %v, want %+v", text, encoding, known, want)
		}
	}
}

// 壊れた ZIP と、展開の上限を超える ZIP と、UTF-8 でない名前を区別して返す。
func TestZipListingOfSeparatesTheStates(t *testing.T) {
	archive := syntheticZip(t)
	broken := bytes.Clone(archive)
	// 最初の entry の中身の 1 byte を書き換え、CRC を合わなくする。
	broken[40] ^= 0xff
	if got := zipListingOf(broken); !got.Readable || got.Integrity != core.ZipIntegrityFailed {
		t.Errorf("broken = %+v", got)
	}
	if got := zipListingOf(archive[:len(archive)/2]); got.Readable || len(got.Entries) != 0 {
		t.Errorf("truncated = %+v", got)
	}

	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	large, err := writer.CreateHeader(&zip.FileHeader{Name: "large\xff.bin", Method: zip.Deflate})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := large.Write(make([]byte, maxZipInflatedBytes+1)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	got := zipListingOf(buffer.Bytes())
	if got.Integrity != core.ZipIntegrityNotChecked {
		t.Errorf("large integrity = %q", got.Integrity)
	}
	if got.Entries[0].NameHex != hex.EncodeToString([]byte("large\xff.bin")) || !strings.HasPrefix(got.Entries[0].Name, "large") {
		t.Errorf("non utf-8 name = %+v", got.Entries[0])
	}
}
