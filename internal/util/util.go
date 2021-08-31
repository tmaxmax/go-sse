package util

import (
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
