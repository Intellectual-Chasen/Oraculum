package winevent_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/winevent"
)

// FuzzXMLReader は任意の byte 列を XMLReader と Observe に渡してもパニックせず、走査が
// 終わることを確かめる。失敗の分類は問わない。
func FuzzXMLReader(f *testing.F) {
	f.Add([]byte(twoEventsDocument))
	f.Add([]byte(processCreationEvent))
	f.Add([]byte(`<Event><System>`))
	f.Add([]byte(`<Event></Event </Event><Event>`))
	f.Add([]byte(""))
	f.Add([]byte("\x00\x01\x02"))
	f.Fuzz(func(t *testing.T, data []byte) {
		var reader winevent.XMLReader
		reader.Reset(strings.NewReader(string(data)))
		// 1 件ごとに位置が進むため、呼び出しの数は入力の byte 数を超えない。
		for i := 0; i <= len(data)+1; i++ {
			event, failure, err := reader.Next()
			if err != nil {
				return
			}
			if failure == nil {
				winevent.Observe(event)
			}
		}
		t.Fatalf("Next() did not reach the end of %d bytes", len(data))
	})
}

// evtxFuzzTimeLimit は 1 つの入力を読み終えるまでの時間の上限である。chunk 1 つを読む
// 時間はミリ秒の桁であり、上限を超えた入力は走査が止まらない入力である。
const evtxFuzzTimeLimit = 10 * time.Second

// FuzzEVTXReader は任意の byte 列を EVTXReader と Observe に渡してもパニックせず、走査が
// 時間の上限の中で終わることを確かめる。失敗の分類は問わない。
func FuzzEVTXReader(f *testing.F) {
	chunk := newSynthChunk()
	chunk.record(synthEventAt(1, 1, "host-a.example.test"))
	chunk.record(synthEventAt(2, 2, "host-b.example.test"))
	f.Add(append(synthFileHeader(3, 1, 0), chunk.bytes()...))
	f.Add(synthFileHeader(1, 0, 0))
	f.Add([]byte("ElfFile\x00"))
	f.Add([]byte(""))
	f.Fuzz(func(t *testing.T, data []byte) {
		ended := make(chan string, 1)
		// **1 回の Next() の中で止まる入力も捉えるため、走査を別の goroutine で回す。**
		go func() {
			var reader winevent.EVTXReader
			reader.Reset(strings.NewReader(string(data)))
			// 1 件ごとに別の byte 範囲を指すため、呼び出しの数は入力の byte 数を超えない。
			for i := 0; i <= len(data)+1; i++ {
				event, failure, err := reader.Next()
				if err != nil {
					ended <- ""
					return
				}
				if failure == nil {
					winevent.Observe(event)
				}
			}
			ended <- "Next() did not reach the end"
		}()
		select {
		case problem := <-ended:
			if problem != "" {
				t.Fatalf("%s of %d bytes", problem, len(data))
			}
		case <-time.After(evtxFuzzTimeLimit):
			t.Fatalf("reading %d bytes took longer than %v", len(data), evtxFuzzTimeLimit)
		}
	})
}

// FuzzCSVReader は任意の byte 列を CSVReader と Observe に渡してもパニックせず、走査が
// 終わることを確かめる。失敗の分類は問わない。
func FuzzCSVReader(f *testing.F) {
	f.Add([]byte(levelHeader + viewerRecord("情報", "2001/02/03 04:05:06",
		"Microsoft-Windows-Security-Auditing", "4688", "", processCreationDescription)))
	f.Add([]byte(keywordHeader + "成功の監査,2001/02/03 04:05:06,x,1,,\"open\n"))
	f.Add([]byte(levelHeader + "\n\n"))
	f.Add([]byte(""))
	f.Fuzz(func(t *testing.T, data []byte) {
		var reader winevent.CSVReader
		reader.Reset(strings.NewReader(string(data)))
		for i := 0; i <= len(data)+1; i++ {
			event, failure, err := reader.Next()
			if err != nil {
				return
			}
			if failure == nil {
				winevent.Observe(event)
			}
		}
		t.Fatalf("Next() did not reach the end of %d bytes", len(data))
	})
}
