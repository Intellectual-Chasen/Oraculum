package core_test

import (
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// spoiledCandidates は、最後の候補の位置を検査の通らない形にした並びを返す。
func spoiledCandidates(t *testing.T) []core.MatchCandidateRecord {
	t.Helper()
	candidates := slices.Clone(positiveCandidates(t))
	candidates[len(candidates)-1].Observation.Ref.SourceContentSha256 = "not a digest"
	return candidates
}

// 検査を通る並びは検査済みの並びになり、1 件でも通らない並びは退ける。
func TestValidateCandidatesRejectsAnInvalidCandidate(t *testing.T) {
	valid := positiveCandidates(t)
	validated, err := core.ValidateCandidates(valid)
	if err != nil {
		t.Fatalf("the valid candidates are rejected: %v", err)
	}
	if !reflect.DeepEqual(validated.Records(), valid) {
		t.Error("the validated candidates differ from the given candidates")
	}
	if _, err := core.ValidateCandidates(spoiledCandidates(t)); !errors.Is(err, core.ErrInvalid) {
		t.Errorf("ValidateCandidates returned %v, want the invalid candidate rejected", err)
	}
}

// 検査済みの並びを載せた要求は、載せない要求と同じ候補集合を返す。
func TestAValidatedRequestBuildsTheSameCandidateSet(t *testing.T) {
	candidates := positiveCandidates(t)
	validated, err := core.ValidateCandidates(candidates)
	if err != nil {
		t.Fatal(err)
	}
	plain := buildSet(t, matchRequest(t, candidates))
	checked := buildSet(t, matchRequest(t, candidates).WithValidatedCandidates(validated))
	if !reflect.DeepEqual(checked, plain) {
		t.Error("the request with validated candidates builds another candidate set")
	}
}

// 載せた後に候補の並びを差し替えた要求は、候補の検査を省かない。
func TestAReplacedCandidateListIsValidatedAgain(t *testing.T) {
	validated, err := core.ValidateCandidates(positiveCandidates(t))
	if err != nil {
		t.Fatal(err)
	}
	request := matchRequest(t, nil).WithValidatedCandidates(validated)
	if err := request.Validate(); err != nil {
		t.Fatalf("the request with validated candidates is rejected: %v", err)
	}
	request.Candidates = spoiledCandidates(t)
	if err := request.Validate(); !errors.Is(err, core.ErrInvalid) {
		t.Errorf("Validate returned %v, want the replaced invalid candidate rejected", err)
	}
}

// 一部を取り出した並びは、指定した番号の要素を指定した順で持つ。
func TestASubsetKeepsTheNamedCandidatesInOrder(t *testing.T) {
	candidates := positiveCandidates(t)
	if len(candidates) < 2 {
		t.Fatal("the fixture needs two candidates or more")
	}
	validated, err := core.ValidateCandidates(candidates)
	if err != nil {
		t.Fatal(err)
	}
	indexes := []int{len(candidates) - 1, 0}
	subset := validated.Subset(indexes).Records()
	want := []core.MatchCandidateRecord{candidates[len(candidates)-1], candidates[0]}
	if !reflect.DeepEqual(subset, want) {
		t.Error("the subset does not carry the named candidates in the named order")
	}
}
