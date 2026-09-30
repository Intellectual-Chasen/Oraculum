// in-package test: 任意の byte 列を読んでも止まらずに返ることを確かめる。
package winregistry

import (
	"bytes"
	"testing"
	"time"
)

// FuzzReader は任意の byte 列を主 file と log に分けて Reader に渡しても、パニックせず時間の
// 上限の中で返ることを確かめる。split は主 file の byte 数である。失敗の分類は問わない。
func FuzzReader(f *testing.F) {
	oldBins, root := synthHive(oldText, false)
	newBins, _ := synthHive(newText, true)
	primary := primaryFile(oldBins, 11, 10, root)
	log := logBytes(root, synthEntry{sequence: 11, binsSize: uint32(len(newBins)), pages: changedPages(oldBins, newBins)})
	f.Add(append(bytes.Clone(primary), log...), uint32(len(primary)))
	f.Add(primaryFile(oldBins, 1, 1, root), uint32(0))
	f.Add([]byte("regf"), uint32(4))
	f.Fuzz(func(t *testing.T, data []byte, split uint32) {
		split = min(split, uint32(len(data)))
		done := make(chan struct{})
		go func() {
			defer close(done)
			var reader Reader
			reader.SetMembers([]Member{
				{Name: "h", Size: int64(split)},
				{Name: "h.LOG1", Offset: int64(split), Size: int64(len(data)) - int64(split)},
			})
			reader.Reset(bytes.NewReader(data))
			for {
				if _, _, err := reader.Next(); err != nil {
					break
				}
			}
			reader.SourceHeader()
		}()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatalf("reading %d bytes did not return within the limit", len(data))
		}
	})
}
