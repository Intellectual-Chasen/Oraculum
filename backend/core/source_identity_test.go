package core_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// markIIIdentity は markii 形式の側の収集元の識別を返す。
func markIIIdentity(t *testing.T) core.SourceIdentity {
	t.Helper()
	first := clockTime(t, markIIObservedRangeFirst, core.ClockTerminalLocal)
	last := clockTime(t, markIIObservedRangeLast, core.ClockTerminalLocal)
	// 通番の範囲 (fixtures_test.go の rangeFromSequenceNumber から
	// rangeToSequenceNumber まで) の両端を含む件数と揃える。
	recordCount := int64(801)
	return core.SourceIdentity{
		SourceId:           markIISourceId,
		ContentSha256:      markIISha256,
		OriginPath:         markIIOriginPath,
		FileName:           markIIFileName,
		SizeBytes:          1024,
		RecordCount:        &recordCount,
		NewlineCount:       801,
		EndsWithNewline:    true,
		LineEnding:         core.LineEndingLf,
		FormatKey:          core.FormatKey("infotrace_mark_ii"),
		ObservedRangeFirst: &first,
		ObservedRangeLast:  &last,
	}
}

// 観測期間の両端は揃って出るか、揃って出ない。
// 2 項目が出ない場合の意味はどちらも「時刻を持つレコードが 0 件である」である。
func TestSourceIdentityCarriesBothEndsOfTheObservedRange(t *testing.T) {
	withoutBoth := markIIIdentity(t)
	withoutBoth.ObservedRangeFirst = nil
	withoutBoth.ObservedRangeLast = nil
	if err := withoutBoth.Validate(); err != nil {
		t.Fatalf("validating a source identity without the observed range: %v", err)
	}

	onlyFirst := markIIIdentity(t)
	onlyFirst.ObservedRangeLast = nil
	if err := onlyFirst.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
		t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
	}

	onlyLast := markIIIdentity(t)
	onlyLast.ObservedRangeFirst = nil
	if err := onlyLast.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
		t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
	}
}

func TestSourceIdentityValidates(t *testing.T) {
	if err := markIIIdentity(t).Validate(); err != nil {
		t.Fatalf("validating the source identity: %v", err)
	}
}

// レコード件数と改行の個数を別の項目に持つ。末尾の改行が無い収集元を表せる。
func TestSourceIdentityHoldsTheRecordCountApartFromTheNewlineCount(t *testing.T) {
	identity := markIIIdentity(t)
	identity.SourceId = squidSourceId
	identity.ContentSha256 = squidSha256
	identity.OriginPath = squidOriginPath
	identity.FileName = squidFileName
	identity.FormatKey = core.FormatKey("squid_combined")
	recordCount := int64(2000)
	identity.RecordCount = &recordCount
	identity.NewlineCount = 1999
	identity.EndsWithNewline = false
	if err := identity.Validate(); err != nil {
		t.Fatalf("validating the source identity: %v", err)
	}
	if *identity.RecordCount-identity.NewlineCount != 1 {
		t.Errorf("recordCount - newlineCount = %d, want 1",
			*identity.RecordCount-identity.NewlineCount)
	}
}

// レコード件数を確定できない収集元は recordCount を出さない。
// 出ない状態と 0 件の状態を別に表す。
func TestSourceIdentitySeparatesAnAbsentRecordCountFromZero(t *testing.T) {
	undetermined := markIIIdentity(t)
	undetermined.RecordCount = nil
	if err := undetermined.Validate(); err != nil {
		t.Fatalf("validating a source identity without a record count: %v", err)
	}

	zero := markIIIdentity(t)
	zeroCount := int64(0)
	zero.RecordCount = &zeroCount
	if err := zero.Validate(); err != nil {
		t.Fatalf("validating a source identity with a record count of 0: %v", err)
	}
	if undetermined.RecordCount != nil {
		t.Error("an undetermined record count must stay absent")
	}
	if zero.RecordCount == nil || *zero.RecordCount != 0 {
		t.Errorf("recordCount = %v, want a pointer to 0", zero.RecordCount)
	}
}

func headerField(t *testing.T, name, text string) core.RecordField {
	t.Helper()
	value, err := core.NewRawValue(core.ValueStatePresent, text)
	if err != nil {
		t.Fatal(err)
	}
	field, err := core.NewTextField(name, "", value)
	if err != nil {
		t.Fatal(err)
	}
	return field
}

// 原文を変換した収集元と、file の見出しの値は、JSON を通しても同じ値に戻る。
func TestSourceIdentityCarriesTheConvertedRawTextAndTheFileHeader(t *testing.T) {
	identity := markIIIdentity(t)
	identity.RawTextConverted = true
	identity.FileHeader = []core.RecordField{headerField(t, "NextRecordID", "7"), headerField(t, "Dirty", "true")}
	// 説明を組めなかった件数と端末の候補も同じ収集元の識別に並ぶ。
	unrendered := int64(3)
	identity.MessageUnrenderedCount = &unrendered
	identity.TerminalCandidates = []core.TerminalCandidate{{Name: "host-a$", RecordCount: 2}}
	if err := identity.Validate(); err != nil {
		t.Fatalf("Validate() = %v", err)
	}
	encoded, err := json.Marshal(identity)
	if err != nil {
		t.Fatal(err)
	}
	var decoded core.SourceIdentity
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("decoding %s: %v", encoded, err)
	}
	if !decoded.RawTextConverted || len(decoded.FileHeader) != 2 || decoded.FileHeader[1].Name != "Dirty" ||
		decoded.MessageUnrenderedCount == nil || *decoded.MessageUnrenderedCount != 3 ||
		len(decoded.TerminalCandidates) != 1 || decoded.TerminalCandidates[0] != identity.TerminalCandidates[0] {
		t.Errorf("decoded = %+v, want every item of the identity", decoded)
	}
	plain, _ := json.Marshal(markIIIdentity(t))
	if bytes.Contains(plain, []byte("rawTextConverted")) || bytes.Contains(plain, []byte("fileHeader")) {
		t.Errorf("a source of the original bytes encodes %s, want neither item", plain)
	}
}

// withMembers は収集元を主 file 600 byte と付属の file 424 byte の 2 つで構成する。
func withMembers(identity *core.SourceIdentity) {
	identity.Members = []core.SourceMember{
		{OriginPath: identity.OriginPath, ContentSha256: markIISha256, ByteOffset: 0, SizeBytes: 600},
		{OriginPath: identity.OriginPath + ".LOG1", ContentSha256: squidSha256, ByteOffset: 600, SizeBytes: 424},
	}
}

// 構成する file は JSON を通しても同じ値に戻り、持たない収集元は項目を出さない。
func TestSourceIdentityCarriesTheMemberFiles(t *testing.T) {
	identity := markIIIdentity(t)
	withMembers(&identity)
	if err := identity.Validate(); err != nil {
		t.Fatalf("Validate() = %v", err)
	}
	encoded, err := json.Marshal(identity)
	if err != nil {
		t.Fatal(err)
	}
	var decoded core.SourceIdentity
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("decoding %s: %v", encoded, err)
	}
	if len(decoded.Members) != 2 || decoded.Members[1] != identity.Members[1] {
		t.Errorf("decoded members = %+v", decoded.Members)
	}
	if plain, _ := json.Marshal(markIIIdentity(t)); bytes.Contains(plain, []byte("members")) {
		t.Errorf("a source of one file encodes %s", plain)
	}
}

func TestSourceIdentityAcceptsAFormatKeyWithoutCoreRegistration(t *testing.T) {
	identity := markIIIdentity(t)
	identity.FormatKey = "vendor_format_v1"
	if err := identity.Validate(); err != nil {
		t.Fatalf("validating a source identity with an unregistered format key: %v", err)
	}
}

func TestSourceIdentityRejectsBrokenItems(t *testing.T) {
	cases := map[string]struct {
		change func(identity *core.SourceIdentity)
		want   error
	}{
		"a content identifier in upper case": {
			change: func(identity *core.SourceIdentity) {
				identity.ContentSha256 = "AA11BB22CC33DD44EE55FF6600778899AA11BB22CC33DD44EE55FF6600778899"
			},
			want: core.ErrInvalid,
		},
		"a content identifier of another length": {
			change: func(identity *core.SourceIdentity) {
				identity.ContentSha256 = "aa11bb22"
			},
			want: core.ErrInvalid,
		},
		"a line ending outside the 4 values": {
			change: func(identity *core.SourceIdentity) {
				identity.LineEnding = "cr"
			},
			want: core.ErrUnknownEnumValue,
		},
		"an empty format key": {
			change: func(identity *core.SourceIdentity) {
				identity.FormatKey = ""
			},
			want: core.ErrMissingRequiredItem,
		},
		"a negative size": {
			change: func(identity *core.SourceIdentity) {
				identity.SizeBytes = -1
			},
			want: core.ErrNegativeCount,
		},
		"a file name without an origin path": {
			change: func(identity *core.SourceIdentity) {
				identity.OriginPath = ""
			},
			want: core.ErrMissingRequiredItem,
		},
		"a file header field without a value": {
			change: func(identity *core.SourceIdentity) {
				identity.FileHeader = []core.RecordField{{Name: "NextRecordID", Kind: core.RecordFieldKindText}}
			},
			want: core.ErrMissingRequiredItem,
		},
		"a file header that repeats a name": {
			change: func(identity *core.SourceIdentity) {
				identity.FileHeader = []core.RecordField{headerField(t, "Dirty", "true"), headerField(t, "Dirty", "false")}
			},
			want: core.ErrInconsistentValue,
		},
		"members that start with another file": {
			change: func(identity *core.SourceIdentity) {
				withMembers(identity)
				identity.Members[0].OriginPath = "other.log"
			},
			want: core.ErrInconsistentValue,
		},
		"members with a gap": {
			change: func(identity *core.SourceIdentity) {
				withMembers(identity)
				identity.Members[1].ByteOffset = 601
			},
			want: core.ErrInconsistentValue,
		},
		"members that do not add up to the size": {
			change: func(identity *core.SourceIdentity) {
				withMembers(identity)
				identity.Members[1].SizeBytes = 400
			},
			want: core.ErrInconsistentValue,
		},
		"a member digest in upper case": {
			change: func(identity *core.SourceIdentity) {
				withMembers(identity)
				identity.Members[1].ContentSha256 = "AA11BB22CC33DD44EE55FF6600778899AA11BB22CC33DD44EE55FF6600778899"
			},
			want: core.ErrInvalid,
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			identity := markIIIdentity(t)
			testCase.change(&identity)
			if err := identity.Validate(); !errors.Is(err, testCase.want) {
				t.Errorf("error = %v, want one wrapping %v", err, testCase.want)
			}
		})
	}
}

// 時刻の範囲は両端を比べられるとき前後を確かめる。
func TestTimeRangeRejectsAReversedRange(t *testing.T) {
	reversed := core.TimeRange{
		From: clockTime(t, markIIObservedRangeLast, core.ClockTerminalLocal),
		To:   clockTime(t, markIIObservedRangeFirst, core.ClockTerminalLocal),
	}
	if err := reversed.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
		t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
	}
}

// 両端を比べられない範囲は前後を判定しない。
func TestTimeRangeWithoutInstantsIsNotOrdered(t *testing.T) {
	undetermined := core.TimeRange{
		From: markIIStartTime(t),
		To:   markIIStartTime(t),
	}
	if err := undetermined.Validate(); err != nil {
		t.Fatalf("validating a range whose offset is undetermined: %v", err)
	}
	if _, ok := undetermined.Contains(markIIStartTime(t)); ok {
		t.Error("a range without instants must not report a containment")
	}
}
