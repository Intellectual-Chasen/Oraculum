// in-package test: 任意の byte 列を読んでも止まらずに返ることを確かめる。
package prefetch

import (
	"bytes"
	"testing"
	"time"
)

// FuzzReader は任意の byte 列を Reader に渡しても、パニックせず時間の上限の中で返ることを
// 確かめる。成功したレコードは項目へ直す。失敗の分類は問わない。
func FuzzReader(f *testing.F) {
	plain := sampleFile(30, 0x130).build()
	f.Add(plain)
	f.Add(compressedFile(plain, false))
	f.Add(compressedFile(sampleFile(17, 0x98).build(), true))
	f.Add([]byte("MAM\x04\xff\xff\x00\x00"))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		done := make(chan struct{})
		go func() {
			defer close(done)
			var reader Reader
			reader.Reset(bytes.NewReader(data))
			for range 2 {
				record, failure, err := reader.Next()
				if err != nil {
					return
				}
				if failure == nil {
					Fields(record.File)
					LastRunTime(record.File)
				}
			}
		}()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatalf("reading %d bytes did not return within the limit", len(data))
		}
	})
}
