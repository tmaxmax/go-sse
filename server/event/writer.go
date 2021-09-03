package event

import (
	"io"

	"github.com/tmaxmax/go-sse/internal/parser"
)

var newline = []byte{'\n'}

func (f *Field) WriteTo(w io.Writer) (n int64, err error) {
	if len(f.data) == 0 {
		return 0, nil
	}

	var m int

	if f.singleLine {
		m, err = w.Write(f.nameBytes)
		n += int64(m)
		if err != nil {
			return
		}
		m, err = w.Write(f.data)
		n += int64(m)
		if err != nil {
			return
		}
		m, err = w.Write(newline)
		n += int64(m)
		return
	}

	s := parser.NewChunkScanner(f.data)
	var chunk []byte
	var isNewLine bool

	for s.Scan() {
		m, err = w.Write(f.nameBytes)
		n += int64(m)
		if err != nil {
			return
		}
		chunk, isNewLine = s.Chunk()
		m, err = w.Write(chunk)
		n += int64(m)
		if err != nil {
			return
		}
	}

	if !isNewLine {
		m, err = w.Write(newline)
		n += int64(m)
	}

	return
}
