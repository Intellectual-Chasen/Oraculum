package main

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// openRequestedFiles は、読み込みの要求が指す file を基準の directory の下だけで開く口を返す。
//
// **基準の外を指す path を退ける。** 要求の path は基準からの相対 path に限り、`..` で外へ
// 出る path と、基準の外を指す symbolic link と、途中で基準の外へ出てから戻る symbolic link と、
// 絶対 path の symbolic link を os.Root が退ける。基準の directory から上へ出ずに辿れる相対 path
// の symbolic link は開ける。返す io.Closer は基準を閉じる。
func openRequestedFiles(sourceRoot string) (*pipeline.RequestedFiles, io.Closer, error) {
	root, err := os.OpenRoot(sourceRoot)
	if err != nil {
		return nil, nil, fmt.Errorf("opening the source root %q: %w", sourceRoot, err)
	}
	return &pipeline.RequestedFiles{
		Upload: func(batch, name string, body io.Reader) (string, error) {
			if err := requireLocalPath(name); err != nil {
				return "", err
			}
			if batch == "" {
				batch = rand.Text()
			}
			if !validUploadBatch(batch) {
				return "", pipeline.ErrRequestedPathOutsideBase
			}
			dir := filepath.Join("oraculum-uploads", batch)
			if err := root.MkdirAll(filepath.Join(dir, filepath.Dir(name)), uploadDirectoryMode); err != nil {
				return "", err
			}
			originPath := filepath.Join(dir, name)
			file, err := root.OpenFile(originPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, uploadFileMode)
			if err != nil {
				return "", err
			}
			_, copyErr := io.Copy(file, body)
			closeErr := file.Close()
			if err := errors.Join(copyErr, closeErr); err != nil {
				_ = root.Remove(originPath)
				_ = root.Remove(filepath.Dir(originPath))
				_ = root.Remove(dir)
				return "", err
			}
			return filepath.ToSlash(originPath), nil
		},
		// os.Root の file system は、基準の外へ出る path と symbolic link を退ける。
		FS: root.FS(),
		Open: func(originPath string) (io.ReadCloser, error) {
			if err := requireLocalPath(originPath); err != nil {
				return nil, err
			}
			file, _, err := openRegularFile(root, originPath)
			if err != nil {
				return nil, err
			}
			return file, nil
		},
		// **種類を確かめてから開く。** 通常の file でない path (FIFO、device file) を開かずに
		// 退ける。開くのは、読む権限の無い file を読み込みを始める前に退けるためである。
		Size: func(originPath string) (int64, error) {
			if err := requireLocalPath(originPath); err != nil {
				return 0, err
			}
			if err := requireRegularFile(root, originPath); err != nil {
				return 0, err
			}
			file, info, err := openRegularFile(root, originPath)
			if err != nil {
				return 0, err
			}
			_ = file.Close() // 大きさは読み終えている。読み取り専用の file を閉じる失敗は確認の結果を変えない。
			return info.Size(), nil
		},
	}, root, nil
}

const uploadDirectoryMode = 0o700
const uploadFileMode = 0o600

// validUploadBatch は、1 回の選択で送った資料を同じ directory に保存する識別子を検査する。
func validUploadBatch(batch string) bool {
	if len(batch) == 0 || len(batch) > 64 {
		return false
	}
	for _, c := range batch {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-':
		default:
			return false
		}
	}
	return true
}

// openRegularFile は通常の file だけを開き、開いた file の情報と一緒に返す。
//
// **開いた後の file で通常の file かを確かめる。** Size の確認の後に同じ名前が FIFO へ
// 差し替えられても、O_NONBLOCK で開くため書き手を待って止まらない。
func openRegularFile(root *os.Root, originPath string) (*os.File, os.FileInfo, error) {
	file, err := root.OpenFile(originPath, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, nil, requestedFileError(originPath, err)
	}
	info, err := file.Stat()
	if err == nil && !info.Mode().IsRegular() {
		err = fmt.Errorf("the source %q: %w", originPath, pipeline.ErrRequestedFileNotRegular)
	} else if err != nil {
		err = requestedFileError(originPath, err)
	}
	if err != nil {
		_ = file.Close() // 開いた file の検査の失敗を返す。閉じる失敗は検査の失敗に含めない。
		return nil, nil, err
	}
	return file, info, nil
}

// requireRegularFile は、path が指す先を開かずに、通常の file だけを通す。
func requireRegularFile(root *os.Root, originPath string) error {
	info, err := root.Stat(originPath)
	if err != nil {
		return requestedFileError(originPath, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("the source %q: %w", originPath, pipeline.ErrRequestedFileNotRegular)
	}
	return nil
}

// requireLocalPath は、基準の directory の下に留まる相対 path だけを通す。
func requireLocalPath(originPath string) error {
	if !filepath.IsLocal(originPath) {
		return fmt.Errorf("the source %q is not a path under the base directory: %w",
			originPath, pipeline.ErrRequestedPathOutsideBase)
	}
	return nil
}

// requestedFileError は開けなかった理由を、分類を errors.Is で辿れる形にする。
//
// **「無い」と「読めない」と「基準の外を指す」を分ける。** 分析者が次に採る手が異なる。
// 分類に該当しない失敗は、元の失敗を %w で残す。元の失敗の文字列は基準の directory の path を
// 含みうるため、要求の応答には載せない (pipeline.LoadingRequestError.Message)。
func requestedFileError(originPath string, err error) error {
	switch {
	case errors.Is(err, os.ErrNotExist):
		return fmt.Errorf("the source %q does not exist under the base directory: %w", originPath, os.ErrNotExist)
	case errors.Is(err, os.ErrPermission):
		return fmt.Errorf("the source %q is not readable: %w", originPath, os.ErrPermission)
	case escapesRoot(err):
		return fmt.Errorf("the source %q or a symbolic link on its path leaves the base directory or is absolute: %w",
			originPath, pipeline.ErrRequestedPathOutsideBase)
	default:
		return fmt.Errorf("the source %q cannot be opened under the base directory: %w", originPath, err)
	}
}

// escapesRoot は、os.Root が基準の外への脱出を理由に退けた失敗であるかを返す。
//
// **os.Root は脱出を、OS の errno を持たない失敗で返す。** 脱出を指す公開の識別値は無い。
// OS が返した失敗 (ENOTDIR、ELOOP など) は errno を持つため、脱出に数えない。閉じた基準の
// 失敗 (os.ErrClosed) も脱出に数えない。
//
// **EXDEV を脱出に数える。** openat2 の RESOLVE_BENEATH は脱出を EXDEV で退ける。Go 1.26 の
// os.Root は openat2 を呼ばないので、この分岐は、将来の os.Root が EXDEV をそのまま返す場合に
// 備える。toolchain を上げたときに、os.Root の脱出の返し方を os/root_*.go で確かめる。
func escapesRoot(err error) bool {
	if errors.Is(err, syscall.EXDEV) {
		return true
	}
	var errno syscall.Errno
	return !errors.As(err, &errno) && !errors.Is(err, os.ErrClosed)
}
