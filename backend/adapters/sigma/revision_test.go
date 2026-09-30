package sigma_test

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/sigma"
)

// syntheticCommit は commit の文字列である。
var syntheticCommit = strings.Repeat("ab12", 10)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// gitIndex は names を追跡する git の index の byte 列を返す。version は 2 か 4 である。
// 各項目の stat と object の値は 0 である。
func gitIndex(version uint32, names ...string) []byte {
	var out bytes.Buffer
	out.WriteString("DIRC")
	_ = binary.Write(&out, binary.BigEndian, version)
	_ = binary.Write(&out, binary.BigEndian, uint32(len(names)))
	previous := ""
	for _, name := range names {
		start := out.Len()
		out.Write(make([]byte, 40+20))
		_ = binary.Write(&out, binary.BigEndian, uint16(len(name)))
		if version == 4 {
			// 前の名前と共通の先頭を除き、除く byte 数を 1 byte の数で書く。
			common := 0
			for common < len(previous) && common < len(name) && previous[common] == name[common] {
				common++
			}
			out.WriteByte(byte(len(previous) - common))
			out.WriteString(name[common:])
			out.WriteByte(0)
		} else {
			out.WriteString(name)
			out.Write(make([]byte, 8-(out.Len()-start)%8))
		}
		previous = name
	}
	return out.Bytes()
}

// checkout は HEAD が branch を指す git の directory を持つ作業ツリーを作り、ルールの
// subdirectory を返す。index は rules/windows の下の file を追跡する。
func checkout(t *testing.T, packed bool) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/main\n")
	writeFile(t, filepath.Join(root, ".git", "index"), string(gitIndex(2, "README.md", "rules/windows/a.yml")))
	if packed {
		writeFile(t, filepath.Join(root, ".git", "packed-refs"), "# pack-refs with: peeled\n"+syntheticCommit+" refs/heads/main\n")
	} else {
		writeFile(t, filepath.Join(root, ".git", "refs", "heads", "main"), syntheticCommit+"\n")
	}
	rules := filepath.Join(root, "rules", "windows")
	if err := os.MkdirAll(rules, 0o750); err != nil {
		t.Fatal(err)
	}
	return rules
}

func TestResolveRevisionReadsGitHead(t *testing.T) {
	for _, packed := range []bool{false, true} {
		dir := checkout(t, packed)
		got, err := sigma.ResolveRevision(dir, "")
		if err != nil || got.Commit != syntheticCommit || got.Source != sigma.RevisionSourceGitHead ||
			got.WorkTree != filepath.Dir(filepath.Dir(dir)) {
			t.Errorf("packed=%v: got %+v, %v", packed, got, err)
		}
	}
}

// バージョン 4 の index は名前の先頭を前の項目と共有する。
func TestResolveRevisionReadsIndexVersion4(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".git", "HEAD"), syntheticCommit+"\n")
	writeFile(t, filepath.Join(root, ".git", "index"),
		string(gitIndex(4, "rules/linux/a.yml", "rules/windows/b.yml", "tests/c.py")))
	for dir, want := range map[string]sigma.RevisionSource{
		"rules/windows": sigma.RevisionSourceGitHead, "rules/win": sigma.RevisionSourceUnverified,
		"deprecated": sigma.RevisionSourceUnverified,
	} {
		path := filepath.Join(root, filepath.FromSlash(dir))
		if err := os.MkdirAll(path, 0o750); err != nil {
			t.Fatal(err)
		}
		got, err := sigma.ResolveRevision(path, "")
		if err != nil || got.Source != want {
			t.Errorf("%s: got %+v, %v; want %s", dir, got, err, want)
		}
	}
}

func TestResolveRevisionChecksTheGivenCommitAgainstHead(t *testing.T) {
	dir := checkout(t, false)
	got, err := sigma.ResolveRevision(dir, strings.ToUpper(syntheticCommit[:8]))
	if err != nil || got.Commit != syntheticCommit || got.Source != sigma.RevisionSourceGitHead {
		t.Fatalf("abbreviated: got %+v, %v", got, err)
	}
	for _, given := range []string{"ab12", strings.Repeat("cd34", 10)} {
		if _, err := sigma.ResolveRevision(dir, given); err == nil {
			t.Errorf("given %q: accepted a revision that differs from HEAD", given)
		}
	}
}

func TestResolveRevisionWithoutGit(t *testing.T) {
	dir := t.TempDir()
	given, err := sigma.ResolveRevision(dir, syntheticCommit)
	if err != nil || given.Commit != syntheticCommit || given.Source != sigma.RevisionSourceArgument || given.Detail == "" {
		t.Fatalf("given: %+v, %v", given, err)
	}
	unverified, err := sigma.ResolveRevision(dir, "")
	if err != nil || unverified.Commit != "" || unverified.Source != sigma.RevisionSourceUnverified || unverified.Detail == "" {
		t.Fatalf("unverified: %+v, %v", unverified, err)
	}
}

// symlink を経由して渡した directory も、symlink の先の作業ツリーの HEAD をルールの集合の commit にする。
func TestResolveRevisionFollowsSymlinks(t *testing.T) {
	rules := checkout(t, false)
	link := filepath.Join(t.TempDir(), "rules-link")
	if err := os.Symlink(rules, link); err != nil {
		t.Fatal(err)
	}
	got, err := sigma.ResolveRevision(link, "")
	if err != nil || got.Commit != syntheticCommit || got.Source != sigma.RevisionSourceGitHead {
		t.Fatalf("got %+v, %v", got, err)
	}
}

// ルールの directory を追跡していない上位の作業ツリーの HEAD をルールの集合の commit にしない。
func TestResolveRevisionIgnoresAnUnrelatedWorkTree(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".git", "HEAD"), syntheticCommit+"\n")
	rules := filepath.Join(root, "downloads", "rules")
	writeFile(t, filepath.Join(rules, "a.yml"), "title: t\n")
	got, err := sigma.ResolveRevision(rules, "")
	if err != nil || got.Source != sigma.RevisionSourceUnverified || got.Commit != "" || got.Detail == "" {
		t.Fatalf("without a revision: %+v, %v", got, err)
	}
	other := strings.Repeat("cd34", 10)
	got, err = sigma.ResolveRevision(rules, other)
	if err != nil || got.Source != sigma.RevisionSourceArgument || got.Commit != other {
		t.Fatalf("with a revision: %+v, %v", got, err)
	}
}

func TestResolveRevisionRejectsARevisionThatIsNotACommit(t *testing.T) {
	for _, given := range []string{"abc12", "release-1", "zzzzzzzz"} {
		if _, err := sigma.ResolveRevision(t.TempDir(), given); err == nil {
			t.Errorf("accepted %q", given)
		}
	}
}

func TestResolveRevisionFollowsWorktreeGitFile(t *testing.T) {
	main := t.TempDir()
	writeFile(t, filepath.Join(main, "refs", "heads", "topic"), syntheticCommit+"\n")
	worktreeGitDir := filepath.Join(main, "worktrees", "wt")
	writeFile(t, filepath.Join(worktreeGitDir, "HEAD"), "ref: refs/heads/topic\n")
	writeFile(t, filepath.Join(worktreeGitDir, "commondir"), "../..\n")
	writeFile(t, filepath.Join(worktreeGitDir, "index"), string(gitIndex(2, "a.yml")))
	tree := t.TempDir()
	writeFile(t, filepath.Join(tree, ".git"), "gitdir: "+worktreeGitDir+"\n")
	got, err := sigma.ResolveRevision(tree, "")
	if err != nil || got.Commit != syntheticCommit || got.Source != sigma.RevisionSourceGitHead {
		t.Fatalf("got %+v, %v", got, err)
	}
}
