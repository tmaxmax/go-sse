package parser

import (
	"bufio"
	"reflect"
	"strings"
	"testing"
)

func TestSplitFunc(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name     string
		input    string
		expected []string
	}

	longString := strings.Repeat("abcdef\rghijklmn\nopqrstu\r\nvwxyz", 193)
	testCases := []testCase{
		{
			name:  "Short sample",
			input: "mama mea e super\nce genial\nsincer n-am ce sa zic\r\n\r\n\nmama tata bunica bunicul\nsarmale\r\n\r\r\naualeu\nce taraboi",
			expected: []string{
				"mama mea e super\nce genial\nsincer n-am ce sa zic\r\n\r\n",
				"mama tata bunica bunicul\nsarmale\r\n\r",
				"aualeu\nce taraboi",
			},
		},
		{
			name:  "Long sample",
			input: longString + "\n\n" + longString + "\r\r" + longString + "\r\n\r\n" + longString,
			expected: []string{
				longString + "\n\n",
				longString + "\r\r",
				longString + "\r\n\r\n",
				longString,
			},
		},
	}

	for _, tc := range testCases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := strings.NewReader(tc.input)
			s := bufio.NewScanner(r)
			s.Split(splitFunc)

			tokens := make([]string, 0, len(tc.expected))

			for s.Scan() {
				tokens = append(tokens, s.Text())
			}

			if s.Err() != nil {
				t.Fatalf("an error occurred: %v", s.Err())
			}

			if !reflect.DeepEqual(tokens, tc.expected) {
				t.Fatalf("wrong tokens:\nreceived: %#v\nexpected: %#v", tokens, tc.expected)
			}
		})
	}
}
