package royalts_test

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/royalts"
)

// synthesized Royal TS 文書。RFC 5737 の文書用範囲と example.test 系の文字列を使う。
// CredentialPassword は実在の暗号文の形を保つダミー値であり、復号鍵を持たない値である。
const singleConnectionDocument = `<?xml version="1.0" encoding="utf-8"?>
<RTSZDocument>
  <RoyalDocument>
    <RoyalFolder>
      <Name>Connections</Name>
      <RoyalRDSConnection>
        <ID>00000000-0000-0000-0000-000000000001</ID>
        <Name>Example RDP</Name>
        <URI>host01.example.test</URI>
        <CredentialUsername>exampleuser</CredentialUsername>
        <CredentialPassword>AQAAANCMnd8BFdERjHoAwE_example_ciphertext</CredentialPassword>
      </RoyalRDSConnection>
    </RoyalFolder>
  </RoyalDocument>
</RTSZDocument>
`

func TestReaderReadsSingleConnection(t *testing.T) {
	var reader royalts.Reader
	reader.Reset(strings.NewReader(singleConnectionDocument))
	connection, failure, err := reader.Next()
	if err != nil {
		t.Fatalf("Next() returned error %v", err)
	}
	if failure != nil {
		t.Fatalf("Next() returned failure %+v", failure)
	}
	if name, ok := connection.Name(); !ok || name != "Example RDP" {
		t.Errorf("Name() = %q, %v, want %q, true", name, ok, "Example RDP")
	}
	if uri, ok := connection.URI(); !ok || uri != "host01.example.test" {
		t.Errorf("URI() = %q, %v, want %q, true", uri, ok, "host01.example.test")
	}
	if user, ok := connection.CredentialUsername(); !ok || user != "exampleuser" {
		t.Errorf("CredentialUsername() = %q, %v, want %q, true", user, ok, "exampleuser")
	}
	if connection.Sequence() != 1 {
		t.Errorf("Sequence() = %d, want 1", connection.Sequence())
	}
	if connection.LineNumber() != 6 {
		t.Errorf("LineNumber() = %d, want 6", connection.LineNumber())
	}
	// RawText は原資料の原文を保持する対象だが、CredentialPassword の暗号文だけは
	// redactCredentialPassword が除く。Connection は CredentialPassword を読む method も公開しない
	// (TestReaderCredentialPasswordNeverDecoded)。
	if strings.Contains(connection.RawText(), "example_ciphertext") {
		t.Error("RawText() must redact the CredentialPassword ciphertext")
	}
	if !strings.Contains(connection.RawText(), "<CredentialPassword>[redacted]</CredentialPassword>") {
		t.Errorf("RawText() = %q, want a redacted CredentialPassword element", connection.RawText())
	}

	_, _, err = reader.Next()
	if !errors.Is(err, io.EOF) {
		t.Errorf("second Next() returned err = %v, want io.EOF", err)
	}
}

func TestReaderCredentialPasswordNeverDecoded(t *testing.T) {
	var reader royalts.Reader
	reader.Reset(strings.NewReader(singleConnectionDocument))
	connection, _, err := reader.Next()
	if err != nil {
		t.Fatalf("Next() returned error %v", err)
	}
	// Connection は CredentialPassword を読む method を公開しない。公開 API から値を
	// 取り出す経路が無いことが本 test の対象である。
	if id, ok := connection.ID(); !ok || id != "00000000-0000-0000-0000-000000000001" {
		t.Errorf("ID() = %q, %v", id, ok)
	}
}

func TestReaderMultipleConnections(t *testing.T) {
	document := `<RTSZDocument><RoyalFolder>` +
		`<RoyalRDSConnection><Name>First</Name><URI>203.0.113.10</URI></RoyalRDSConnection>` +
		`<RoyalRDSConnection><Name>Second</Name><URI>host02.example.test</URI></RoyalRDSConnection>` +
		`</RoyalFolder></RTSZDocument>`
	var reader royalts.Reader
	reader.Reset(strings.NewReader(document))
	var names []string
	for {
		connection, failure, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if failure != nil {
			t.Fatalf("Next() returned failure %+v", failure)
		}
		if err != nil {
			t.Fatalf("Next() returned error %v", err)
		}
		name, _ := connection.Name()
		names = append(names, name)
	}
	if want := []string{"First", "Second"}; len(names) != len(want) || names[0] != want[0] || names[1] != want[1] {
		t.Errorf("names = %v, want %v", names, want)
	}
}

func TestReaderNoConnections(t *testing.T) {
	var reader royalts.Reader
	reader.Reset(strings.NewReader(`<RTSZDocument><RoyalFolder><Name>Empty</Name></RoyalFolder></RTSZDocument>`))
	_, _, err := reader.Next()
	if !errors.Is(err, io.EOF) {
		t.Errorf("Next() returned err = %v, want io.EOF", err)
	}
}

func TestReaderMalformedDocument(t *testing.T) {
	var reader royalts.Reader
	reader.Reset(strings.NewReader(`<RTSZDocument><RoyalRDSConnection><Name>Unterminated`))
	_, failure, err := reader.Next()
	if err == nil || errors.Is(err, io.EOF) {
		t.Fatalf("Next() returned err = %v, want a non-EOF error", err)
	}
	if failure == nil {
		t.Fatal("Next() did not return a failure")
	}
}

func TestReaderStripsUTF8BOM(t *testing.T) {
	bom := "\xEF\xBB\xBF"
	var reader royalts.Reader
	reader.Reset(strings.NewReader(bom + singleConnectionDocument))
	connection, failure, err := reader.Next()
	if err != nil {
		t.Fatalf("Next() returned error %v", err)
	}
	if failure != nil {
		t.Fatalf("Next() returned failure %+v", failure)
	}
	if name, ok := connection.Name(); !ok || name != "Example RDP" {
		t.Errorf("Name() = %q, %v, want %q, true", name, ok, "Example RDP")
	}
}

// ByteOffset は BOM を含めた文書全体の中の位置である。BOM の有無で
// RoyalRDSConnection の開始位置がずれないことを確かめる。
func TestReaderByteOffsetAccountsForBOM(t *testing.T) {
	document := "<RTSZDocument><RoyalRDSConnection><Name>A</Name></RoyalRDSConnection></RTSZDocument>"
	var withoutBOM royalts.Reader
	withoutBOM.Reset(strings.NewReader(document))
	plain, failure, err := withoutBOM.Next()
	if err != nil || failure != nil {
		t.Fatalf("Next() = %+v, %v", failure, err)
	}

	var withBOM royalts.Reader
	withBOM.Reset(strings.NewReader("\xEF\xBB\xBF" + document))
	bomed, failure, err := withBOM.Next()
	if err != nil || failure != nil {
		t.Fatalf("Next() = %+v, %v", failure, err)
	}

	if bomed.ByteOffset() != plain.ByteOffset()+3 {
		t.Errorf("ByteOffset() with BOM = %d, want %d (plain offset + 3-byte BOM)",
			bomed.ByteOffset(), plain.ByteOffset()+3)
	}
}

func TestReaderRejectsOversizedDocument(t *testing.T) {
	// maxDocumentBytes の値 (reader.go) をここで写す。
	const maxDocumentBytes = 8 << 20
	oversized := strings.NewReader(strings.Repeat("x", maxDocumentBytes+1))
	var reader royalts.Reader
	reader.Reset(oversized)
	_, failure, err := reader.Next()
	if err == nil || errors.Is(err, io.EOF) {
		t.Fatalf("Next() returned err = %v, want a non-EOF error", err)
	}
	if failure == nil {
		t.Fatal("Next() did not return a failure")
	}
	if failure.Stage != "read" {
		t.Errorf("failure.Stage = %q, want read", failure.Stage)
	}
}

func TestRedactCredentialPasswordSelfClosingForm(t *testing.T) {
	document := `<RTSZDocument><RoyalRDSConnection>` +
		`<Name>SelfClosing</Name><CredentialPassword/>` +
		`</RoyalRDSConnection></RTSZDocument>`
	connection := connectionOf(t, document)
	if strings.Contains(connection.RawText(), "<CredentialPassword/>") {
		t.Error("RawText() must redact the self-closing CredentialPassword element")
	}
}

// 要素の判定は Name.Local で行うため、名前空間の接頭辞を持つ CredentialPassword も
// 見逃さない (royalRDSConnectionXML の decode と同じ基準)。
func TestRedactCredentialPasswordNamespacedElement(t *testing.T) {
	document := `<RTSZDocument xmlns:ns="urn:example:royalts">` +
		`<RoyalRDSConnection>` +
		`<Name>Namespaced</Name>` +
		`<ns:CredentialPassword>example_ciphertext</ns:CredentialPassword>` +
		`</RoyalRDSConnection></RTSZDocument>`
	connection := connectionOf(t, document)
	if strings.Contains(connection.RawText(), "example_ciphertext") {
		t.Errorf("RawText() = %q, must redact a namespaced CredentialPassword element", connection.RawText())
	}
}

// CredentialPassword の前後の文字列は redaction の影響を受けない。
func TestRedactCredentialPasswordPreservesSurroundingText(t *testing.T) {
	document := `<RTSZDocument><RoyalRDSConnection>` +
		`<Name>Before</Name><CredentialPassword>example_ciphertext</CredentialPassword><URI>host01.example.test</URI>` +
		`</RoyalRDSConnection></RTSZDocument>`
	connection := connectionOf(t, document)
	if !strings.Contains(connection.RawText(), "<Name>Before</Name>") {
		t.Errorf("RawText() = %q, must preserve text before CredentialPassword", connection.RawText())
	}
	if !strings.Contains(connection.RawText(), "<URI>host01.example.test</URI>") {
		t.Errorf("RawText() = %q, must preserve text after CredentialPassword", connection.RawText())
	}
}
