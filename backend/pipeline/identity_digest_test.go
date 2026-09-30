// in-package test: 通番の上限直前を設定し、枯渇時の再発行防止を検証する。
package pipeline

import (
	"encoding/json"
	"math"
	"os"
	"slices"
	"sync"
	"testing"
)

type identityFixture struct {
	Versions []struct {
		Name, Parser, Format, Revision, Settings, Want string
	}
	Sources []struct {
		Name    string
		Path    string
		Content string
		Ordinal int64
		Want    string
	}
	Runs []struct {
		Name    string
		Entries []RunManifestEntry
		Want    string
	}
}

func readIdentityFixture(t *testing.T) identityFixture {
	t.Helper()
	data, err := os.ReadFile("testdata/identity/manifest.json")
	if err != nil {
		t.Fatalf("read identity fixture: %v", err)
	}
	var fixture identityFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("decode identity fixture: %v", err)
	}
	if len(fixture.Sources) == 0 || len(fixture.Runs) == 0 || len(fixture.Versions) == 0 {
		t.Fatalf("identity fixture carries %d sources, %d runs and %d versions, want each populated",
			len(fixture.Sources), len(fixture.Runs), len(fixture.Versions))
	}
	return fixture
}

func TestDigestMinterSourceId(t *testing.T) {
	var minter IdentityMinter = DigestMinter{}
	for _, tc := range readIdentityFixture(t).Sources {
		t.Run(tc.Name, func(t *testing.T) {
			got, err := minter.SourceId(tc.Path, tc.Content, tc.Ordinal)
			if err != nil || got != tc.Want {
				t.Fatalf("SourceId = %q, %v; want %q", got, err, tc.Want)
			}
		})
	}
	for _, ordinal := range []int64{0, -1} {
		got, err := minter.SourceId("input/log", "abc", ordinal)
		if err == nil || got != "" {
			t.Fatalf("ordinal %d: got %q, %v; want empty and error", ordinal, got, err)
		}
	}
}

func TestDigestMinterAnalysisRunRef(t *testing.T) {
	minter := DigestMinter{}
	for _, tc := range readIdentityFixture(t).Runs {
		t.Run(tc.Name, func(t *testing.T) {
			before := slices.Clone(tc.Entries)
			got, err := minter.AnalysisRunRef(tc.Entries)
			if err != nil || got != tc.Want {
				t.Fatalf("AnalysisRunRef = %q, %v; want %q", got, err, tc.Want)
			}
			if !slices.Equal(tc.Entries, before) {
				t.Fatal("AnalysisRunRef mutated input")
			}
			slices.Reverse(tc.Entries)
			got, err = minter.AnalysisRunRef(tc.Entries)
			if err != nil || got != tc.Want {
				t.Fatalf("reversed AnalysisRunRef = %q, %v; want %q", got, err, tc.Want)
			}
		})
	}
}

func TestDigestMinterDuplicateSource(t *testing.T) {
	for _, version := range []string{"v1", "v2"} {
		got, err := (DigestMinter{}).AnalysisRunRef([]RunManifestEntry{
			{SourceId: "a", ParserVersion: "v1"}, {SourceId: "b", ParserVersion: "v1"},
			{SourceId: "a", ParserVersion: version},
		})
		if err == nil || got != "" {
			t.Fatalf("duplicate source: got %q, %v; want empty and error", got, err)
		}
	}
}

func TestDigestMinterRejectsAPartlyCasedRun(t *testing.T) {
	got, err := (DigestMinter{}).AnalysisRunRef([]RunManifestEntry{
		{SourceId: "a", ParserVersion: "v1", CaseId: "baseline"},
		{SourceId: "b", ParserVersion: "v1"},
	})
	if err == nil || got != "" {
		t.Fatalf("partly cased run: got %q, %v; want empty and error", got, err)
	}
}

func TestDigestMinterParserVersion(t *testing.T) {
	for _, tc := range readIdentityFixture(t).Versions {
		t.Run(tc.Name, func(t *testing.T) {
			got, err := (DigestMinter{}).ParserVersion(ParserIdentity{ParserID: tc.Parser, SupportedFormatVersion: tc.Format}, tc.Revision, tc.Settings)
			if err != nil || got != tc.Want {
				t.Fatalf("ParserVersion = %q, %v; want %q", got, err, tc.Want)
			}
		})
	}
}

func TestInMemoryOrdinals(t *testing.T) {
	source := NewInMemoryOrdinals()
	for _, tc := range []struct {
		path, content string
		want          int64
	}{
		{"a", "b", 1}, {"a", "b", 2}, {"a", "b", 3},
		{"c", "b", 1}, {"a", "c", 1}, {"ab", "c", 1},
		{"a", "bc", 1}, {"a\x00b", "c", 1}, {"a", "b\x00c", 1},
	} {
		got, err := source.Next(tc.path, tc.content)
		if err != nil || got != tc.want {
			t.Fatalf("Next(%q, %q) = %d, %v; want %d", tc.path, tc.content, got, err, tc.want)
		}
	}
	got, err := NewInMemoryOrdinals().Next("a", "b")
	if err != nil || got != 1 {
		t.Fatalf("new execution ordinal = %d, %v; want 1", got, err)
	}
}

func TestInMemoryOrdinalsExhausted(t *testing.T) {
	source := &inMemoryOrdinals{counts: map[[2]string]int64{{"a", "b"}: math.MaxInt64 - 1}}
	got, err := source.Next("a", "b")
	if err != nil || got != math.MaxInt64 {
		t.Fatalf("last ordinal = %d, %v", got, err)
	}
	for range 2 {
		got, err = source.Next("a", "b")
		if err == nil || got != 0 {
			t.Fatalf("exhausted ordinal = %d, %v; want 0 and error", got, err)
		}
	}
}

func TestInMemoryOrdinalsConcurrent(t *testing.T) {
	source := NewInMemoryOrdinals()
	const count = 32
	results := make([]int64, count)
	errors := make([]error, count)
	var group sync.WaitGroup
	for i := range count {
		group.Go(func() { results[i], errors[i] = source.Next("a", "b") })
	}
	group.Wait()
	for _, err := range errors {
		if err != nil {
			t.Fatalf("concurrent Next: %v", err)
		}
	}
	slices.Sort(results)
	for i, got := range results {
		if got != int64(i+1) {
			t.Fatalf("ordinal at %d = %d; want %d", i, got, i+1)
		}
	}
}
