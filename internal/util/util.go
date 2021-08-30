package util

import "strings"

func EscapeNewlines(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\n", "\\n"), "\r", "\\r")
}

func IsNewlineChar(b byte) bool {
	return b == '\n' || b == '\r'
}
