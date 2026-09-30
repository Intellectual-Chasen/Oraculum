package sigma

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RevisionSource は、ルールの集合の commit をどこから決めたかである。
type RevisionSource string

// RevisionSource の値。
const (
	// RevisionSourceGitHead は directory を追跡する git の作業ツリーの HEAD から読んだ commit である。
	// 起動が commit を渡したときは、渡した文字列が HEAD の commit と一致した。
	RevisionSourceGitHead RevisionSource = "git_head"
	// RevisionSourceArgument は起動が渡した commit である。directory を追跡する git の HEAD を読めず、
	// 渡した文字列を HEAD と突き合わせていない。
	RevisionSourceArgument RevisionSource = "argument"
	// RevisionSourceUnverified は commit を確かめられなかったことを表す。起動が commit を渡さず、
	// git の HEAD も読めなかった。
	RevisionSourceUnverified RevisionSource = "unverified"
)

// Revision はルールの集合の commit である。
type Revision struct {
	// Commit は commit の文字列である。Source が unverified のときは空である。
	Commit string
	Source RevisionSource
	// Detail は、git の HEAD をルールの集合の commit にできなかった理由である。HEAD を commit にしたときは空である。
	Detail string
	// WorkTree は directory を含む git の作業ツリーの root の絶対 path である。作業ツリーが
	// 見つからないときは空である。
	WorkTree string
}

// minimumAbbreviatedCommit は、起動が渡す commit の文字列の最短の長さである。git が
// 短縮した commit を表示する既定の長さと同じである。
const minimumAbbreviatedCommit = 7

// ResolveRevision は directory のルールの集合の commit を決める。
//
// given は起動が渡した commit の文字列であり、空は渡していないことを表す。16 進の 7 文字以上で
// ない given は error を返す。directory を含む git の作業ツリーが directory の file を追跡して
// いて HEAD を読めたときは、given が HEAD の commit と一致すること (短縮した文字列なら先頭が
// 一致すること) を確かめ、一致しなければ error を返す。作業ツリーが directory を追跡して
// いないときは、その作業ツリーの HEAD を commit にしない。
func ResolveRevision(directory, given string) (Revision, error) {
	given = strings.ToLower(strings.TrimSpace(given))
	if given != "" && !isCommitPrefix(given) {
		return Revision{}, fmt.Errorf("the given revision %q is not a commit of 7 or more hexadecimal digits", given)
	}
	head, workTree, problem := trackedHead(directory)
	if problem != nil {
		if given != "" {
			return Revision{Commit: given, Source: RevisionSourceArgument, Detail: problem.Error(), WorkTree: workTree}, nil
		}
		return Revision{Source: RevisionSourceUnverified, Detail: problem.Error(), WorkTree: workTree}, nil
	}
	if given != "" && !strings.HasPrefix(head, given) {
		return Revision{}, fmt.Errorf("the given revision %q differs from the git HEAD %s of %q", given, head, workTree)
	}
	return Revision{Commit: head, Source: RevisionSourceGitHead, WorkTree: workTree}, nil
}

func isCommitPrefix(text string) bool {
	const sha256Length = 64
	if len(text) < minimumAbbreviatedCommit || len(text) > sha256Length {
		return false
	}
	return !strings.ContainsFunc(text, func(char rune) bool {
		return (char < '0' || char > '9') && (char < 'a' || char > 'f')
	})
}

// trackedHead は directory を含む git の作業ツリーの root と、HEAD の commit を返す。作業
// ツリーの index が directory の下の file を 1 つも持たないときは、作業ツリーの root と理由を返す。
func trackedHead(directory string) (head, workTree string, problem error) {
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return "", "", fmt.Errorf("resolving %q: %w", directory, err)
	}
	// symlink を経由した directory は、親をたどっても symlink の先の作業ツリーに届かない。
	if absolute, err = filepath.EvalSymlinks(absolute); err != nil {
		return "", "", fmt.Errorf("resolving the symbolic links of %q: %w", directory, err)
	}
	gitDir, workTree, err := findGitDir(absolute)
	if err != nil {
		return "", "", err
	}
	head, err = gitHead(gitDir)
	if err != nil {
		return "", workTree, err
	}
	relative, err := filepath.Rel(workTree, absolute)
	if err != nil {
		return "", workTree, fmt.Errorf("resolving %q in the git working tree %q: %w", absolute, workTree, err)
	}
	index, err := os.ReadFile(filepath.Join(gitDir, "index")) // #nosec G304 -- git の directory の中の file を読む。
	if err != nil {
		return "", workTree, fmt.Errorf("reading the git index of %q: %w", workTree, err)
	}
	const sha1Bytes, sha256Bytes = 20, 32
	objectSize := sha1Bytes
	if len(head) == 2*sha256Bytes {
		objectSize = sha256Bytes
	}
	tracked, err := indexTracks(index, filepath.ToSlash(relative), objectSize)
	if err != nil {
		return "", workTree, fmt.Errorf("reading the git index of %q: %w", workTree, err)
	}
	if !tracked {
		return "", workTree, fmt.Errorf("the git working tree %q does not track the rule directory", workTree)
	}
	return head, workTree, nil
}

// gitHead は git の directory の HEAD の commit を、file を読んで返す。
func gitHead(gitDir string) (string, error) {
	head, err := os.ReadFile(filepath.Join(gitDir, "HEAD")) // #nosec G304 -- 運用者が起動引数で指定した directory の git の file を読む。
	if err != nil {
		return "", fmt.Errorf("reading the git HEAD: %w", err)
	}
	text := strings.TrimSpace(string(head))
	ref, symbolic := strings.CutPrefix(text, "ref: ")
	if !symbolic {
		return commitOf(text)
	}
	// ref を file の path に使うため、git の directory の外を指す文字列を退ける。
	if !strings.HasPrefix(ref, "refs/") || strings.Contains(ref, "..") {
		return "", fmt.Errorf("the git HEAD points to an unexpected ref %q", ref)
	}
	// 作業ツリーを足した repository の ref は、commondir が指す本体の directory にある。
	commonDir := gitDir
	if common, err := os.ReadFile(filepath.Join(gitDir, "commondir")); err == nil { // #nosec G304 -- git の directory の中の file を読む。
		commonDir = resolveFrom(gitDir, strings.TrimSpace(string(common)))
	}
	for _, dir := range []string{gitDir, commonDir} {
		// #nosec G304 G703 -- ref は refs/ で始まり .. を含まない文字列に限っている。
		if loose, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(ref))); err == nil {
			return commitOf(strings.TrimSpace(string(loose)))
		}
	}
	packed, err := os.ReadFile(filepath.Join(commonDir, "packed-refs")) // #nosec G304 G703 -- git の directory の中の file を読む。
	if err != nil {
		return "", fmt.Errorf("reading the git ref %s: %w", ref, err)
	}
	scanner := bufio.NewScanner(bytes.NewReader(packed))
	for scanner.Scan() {
		commit, name, found := strings.Cut(scanner.Text(), " ")
		if found && name == ref {
			return commitOf(commit)
		}
	}
	return "", fmt.Errorf("the git ref %s is not found", ref)
}

// findGitDir は directory から親へ向かって `.git` を探し、git の directory と、`.git` を持つ
// 作業ツリーの root を返す。`.git` が file のときは、その `gitdir:` が指す directory を返す。
func findGitDir(directory string) (gitDir, workTree string, err error) {
	for current := directory; ; {
		candidate := filepath.Join(current, ".git")
		info, err := os.Stat(candidate)
		switch {
		case err == nil && info.IsDir():
			return candidate, current, nil
		case err == nil:
			content, err := os.ReadFile(candidate) // #nosec G304 -- 運用者が指定した directory の親にある .git を読む。
			if err != nil {
				return "", current, fmt.Errorf("reading %q: %w", candidate, err)
			}
			pointer, found := strings.CutPrefix(strings.TrimSpace(string(content)), "gitdir: ")
			if !found {
				return "", current, fmt.Errorf("%q does not point to a git directory", candidate)
			}
			return resolveFrom(current, pointer), current, nil
		case !errors.Is(err, os.ErrNotExist):
			return "", "", fmt.Errorf("reading %q: %w", candidate, err)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", "", errors.New("the rule directory is not in a git working tree")
		}
		current = parent
	}
}

// indexTracks は git の index (バージョン 2 から 4) が、作業ツリーの root からの path が relative で
// ある directory の下の file を持つかを返す。relative が `.` なら、file を 1 つでも持つかを返す。
func indexTracks(index []byte, relative string, objectSize int) (bool, error) {
	const headerSize, statSize, flagsSize, extendedFlag, nameMask = 12, 40, 2, 0x4000, 0x0fff
	if len(index) < headerSize || string(index[:4]) != "DIRC" {
		return false, errors.New("the index does not start with DIRC")
	}
	// バージョン 4 は名前の先頭を前の項目と共有し、項目の後ろを NUL で揃えない。
	const oldestVersion, extendedVersion, prefixedVersion = 2, 3, 4
	version := binary.BigEndian.Uint32(index[4:8])
	if version < oldestVersion || version > prefixedVersion {
		return false, fmt.Errorf("the index version %d is not supported", version)
	}
	count := binary.BigEndian.Uint32(index[8:12])
	at, previous := headerSize, ""
	truncated := errors.New("the index ends inside an entry")
	for range count {
		start := at
		at += statSize + objectSize
		if at+flagsSize > len(index) {
			return false, truncated
		}
		flags := binary.BigEndian.Uint16(index[at:])
		at += flagsSize
		if version >= extendedVersion && flags&extendedFlag != 0 {
			at += flagsSize
		}
		if at >= len(index) {
			return false, truncated
		}
		var name string
		if version == prefixedVersion {
			strip, next, ok := offsetVarint(index, at)
			if !ok || strip > uint64(len(previous)) {
				return false, truncated
			}
			end := bytes.IndexByte(index[next:], 0)
			if end < 0 {
				return false, truncated
			}
			kept := len(previous) - int(strip) // #nosec G115 -- strip は len(previous) 以下であることを確かめた。
			name = previous[:kept] + string(index[next:next+end])
			at = next + end + 1
		} else {
			end := bytes.IndexByte(index[at:], 0)
			if end < 0 || (int(flags&nameMask) < nameMask && end != int(flags&nameMask)) {
				return false, truncated
			}
			name = string(index[at : at+end])
			// バージョン 2 と 3 の項目は、名前の後ろの 1 から 8 byte の NUL で 8 byte の倍数に揃う。
			at = start + (at-start+end+8)&^7
		}
		if relative == "." || name == relative || strings.HasPrefix(name, relative+"/") {
			return true, nil
		}
		previous = name
	}
	return false, nil
}

// offsetVarint は git の index のバージョン 4 が名前の前に置く数を読む。数は 7 bit ずつ上位から並び、
// 続く byte があるたびに 1 を足してから桁を送る。
func offsetVarint(data []byte, at int) (value uint64, next int, ok bool) {
	if at >= len(data) {
		return 0, at, false
	}
	const more, bits = 0x80, 0x7f
	value = uint64(data[at] & bits)
	for data[at]&more != 0 {
		at++
		if at >= len(data) {
			return 0, at, false
		}
		value = (value+1)<<7 | uint64(data[at]&bits)
	}
	return value, at + 1, true
}

func resolveFrom(base, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(base, path)
}

// commitOf は 16 進の commit の文字列を確かめて返す。SHA-1 の 40 文字と SHA-256 の 64 文字を受け付ける。
func commitOf(text string) (string, error) {
	const sha1Length, sha256Length = 40, 64
	if len(text) != sha1Length && len(text) != sha256Length {
		return "", fmt.Errorf("the git HEAD %q is not a commit", text)
	}
	for _, char := range text {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return "", fmt.Errorf("the git HEAD %q is not a commit", text)
		}
	}
	return text, nil
}
