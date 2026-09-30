package royalts_test

import (
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/royalts"
)

// FuzzReader は任意の byte 列を Reader に渡してもパニックしないことを確かめる。
// 失敗の分類は問わない。
func FuzzReader(f *testing.F) {
	f.Add([]byte(singleConnectionDocument))
	f.Add([]byte(`<RoyalRDSConnection>`))
	f.Add([]byte(""))
	f.Add([]byte("\xEF\xBB\xBF<RTSZDocument/>"))
	f.Add([]byte("\x00\x01\x02"))
	f.Fuzz(func(t *testing.T, data []byte) {
		var reader royalts.Reader
		reader.Reset(strings.NewReader(string(data)))
		for i := 0; i < 100; i++ {
			_, _, err := reader.Next()
			if err != nil {
				break
			}
		}
	})
}
