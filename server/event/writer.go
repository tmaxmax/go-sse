package event

import (
	"io"
	"log"

	"github.com/tmaxmax/go-sse/internal/parser"
)

var newline = []byte{'\n'}

type fieldWriter struct {
	w io.Writer
	s parser.ChunkScanner

	isNewLine bool // indicates if the next write will start a new line
}

func panicWrite(w io.Writer, p []byte) int {
	n, err := w.Write(p)
	if err != nil {
		panic(err)
	}
	return n
}

func (s *fieldWriter) writeName(name []byte) int {
	if s.isNewLine {
		s.isNewLine = false
		return panicWrite(s.w, name)
	}

	return 0
}

func (s *fieldWriter) writeEnd() int {
	if s.isNewLine {
		return 0
	}
	return panicWrite(s.w, newline)
}

func (s *fieldWriter) writeField(f field) (n int, err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Println(r)
			if e, ok := r.(error); ok {
				err = e
			} else {
				panic(r)
			}
		}
	}()

	s.isNewLine = true
	name, repr, singleLine := f.repr()

	if len(repr) == 0 {
		return
	}

	if singleLine {
		n = s.writeName(name) + panicWrite(s.w, repr) + s.writeEnd()
		return
	}

	s.s.Buffer = repr

	var chunk []byte

	for s.s.Scan() {
		n += s.writeName(name)

		chunk, s.isNewLine = s.s.Chunk()

		n += panicWrite(s.w, chunk)
	}

	n += s.writeEnd()

	return
}

var (
	fieldBytesData    = []byte(parser.FieldNameData + ": ")
	fieldBytesEvent   = []byte(parser.FieldNameEvent + ": ")
	fieldBytesRetry   = []byte(parser.FieldNameRetry + ": ")
	fieldBytesID      = []byte(parser.FieldNameID + ": ")
	fieldBytesComment = []byte{':', ' '}
)
