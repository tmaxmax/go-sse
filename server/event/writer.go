package event

import (
	"io"

	. "github.com/tmaxmax/go-sse/server/event/internal"
)

var newline = []byte{'\n'}

type singleFieldWriter struct {
	w io.Writer
	s *ChunkScanner

	name      []byte // this also has the colon
	isNewLine bool   // indicates if the next write will start a new line

	charsWrittenOnClose int
}

func newSingleFieldWriter(w io.Writer, f Field) *singleFieldWriter {
	return &singleFieldWriter{
		w:                   w,
		s:                   &ChunkScanner{},
		name:                []byte(f.name() + ":"),
		isNewLine:           true,
		charsWrittenOnClose: -1,
	}
}

func (s *singleFieldWriter) writeName() (err error) {
	if s.isNewLine {
		_, err = s.w.Write(s.name)

		s.isNewLine = false
	}

	return
}

func (s *singleFieldWriter) Write(p []byte) (n int, err error) {
	if err = s.writeName(); err != nil {
		return
	}

	var (
		m     int
		chunk []byte
	)

	s.s.Buffer = p

	for s.s.Scan() {
		if err = s.writeName(); err != nil {
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

func (s *singleFieldWriter) Close() error {
	if !s.isNewLine {
		s.isNewLine = true

		_, err := s.w.Write(newline)

		return err
	}

	return nil
}

// writer is a struct that is used to write event fields.
type writer struct {
	Writer io.Writer

	closed bool
}

func (w *writer) WriteField(f Field) (int64, error) {
	s := newSingleFieldWriter(w.Writer, f)
	defer s.Close()

	n, err := f.WriteTo(s)
	if err != nil {
		return n, err
	}

	return n, s.Close()
}

func (w *writer) Close() error {
	if w.closed {
		return nil
	}

	w.closed = true

	_, err := w.Writer.Write(newline)

	return err
}
