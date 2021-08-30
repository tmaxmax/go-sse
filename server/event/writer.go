package event

import (
	"io"

	"github.com/tmaxmax/go-sse/internal/parser"
	"github.com/tmaxmax/go-sse/internal/util"
)

var newline = []byte{'\n'}
var colon = []byte{':', ' '}

type fieldWriter struct {
	w io.Writer
	s parser.ChunkScanner

	isNewLine bool // indicates if the next write will start a new line
}

func (s *fieldWriter) writeName(name parser.FieldName) (n int, err error) {
	if s.isNewLine {
		var m int
		m, err = io.WriteString(s.w, string(name))
		n += m
		if err != nil {
			return
		}
		m, err = s.w.Write(colon)
		n += m
		if err != nil {
			return
		}

		s.isNewLine = false
	}

	return
}

func (s *fieldWriter) writeField(f field) (int, error) {
	s.isNewLine = true

	name := f.name()

	n, err := s.writeName(name)
	if err != nil {
		return n, err
	}

	var (
		m     int
		chunk []byte
	)

	s.s.Buffer = f.repr()

	for s.s.Scan() {
		m, err = s.writeName(name)
		n += m
		if err != nil {
			return n, err
		}

		chunk, s.isNewLine = s.s.Chunk()

		m, err = s.w.Write(chunk)
		n += m
		if err != nil {
			// Set isNewLine to true on incomplete writes so writeEnd tries to insert the field's endline
			// even if the write errored here. If writeEnd succeeds it ensures that the output won't break
			// the protocol, at least.
			// We are checking for endline characters and not for difference in chunk length and written
			// bytes count because an incomplete write of a chunk that ends in \r\n could have \r written,
			// or all the bytes could be written in spite of the error.
			s.isNewLine = m > 0 && util.IsNewlineChar(chunk[m-1])

			return n, err
		}
	}

	if !s.isNewLine {
		m, err = s.w.Write(newline)
		n += m
	}

	return n, err
}
