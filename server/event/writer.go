package event

import (
	"io"

	"github.com/tmaxmax/go-sse/internal/parser"
)

var newline = []byte{'\n'}
var colon = []byte{':', ' '}

type fieldWriter struct {
	w io.Writer
	s parser.ChunkScanner

	isNewLine bool // indicates if the next write will start a new line
}

func panicWriteString(w io.Writer, s string) int {
	n, err := io.WriteString(w, s)
	if err != nil {
		panic(err)
	}
	return n
}

func panicWrite(w io.Writer, p []byte) int {
	n, err := w.Write(p)
	if err != nil {
		panic(err)
	}
	return n
}

func (s *fieldWriter) writeName(name parser.FieldName) int {
	if s.isNewLine {
		s.isNewLine = false
		return panicWriteString(s.w, string(name)) + panicWrite(s.w, colon)
	}

	return 0
}

func (s *fieldWriter) writeField(f field) (n int, err error) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(error); ok {
				err = e
			} else {
				panic(r)
			}
		}
	}()

	s.isNewLine = true
	s.s.Buffer = f.repr()
	name := f.name()
	var chunk []byte

	for s.s.Scan() {
		n += s.writeName(name)

		chunk, s.isNewLine = s.s.Chunk()

		n += panicWrite(s.w, chunk)
	}

	if !s.isNewLine {
		n += panicWrite(s.w, newline)
	}

	return
}
