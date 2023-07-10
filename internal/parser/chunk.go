package parser

// isNewlineChar returns whether the given character is '\n' or '\r'.
func isNewlineChar(b byte) bool {
	return b == '\n' || b == '\r'
}

// NewlineIndex returns the index of the first occurrence of a newline sequence (\n, \r, or \r\n).
// It also returns the sequence's length. If no sequence is found, index is equal to len(s)
// and length is 0.
func NewlineIndex(s []byte) (index, length int) {
	for l := len(s); index < l; index++ {
		b := s[index]

		if isNewlineChar(b) {
			length++
			if b == '\r' && index < l-1 && s[index+1] == '\n' {
				length++
			}

			break
		}
	}

	return
}

// A Chunk of data that may or may not end in a newline.
//
// The newline is defined in the Event Stream standard's documentation:
// https://html.spec.whatwg.org/multipage/server-sent-events.html#server-sent-events
type Chunk struct {
	// Not owned by the Chunk instance.
	Data []byte
	// Whether the Data slice ends with a newline sequence or not.
	HasNewline bool
}

// NextChunk retrieves the next Chunk of data from the given byte slice
// along with the data remaining after the returned Chunk.
// If the returned chunk is the last one, len(remaining) will be 0.
func NextChunk(s []byte) (Chunk, []byte) {
	// NewlineIndex is not reused here so NextChunk can also be inlined.
	for chunkLen, inputLen := 0, len(s); chunkLen < inputLen; chunkLen++ {
		b := s[chunkLen]

		if isNewlineChar(b) {
			chunkLen++
			if b == '\r' && chunkLen < inputLen && s[chunkLen] == '\n' {
				chunkLen++
			}

			return Chunk{Data: s[:chunkLen], HasNewline: true}, s[chunkLen:]
		}
	}

	return Chunk{Data: s}, nil
}
