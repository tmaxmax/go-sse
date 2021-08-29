package parser

import (
	"bufio"
	"reflect"
	"strings"
	"testing"

	"github.com/tmaxmax/go-sse/internal/util"
)

func TestSplitFunc(t *testing.T) {
	t.Parallel()

	text := "mama mea e super\nce genial\nsincer n-am ce sa zic\r\n\n\nmama tata bunica bunicul\nsarmale\r\n\raualeu\nce taraboi"
	r := strings.NewReader(text)
	s := bufio.NewScanner(r)
	s.Split(splitFunc)

	expected := []string{
		util.EscapeNewlines("mama mea e super\nce genial\nsincer n-am ce sa zic\r\n\n"),
		util.EscapeNewlines("mama tata bunica bunicul\nsarmale\r\n\r"),
		util.EscapeNewlines("aualeu\nce taraboi"),
	}
	tokens := make([]string, 0, len(expected))

	for s.Scan() {
		tokens = append(tokens, util.EscapeNewlines(s.Text()))
	}

	if s.Err() != nil {
		t.Fatalf("an error occured: %v", s.Err())
	}

	if !reflect.DeepEqual(tokens, expected) {
		t.Fatalf("wrong tokens:\nreceived: %#v\nexpected: %#v", tokens, expected)
	}
}
