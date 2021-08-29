package parser

import (
	"bufio"
	"io"
)

var splitFunc bufio.SplitFunc = func(data []byte, _ bool) (advance int, token []byte, err error) {
	if len(data) == 0 {
		return
	}

	var chunk []byte
	remaining, start := data, 0
	for {
		chunk, remaining = nextChunk(remaining)
		advance += len(chunk)
		chunkLen := len(trimNewline(chunk))
		if chunkLen == 0 {
			start++
		}
		if len(remaining) == 0 || (isNewlineChar(remaining[0]) && chunkLen > 0) {
			break
		}
	}

	if rl := len(remaining); rl > 0 {
		advance++
		if rl > 1 && remaining[0] == '\r' && remaining[1] == '\n' {
			advance++
		}
	}

	token = data[start:advance]

	return
}

type ReaderParser struct {
	sc *bufio.Scanner
	p  *ByteParser
}

// Scan parses a single field from the reader. It returns false when there are no more fields to parse.
func (r *ReaderParser) Scan() bool {
	if !r.p.Scan() {
		if !r.sc.Scan() {
			return false
		}

		r.p.cs.Buffer = r.sc.Bytes()
		if !r.p.Scan() {
			return false
		}
	}

	return true
}

// Field returns the last parsed field.
func (r *ReaderParser) Field() Field {
	return r.p.Field()
}

// Err returns the last read error.
func (r *ReaderParser) Err() error {
	return r.sc.Err()
}

func NewReaderParser(r io.Reader) *ReaderParser {
	sc := bufio.NewScanner(r)
	sc.Split(splitFunc)

	return &ReaderParser{sc: sc, p: NewByteParser(nil)}
}
