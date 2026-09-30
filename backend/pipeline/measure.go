package pipeline

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

func measureWholeSource(input io.Reader) (sourceMeasurement, error) {
	digest := sha256.New()
	endings := newlineMeasurement{}
	size, err := io.Copy(io.MultiWriter(digest, &endings), input)
	if err != nil {
		return sourceMeasurement{}, fmt.Errorf("measuring the whole source: %w", err)
	}
	return sourceMeasurement{
		ContentSha256: hex.EncodeToString(digest.Sum(nil)),
		SizeBytes:     size, NewlineCount: endings.lf + endings.crlf,
		EndsWithNewline: endings.previous == '\n', LineEnding: endings.kind(),
	}, nil
}

type newlineMeasurement struct {
	previous byte
	lf       int64
	crlf     int64
}

func (m *newlineMeasurement) Write(data []byte) (int, error) {
	for _, value := range data {
		if value == '\n' {
			if m.previous == '\r' {
				m.crlf++
			} else {
				m.lf++
			}
		}
		m.previous = value
	}
	return len(data), nil
}

func (m newlineMeasurement) kind() core.LineEnding {
	switch {
	case m.lf > 0 && m.crlf > 0:
		return core.LineEndingMixed
	case m.crlf > 0:
		return core.LineEndingCrlf
	case m.lf > 0:
		return core.LineEndingLf
	default:
		return core.LineEndingUndetermined
	}
}
