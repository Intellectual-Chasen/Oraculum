package pipeline

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// UploadRequestedFile は PC からの資料を保存し、その資料と関連する付属資料だけを判定する。
func (s *Stages) UploadRequestedFile(ctx context.Context, batch, name string, body io.Reader) (core.SourceUploadResult, error) {
	if !fs.ValidPath(name) || !filepath.IsLocal(name) || strings.Contains(name, "\\") || hasControlCharacter(name) {
		return core.SourceUploadResult{}, &LoadingRequestError{Rejection: core.LoadingRejectionPathOutsideBase, err: errors.New("the upload path must be relative")}
	}
	if s.config.Requested == nil || s.config.Requested.Upload == nil {
		return core.SourceUploadResult{}, &LoadingRequestError{Rejection: core.LoadingRejectionNoBaseDirectory, err: errors.New("uploads are unavailable")}
	}
	if batch != "" && (!fs.ValidPath(batch) || strings.ContainsAny(batch, "/\\") || hasControlCharacter(batch) || len(batch) > 64) {
		return core.SourceUploadResult{}, &LoadingRequestError{Rejection: core.LoadingRejectionPathOutsideBase, err: errors.New("invalid upload identifier")}
	}
	originPath, err := s.config.Requested.Upload(batch, name, body)
	if err != nil {
		if errors.Is(err, ErrRequestedPathOutsideBase) {
			return core.SourceUploadResult{}, &LoadingRequestError{Rejection: core.LoadingRejectionPathOutsideBase, err: errors.New("invalid upload identifier")}
		}
		return core.SourceUploadResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return core.SourceUploadResult{}, err
	}
	info, err := fs.Stat(s.config.Requested.FS, originPath)
	if err != nil {
		return core.SourceUploadResult{}, err
	}
	lister := sourceFileLister{ctx: ctx, fsys: s.config.Requested.FS, open: s.config.Requested.Open, detector: newFormatDetector(s.config.Import.Parsers)}
	entries := []core.SourceFileEntry{lister.entry(originPath, fs.FileInfoToDirEntry(info))}
	seen := map[string]bool{originPath: true}
	addRelated := func(name string) {
		if seen[name] {
			return
		}
		seen[name] = true
		info, err := fs.Stat(lister.fsys, name)
		if err == nil && info.Mode().IsRegular() {
			entries = append(entries, lister.entry(name, fs.FileInfoToDirEntry(info)))
		}
	}
	// 付属資料を送ったときは、既に保存した主資料の形式も判定に使う。
	for _, format := range lister.detector.formats {
		for _, suffix := range format.suffixes {
			if strings.HasSuffix(originPath, suffix) {
				addRelated(strings.TrimSuffix(originPath, suffix))
			}
		}
	}
	// 主資料を送ったときは、先に送った付属資料の分類も更新する。
	for _, entry := range entries {
		for _, key := range entry.FormatCandidates {
			for _, suffix := range lister.detector.companionSuffixes(key) {
				if !strings.HasSuffix(entry.OriginPath, suffix) {
					addRelated(entry.OriginPath + suffix)
				}
			}
		}
	}
	markCompanions(lister.detector, entries)
	return core.SourceUploadResult{Entries: entries}, nil
}
