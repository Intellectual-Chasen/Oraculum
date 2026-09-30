package core_test

import (
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

func validUrlFragmentJoin() core.UrlFragmentJoin {
	return core.UrlFragmentJoin{
		EdgeId: graphEdgeId,
		Segments: []core.UrlFragmentSegment{{
			FragmentCount: 2, LastNumber: 0, DuplicateCount: 1,
			Fragments: []core.UrlFragment{
				{Number: 0, Adopted: true, RecordRef: graphRecordLocator()},
				{Number: 0, RecordRef: graphRecordLocator()},
			},
			Encoding: core.UrlFragmentEncodingBase64Url,
			Decoded: &core.DecodedBytes{
				ByteCount: 22, Sha256: strings.Repeat("a", 64), LeadingBytesHex: "504b0506",
				ContentType: "application/zip",
				Zip:         &core.ZipListing{Readable: true, Integrity: core.ZipIntegrityPassed, Entries: []core.ZipEntryName{}},
			},
		}},
	}
}

func TestUrlFragmentJoinValidate(t *testing.T) {
	requireValid(t, "join", validUrlFragmentJoin())
	for name, change := range map[string]func(*core.UrlFragmentSegment){
		"decoded と failure の両方": func(s *core.UrlFragmentSegment) {
			s.DecodeFailure = core.UrlFragmentDecodeFailureMissingNumbers
		},
		"decoded も failure も無い": func(s *core.UrlFragmentSegment) { s.Decoded, s.Encoding = nil, "" },
		"重複が行の数以上":              func(s *core.UrlFragmentSegment) { s.DuplicateCount = 2 },
		"行の数と一致しない":             func(s *core.UrlFragmentSegment) { s.FragmentCount = 3 },
		"番号が最後の番号を超える":          func(s *core.UrlFragmentSegment) { s.Fragments[1].Number = 1 },
		"sha256 が不正":            func(s *core.UrlFragmentSegment) { s.Decoded.Sha256 = "x" },
		"字母が無い":                 func(s *core.UrlFragmentSegment) { s.Encoding = "" },
		"未知の字母":                 func(s *core.UrlFragmentSegment) { s.Encoding = "base32" },
		"読めない ZIP が名前を持つ": func(s *core.UrlFragmentSegment) {
			s.Decoded.Zip = &core.ZipListing{EntryCount: 1, Entries: []core.ZipEntryName{{Name: "a"}}}
		},
		"位置が不正": func(s *core.UrlFragmentSegment) { s.Fragments[0].RecordRef = core.RecordLocator{} },
	} {
		join := validUrlFragmentJoin()
		change(&join.Segments[0])
		requireInvalid(t, name, join)
	}
	failed := validUrlFragmentJoin()
	failed.Segments[0].Decoded, failed.Segments[0].Encoding = nil, ""
	failed.Segments[0].DecodeFailure = core.UrlFragmentDecodeFailureAlphabetUndetermined
	requireValid(t, "undetermined", failed)
}
