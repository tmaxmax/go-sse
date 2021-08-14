package field

import (
	"io"

	"github.com/tmaxmax/go-sse/sse/server/field/internal"
)

var newline = []byte{'\n'}

type singleFieldWriter struct {
	w io.Writer
	s *internal.ChunkScanner

	name      []byte // this also has the colon
	isNewLine bool   // indicates if the next write will start a new line

	charsWrittenOnClose int
}

func newSingleFieldWriter(w io.Writer, f Field) *singleFieldWriter {
	return &singleFieldWriter{
		w:                   w,
		s:                   &internal.ChunkScanner{},
		name:                []byte(f.name() + ":"),
		isNewLine:           true,
		charsWrittenOnClose: -1,
	}
}

func (s *singleFieldWriter) writeName() (n int, err error) {
	if s.isNewLine {
		n, err = s.w.Write(s.name)

		s.isNewLine = false
	}

	return
}

func (s *singleFieldWriter) Write(p []byte) (n int, err error) {
	n, err = s.writeName()
	if err != nil {
		return
	}

	var (
		m     int
		chunk []byte
	)

	s.s.Buffer = p

	for s.s.Scan() {
		m, err = s.writeName()
		n += m
		if err != nil {
			return
		}

		chunk, s.isNewLine = s.s.Chunk()

		m, err = s.w.Write(chunk)
		n += m
		if err != nil {
			return
		}
	}

	return
}

func (s *singleFieldWriter) End() (int, error) {
	if !s.isNewLine {
		s.isNewLine = false

		return s.w.Write(newline)
	}

	return 0, nil
}

// Writer is a struct that is used to write event fields.
type Writer struct {
	Writer io.Writer
}

func (w *Writer) WriteField(f Field) (int64, error) {
	s := newSingleFieldWriter(w.Writer, f)

	n, err := f.WriteTo(s)
	if err != nil {
		return n, err
	}

	m, err := s.End()

	return n + int64(m), err
}

func (w *Writer) End() (int64, error) {
	n, err := w.Writer.Write(newline)

	return int64(n), err
}
