package parser

import "github.com/tmaxmax/go-sse/internal/util"

// nextChunk returns the first chunk of data that ends in a newline sequence.
// It includes the newline sequence. It also returns the remaining data after
// the chunk.
//
// chunk.data is nil only if s is nil. if there is no data left, remaining is
// nil only if chunk does not end in a newline. It has length 0 otherwise.
func nextChunk(s []byte) (chunk, remaining []byte) {
	// if no line ending is found, chunk is s
	chunk = s

	l := len(s)
	i := 0

	for i < l {
		b := s[i]

		i++

		if util.IsNewlineChar(b) {
			if b == '\r' && i < l && s[i] == '\n' {
				i++
			}

			chunk = s[:i]
			remaining = s[i:]

			break
		}
	}

	return
}

// ChunkScanner reads through the lines of a byte slice. The returned chunks include the line endings.
// A line's ending is specified in the Event Stream standard's documentation:
// https://html.spec.whatwg.org/multipage/server-sent-events.html#server-sent-events
type ChunkScanner struct {
	chunk []byte // the last scanned chunk

	// The buffer to split into chunks. When Scan returns false,
	// the same ChunkScanner can be reused by simply changing this buffer.
	Buffer []byte
}

// Scan retrieves the next chunk from the buffer. It returns true if there is data left in the buffer.
// Calling Scan multiple times after there is no data will change the buffer!
func (s *ChunkScanner) Scan() bool {
	s.chunk, s.Buffer = nextChunk(s.Buffer)

	return len(s.chunk) != 0
}

// Chunk returns the last scanned chunk of data. It also returns a flag that indicates
// whether the data ends in a newline sequence.
//
// The returned chunk is nil only if no scan was run or if there was nothing to read
// from the provided source, otherwise its length is always greater than 0.
func (s *ChunkScanner) Chunk() (chunk []byte, endsInNewline bool) {
	return s.chunk, s.Buffer != nil
}
