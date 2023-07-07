package parser

import (
	"bufio"
	"reflect"
	"strings"
	"testing"
)

func TestSplitFunc(t *testing.T) {
	t.Parallel()

	text := "\xEF\xBB\xBFmama mea e super\nce genial\nsincer n-am ce sa zic\r\n\r\n\nmama tata bunica bunicul\nsarmale\r\n\r\r\naualeu\nce taraboi"
	r := strings.NewReader(text)
	s := bufio.NewScanner(r)
	s.Split(newSplitFunc())

	expected := []string{
		"mama mea e super\nce genial\nsincer n-am ce sa zic\r\n\r\n",
		"mama tata bunica bunicul\nsarmale\r\n\r",
		"aualeu\nce taraboi",
	}
	tokens := make([]string, 0, len(expected))

	for s.Scan() {
		tokens = append(tokens, s.Text())
	}

	if s.Err() != nil {
		t.Fatalf("an error occurred: %v", s.Err())
	}

	if !reflect.DeepEqual(tokens, expected) {
		t.Fatalf("wrong tokens:\nreceived: %#v\nexpected: %#v", tokens, expected)
	}
}

func TestSplitFuncWithLongLine(t *testing.T) {
	t.Parallel()

	longString := strings.Repeat("abcdef\rghijklmn\nopqrstu\r\nvwxyz", 193)
	text := longString + "\n\n" + longString + "\r\r" + longString + "\r\n\r\n" + longString
	r := strings.NewReader(text)
	s := bufio.NewScanner(r)
	s.Split(newSplitFunc())

	expected := []string{
		longString + "\n\n",
		longString + "\r\r",
		longString + "\r\n\r\n",
		longString,
	}
	tokens := make([]string, 0, len(expected))

	for s.Scan() {
		tokens = append(tokens, s.Text())
	}

	if s.Err() != nil {
		t.Fatalf("an error occurred: %v", s.Err())
	}

	if !reflect.DeepEqual(tokens, expected) {
		t.Fatalf("wrong tokens:\nreceived: %#v\nexpected: %#v", tokens, expected)
	}
}
