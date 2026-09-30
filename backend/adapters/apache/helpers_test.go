package apache_test

import (
	"io"
	"strings"
)

// newSingleLineReader は 1 行分の原文に LF を付けた io.Reader を返す。
func newSingleLineReader(line string) io.Reader {
	return strings.NewReader(line + "\n")
}
