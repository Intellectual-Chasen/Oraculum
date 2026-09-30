package pipeline_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"reflect"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

const companionFormatKey core.FormatKey = "companion_test"

// companionParser は付属の file を読む形式の代わりである。連結した byte 列全体を 1 件の
// レコードとして返し、受け取った構成の file を残す。
type companionParser struct {
	members []core.SourceMember
	content []byte
	done    bool
}

func (p *companionParser) Identity() pipeline.ParserIdentity {
	return pipeline.ParserIdentity{
		ParserID: "companion-test", FormatKey: companionFormatKey, PositionKind: core.PositionKindByteRange,
		TimePrecision: core.PrecisionSecond, RecordedByOneTerminal: true, ItemSemantics: []core.SemanticKey{},
		ConnectionRequestKinds: []core.ObservationKindSelector{}, ConnectionMatchConditions: []pipeline.ConnectionMatchCondition{},
		TranscriptIdentityItems: []string{},
	}
}

func (p *companionParser) CompanionSuffixes() []string { return []string{".LOG1", ".LOG2"} }

func (p *companionParser) SetMembers(members []core.SourceMember) { p.members = members }

func (p *companionParser) Reset(input io.Reader) {
	p.content, _ = io.ReadAll(input)
	p.done = false
}

func (p *companionParser) Next() (pipeline.ParsedRecord, *core.ImportFailure, error) {
	if p.done {
		return pipeline.ParsedRecord{}, nil, io.EOF
	}
	p.done = true
	length := int64(len(p.content))
	return pipeline.ParsedRecord{RawText: string(p.content), ByteLength: &length}, nil, nil
}

func companionRunner(t *testing.T, files map[string]string, parser *companionParser, openErr error) *pipeline.Runner {
	t.Helper()
	config := runnerConfig("")
	config.Parsers = map[core.FormatKey]pipeline.ParserFactory{
		companionFormatKey: func(*string) (pipeline.SourceParser, error) { return parser, nil },
	}
	config.Open = func(path string) (io.ReadCloser, error) {
		if content, ok := files[path]; ok {
			return io.NopCloser(strings.NewReader(content)), nil
		}
		if openErr != nil {
			return nil, openErr
		}
		return nil, fmt.Errorf("opening %s: %w", path, fs.ErrNotExist)
	}
	runner, err := pipeline.NewRunner(config)
	if err != nil {
		t.Fatal(err)
	}
	return runner
}

func sha256Text(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func TestRunnerConcatenatesTheCompanionFilesThatExist(t *testing.T) {
	files := map[string]string{"logs/h": "PRIMARY", "logs/h.LOG2": "LOG-TWO"}
	parser := &companionParser{}
	runner := companionRunner(t, files, parser, nil)
	result, err := runner.Run([]pipeline.SourcePlan{{OriginPath: "logs/h", FileName: "h", FormatKey: companionFormatKey}})
	if err != nil {
		t.Fatal(err)
	}
	wantMembers := []core.SourceMember{
		{OriginPath: "logs/h", ContentSha256: sha256Text("PRIMARY"), ByteOffset: 0, SizeBytes: 7},
		{OriginPath: "logs/h.LOG2", ContentSha256: sha256Text("LOG-TWO"), ByteOffset: 7, SizeBytes: 7},
	}
	if !reflect.DeepEqual(parser.members, wantMembers) {
		t.Errorf("the parser received %v, want %v", parser.members, wantMembers)
	}
	if string(parser.content) != "PRIMARYLOG-TWO" {
		t.Errorf("the parser read %q", parser.content)
	}
	sourceId := result.Statuses()[0].SourceId
	identity, _ := result.Identity(sourceId)
	if identity.ContentSha256 != sha256Text("PRIMARYLOG-TWO") || identity.SizeBytes != 14 {
		t.Errorf("identity = %s %d, want the concatenated bytes", identity.ContentSha256, identity.SizeBytes)
	}
	if !reflect.DeepEqual(identity.Members, wantMembers) {
		t.Errorf("identity members = %v", identity.Members)
	}
}

func TestRunnerReportsTheProgressOfThePrimaryFileAlone(t *testing.T) {
	files := map[string]string{"h": "PRIMARY", "h.LOG1": "LOG-ONE-LONGER"}
	config := runnerConfig("")
	parser := &companionParser{}
	config.Parsers = map[core.FormatKey]pipeline.ParserFactory{
		companionFormatKey: func(*string) (pipeline.SourceParser, error) { return parser, nil },
	}
	config.Open = func(path string) (io.ReadCloser, error) {
		if content, ok := files[path]; ok {
			return io.NopCloser(strings.NewReader(content)), nil
		}
		return nil, fs.ErrNotExist
	}
	var maxRead int64
	config.Progress = func(progress pipeline.SourceProgress) { maxRead = max(maxRead, progress.ReadBytes) }
	runner, err := pipeline.NewRunner(config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Run([]pipeline.SourcePlan{{OriginPath: "h", FileName: "h", FormatKey: companionFormatKey}}); err != nil {
		t.Fatal(err)
	}
	if maxRead != 7 {
		t.Errorf("the progress reached %d bytes, want the primary file size 7", maxRead)
	}
}

func TestRunnerStopsWhenACompanionFileCannotBeRead(t *testing.T) {
	denied := errors.New("permission denied")
	runner := companionRunner(t, map[string]string{"h": "PRIMARY"}, &companionParser{}, denied)
	_, err := runner.Run([]pipeline.SourcePlan{{OriginPath: "h", FileName: "h", FormatKey: companionFormatKey}})
	if !errors.Is(err, denied) || !strings.Contains(err.Error(), "h.LOG1") {
		t.Errorf("err = %v, want the companion failure", err)
	}
}

func TestRunnerComparesTheRecordedDigestWithTheConcatenatedBytes(t *testing.T) {
	files := map[string]string{"h": "PRIMARY", "h.LOG1": "LOG-ONE"}
	for _, tc := range []struct {
		name     string
		expected string
		wantErr  bool
	}{
		{name: "concatenated", expected: sha256Text("PRIMARYLOG-ONE")},
		{name: "primary alone", expected: sha256Text("PRIMARY"), wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner := companionRunner(t, files, &companionParser{}, nil)
			expected := tc.expected
			_, err := runner.Run([]pipeline.SourcePlan{{
				OriginPath: "h", FileName: "h", FormatKey: companionFormatKey, ExpectedContentSha256: &expected,
			}})
			if (err != nil) != tc.wantErr {
				t.Errorf("err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
