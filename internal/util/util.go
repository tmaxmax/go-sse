package util

import (
	"io"
	"strings"
)

// EscapeNewlines escapes the '\n' and '\r' characters with a backslash.
func EscapeNewlines(s string) string {
	return strings.NewReplacer("\n", "\\n", "\r", "\\r").Replace(s)
}

// IsNewlineChar returns whether the given character is '\n' or '\r'.
func IsNewlineChar(b byte) bool {
	return b == '\n' || b == '\r'
}

// CloneBytes creates a copy of the given byte slice.
func CloneBytes(p []byte) []byte {
	return append(make([]byte, 0, len(p)), p...)
}

type unprefixedReader struct {
	r      io.Reader
	prefix string
	buf    []byte
}

// RemovePrefix returns a reader that removes the given prefix from the input.
// If you want to remove a long prefix make sure you read using a buffer
// sized equally to or greater than the prefix, otherwise an allocation will be made.
// The function returns the given reader if the prefix is an empty string.
func RemovePrefix(r io.Reader, prefix string) io.Reader {
	if prefix == "" {
		return r
	}
	return &unprefixedReader{
		r:      r,
		prefix: prefix,
	}
}

func (n *unprefixedReader) Read(p []byte) (int, error) {
	var read int

	if n.prefix != "" {
		lenPrefix := len(n.prefix)
		lenP := len(p)

		var buf []byte
		if lenP < lenPrefix {
			buf = make([]byte, lenPrefix)
		} else {
			buf = p[:lenPrefix]
		}

		for read < lenPrefix {
			c, err := n.r.Read(buf[read:])
			read += c
			if err != nil {
				read = copy(p, buf[:read])
				n.prefix = ""
				return read, err
			}
		}

		if string(buf) == n.prefix {
			read = 0
		} else if lenP < lenPrefix {
			n.buf = buf
		} else {
			p = p[lenPrefix:]
		}

		n.prefix = ""
	}

	if len(n.buf) > 0 {
		read = copy(p, n.buf)
		n.buf = n.buf[read:]
		p = p[read:]
		if len(p) == 0 {
			return read, nil
		}
	}

	c, err := n.r.Read(p)
	return read + c, err
}
