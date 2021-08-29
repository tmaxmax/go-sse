package event

import (
	"io"
	"reflect"
	"unsafe"

	"github.com/tmaxmax/go-sse/internal/parser"
)

var newline = []byte{'\n'}
var colon = []byte{':'}

type singleFieldWriter struct {
	w io.Writer
	s parser.ChunkScanner

	name      string
	isNewLine bool // indicates if the next write will start a new line

	charsWrittenOnClose int
}

func (s *singleFieldWriter) start(name string) {
	s.name = name
	s.isNewLine = true
	s.charsWrittenOnClose = 0
}

func (s *singleFieldWriter) writeName() (n int, err error) {
	if s.isNewLine {
		var m int
		m, err = io.WriteString(s.w, s.name)
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

func unsafeGetBytes(s string) []byte {
	return (*[0x7fff0000]byte)(unsafe.Pointer(
		(*reflect.StringHeader)(unsafe.Pointer(&s)).Data),
	)[:len(s):len(s)]
}

func (s *singleFieldWriter) WriteString(v string) (int, error) {
	return s.Write(unsafeGetBytes(v))
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
	fw singleFieldWriter

	closed         bool
	writtenOnClose int
}

func (w *writer) WriteField(f field) (n int64, err error) {
	w.fw.start(f.name())
	defer func() {
		n += int64(w.fw.charsWrittenOnClose)
	}()
	defer w.fw.Close()

	n, err = f.WriteTo(&w.fw)
	if err != nil {
		return
	}

	err = w.fw.Close()

	return
}

func (w *writer) Close() (err error) {
	if w.closed {
		return nil
	}

	w.closed = true

	w.writtenOnClose, err = w.fw.w.Write(newline)

	return
}
