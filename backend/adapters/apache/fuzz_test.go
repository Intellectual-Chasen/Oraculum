package apache_test

import (
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/apache"
)

// FuzzAccessReader は任意の byte 列を AccessReader に渡してもパニックしないことを確かめる。
// 失敗の分類は問わない。文字列の分割の手書きの範囲検査が原資料の任意の byte 列で異常終了しないことが
// 対象である。
func FuzzAccessReader(f *testing.F) {
	f.Add([]byte(fullShapeLine))
	f.Add([]byte(`[[[[[[[[`))
	f.Add([]byte(`"""""""`))
	f.Add([]byte(""))
	f.Add([]byte("\x00\x01\x02"))
	f.Fuzz(func(t *testing.T, data []byte) {
		var reader apache.AccessReader
		reader.Reset(newSingleLineReader(string(data)))
		for i := 0; i < 100; i++ {
			_, _, err := reader.Next()
			if err != nil {
				break
			}
		}
	})
}

// FuzzErrorReader は任意の byte 列を ErrorReader に渡してもパニックしないことを確かめる。
func FuzzErrorReader(f *testing.F) {
	f.Add([]byte(errorFullShapeLine))
	f.Add([]byte(`[[[[[[[[`))
	f.Add([]byte(`[pid tid]`))
	f.Add([]byte(""))
	f.Add([]byte("\x00\x01\x02"))
	f.Fuzz(func(t *testing.T, data []byte) {
		var reader apache.ErrorReader
		reader.Reset(newSingleLineReader(string(data)))
		for i := 0; i < 100; i++ {
			_, _, err := reader.Next()
			if err != nil {
				break
			}
		}
	})
}
