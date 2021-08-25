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

func newSingleFieldWriter(w io.Writer, f field) *singleFieldWriter {
	return &singleFieldWriter{
		w:         w,
		s:         &ChunkScanner{},
		name:      []byte(f.name() + ":"),
		isNewLine: true,
	}
}

func (s *singleFieldWriter) writeName() (n int, err error) {
	if s.isNewLine {
		n, err = s.w.Write(s.name)

		s.isNewLine = false
	}

	return
}

// Write here does not implement the io.Write interface correctly,
// as on return n > len(p). Use cautiously!
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
			// Set isNewLine to true on incomplete writes so Close tries to insert the field's endline
			// even if the write errored here. If Close succeeds it ensures that the output won't break
			// the protocol, at least.
			// We are checking for endline characters and not for difference in chunk length and written
			// bytes count because an incomplete write of a chunk that ends in \r\n could have \r written,
			// or all the bytes could be written in spite of the error.
			s.isNewLine = m > 0 && (chunk[m-1] == '\n' || chunk[m-1] == '\r')

			return
		}
	}

	return
}

func (s *singleFieldWriter) Close() (err error) {
	if !s.isNewLine {
		s.isNewLine = true
		s.charsWrittenOnClose, err = s.w.Write(newline)
	}

	return
}

// writer is a struct that is used to write event fields.
type writer struct {
	Writer io.Writer

	closed         bool
	writtenOnClose int
}

func (w *writer) WriteField(f field) (n int64, err error) {
	s := newSingleFieldWriter(w.Writer, f)
	defer func() {
		n += int64(s.charsWrittenOnClose)
	}()
	defer s.Close()

	n, err = f.WriteTo(s)
	if err != nil {
		return
	}

	err = s.Close()

	return
}

func (w *writer) Close() (err error) {
	if w.closed {
		return nil
	}

	w.closed = true

	w.writtenOnClose, err = w.Writer.Write(newline)

	return
}
