package parser

import "github.com/tmaxmax/go-sse/internal/util"

// NewlineIndex returns the index of the first occurrence of a newline sequence (\n, \r, or \r\n).
// It also returns the sequence's length. If no sequence is found, index is equal to len(s)
// and length is 0.
func NewlineIndex(s []byte) (index int, length int) {
	for l := len(s); index < l; index++ {
		b := s[index]

		if util.IsNewlineChar(b) {
			length++
			if b == '\r' && index < l-1 && s[index+1] == '\n' {
				length++
			}

			break
		}
	}

	return
}

// ChunkScanner reads through the lines of a byte slice. The returned chunks include the line endings.
// A line's ending is specified in the Event Stream standard's documentation:
// https://html.spec.whatwg.org/multipage/server-sent-events.html#server-sent-events
type ChunkScanner struct {
	// The buffer to split into chunks. When Scan returns false,
	// the same ChunkScanner can be reused by simply changing this buffer.
	buffer []byte

	index, length int
}

// Scan retrieves the next chunk from the buffer. It returns true if there is data left in the buffer.
// Calling Scan multiple times after there is no data will change the buffer!
func (s *ChunkScanner) Scan() bool {
	if s.index == len(s.buffer) {
		return false
	}

	s.buffer = s.buffer[s.index+s.length:]
	s.index, s.length = NewlineIndex(s.buffer)

	return true
}

// Chunk returns the last scanned chunk of data. It also returns a flag that indicates
// whether the data ends in a newline sequence.
//
// The returned chunk is nil only if no scan was run or if there was nothing to read
// from the provided source, otherwise its length is always greater than 0.
func (s *ChunkScanner) Chunk() (chunk []byte, endsInNewline bool) {
	return s.buffer[:s.index+s.length], s.length != 0
}

// Reset changes the scanner's buffer. Calling Scan after reset will return the first
// chunk from this buffer.
func (s *ChunkScanner) Reset(buffer []byte) {
	s.buffer = buffer
	s.index = 0
	s.length = 0
}

func NewChunkScanner(buffer []byte) ChunkScanner {
	return ChunkScanner{buffer: buffer}
}
