package util

import (
	"reflect"
	"strings"
	"unsafe"
)

func EscapeNewlines(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\n", "\\n"), "\r", "\\r")
}

func IsNewlineChar(b byte) bool {
	return b == '\n' || b == '\r'
}

func Bytes(s string) []byte {
	return (*[0x7fff0000]byte)(unsafe.Pointer(
		(*reflect.StringHeader)(unsafe.Pointer(&s)).Data),
	)[:len(s):len(s)]
}
