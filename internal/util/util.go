package util

import (
	"io"
	"reflect"
	"strings"
	"unsafe"
)

func EscapeNewlines(s string) string {
	return strings.NewReplacer("\n", "\\n", "\r", "\\r").Replace(s)
}

func IsNewlineChar(b byte) bool {
	return b == '\n' || b == '\r'
}

func Bytes(s string) []byte {
	return (*[0x7fff0000]byte)(unsafe.Pointer(
		(*reflect.StringHeader)(unsafe.Pointer(&s)).Data),
	)[:len(s):len(s)]
}

type unprefixedReader struct {
	r      io.Reader
	prefix string
	buf    []byte
}

// RemovePrefix returns a reader that removes the given prefix from the input.
func RemovePrefix(r io.Reader, prefix string) io.Reader {
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
