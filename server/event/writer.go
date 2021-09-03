package event

import (
	"io"

	"github.com/tmaxmax/go-sse/internal/parser"
)

var newline = []byte{'\n'}

type writeError struct {
	error
}

func panicWrite(w io.Writer, p []byte) int64 {
	n, err := w.Write(p)
	if err != nil {
		panic(writeError{err})
	}
	return int64(n)
}

func (f *Field) WriteTo(w io.Writer) (n int64, err error) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(writeError); ok {
				err = e.error
			} else {
				panic(r)
			}
		}
	}()

	if len(f.data) == 0 {
		return
	}

	if f.singleLine {
		n = panicWrite(w, f.nameBytes) + panicWrite(w, f.data) + panicWrite(w, newline)
		return
	}

	s := parser.ChunkScanner{Buffer: f.data}
	var chunk []byte
	var isNewLine bool

	for s.Scan() {
		n += panicWrite(w, f.nameBytes)
		chunk, isNewLine = s.Chunk()
		n += panicWrite(w, chunk)
	}

	if !isNewLine {
		n += panicWrite(w, newline)
	}

	return
}
