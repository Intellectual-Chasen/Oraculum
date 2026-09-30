// in-package test: 非公開の収集元測定を直接検査する。
package pipeline

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

func TestMeasureWholeSource(t *testing.T) {
	data, err := os.ReadFile("testdata/scan/measure.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name            string
		Input           string
		Sha256          string
		Size            int64
		Newlines        int64
		EndsWithNewline bool
		Ending          core.LineEnding
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("measurement manifest is empty")
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			for _, mode := range []string{"whole", "one_byte", "data_with_eof"} {
				t.Run(mode, func(t *testing.T) {
					var input io.Reader = strings.NewReader(tc.Input)
					switch mode {
					case "one_byte":
						input = iotest.OneByteReader(input)
					case "data_with_eof":
						input = iotest.DataErrReader(input)
					}
					got, err := measureWholeSource(input)
					if err != nil {
						t.Fatal(err)
					}
					if got.ContentSha256 != tc.Sha256 || got.SizeBytes != tc.Size {
						t.Errorf("content measurement = %s / %d, want %s / %d", got.ContentSha256, got.SizeBytes, tc.Sha256, tc.Size)
					}
					if got.NewlineCount != tc.Newlines || got.EndsWithNewline != tc.EndsWithNewline || got.LineEnding != tc.Ending {
						t.Errorf("line measurement = %d / %t / %s, want %d / %t / %s", got.NewlineCount, got.EndsWithNewline, got.LineEnding, tc.Newlines, tc.EndsWithNewline, tc.Ending)
					}
				})
			}
		})
	}
}

func TestMeasureWholeSourceReadFailure(t *testing.T) {
	cause := errors.New("source interrupted")
	for _, prefix := range []string{"", "a\nb"} {
		t.Run(prefix, func(t *testing.T) {
			input := io.MultiReader(strings.NewReader(prefix), iotest.ErrReader(cause))
			got, err := measureWholeSource(input)
			if !errors.Is(err, cause) {
				t.Errorf("error = %v, want source interruption", err)
			}
			if got != (sourceMeasurement{}) {
				t.Errorf("incomplete source produced measurement: %+v", got)
			}
		})
	}
}
