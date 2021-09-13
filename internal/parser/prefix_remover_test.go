package parser_test

import (
	"io"
	"strings"
	"testing"

	"github.com/tmaxmax/go-sse/internal/parser"
)

func cp(tb testing.TB, dst io.Writer, src io.Reader, bufferSize int) (int, error) {
	tb.Helper()

	if bufferSize == 0 {
		n, err := io.Copy(dst, src)
		return int(n), err
	}

	n, err := io.CopyBuffer(dst, src, make([]byte, bufferSize))
	return int(n), err
}

func TestRemovePrefix(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name          string
		r             io.Reader
		prefix        string
		expected      string
		expectedCount int
		bufferSize    int
	}

	tests := []testCase{
		{
			name:          "With prefix, successful read",
			r:             strings.NewReader("helloworld"),
			prefix:        "hello",
			expected:      "world",
			expectedCount: 5,
		},
		{
			name:          "No prefix, successful read",
			r:             strings.NewReader("unmatched prefix"),
			prefix:        "matched",
			expected:      "unmatched prefix",
			expectedCount: 16,
		},
		{
			name:          "With prefix, error before full prefix read",
			r:             io.LimitReader(strings.NewReader("cool reader"), 3),
			prefix:        "cool",
			expected:      "coo",
			expectedCount: 3,
		},
		{
			name:          "With prefix, error after full prefix read",
			r:             io.LimitReader(strings.NewReader("cool reader"), 9),
			prefix:        "cool ",
			expected:      "read",
			expectedCount: 4,
		},
		{
			name:          "With prefix, small buffer",
			r:             strings.NewReader("long prefix small buffer"),
			prefix:        "long prefix ",
			expected:      "small buffer",
			expectedCount: 12,
			bufferSize:    4,
		},
		{
			name:          "No prefix, small buffer",
			r:             strings.NewReader("all output will be shown"),
			prefix:        "abracadabra",
			expected:      "all output will be shown",
			expectedCount: 24,
			bufferSize:    4,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			r := parser.RemovePrefix(test.r, test.prefix)
			w := strings.Builder{}
			n, err := cp(t, &w, r, test.bufferSize)

			if w.String() != test.expected {
				t.Fatalf("wrong output: expected %q, got %q", test.expected, w.String())
			}
			if n != test.expectedCount {
				t.Fatalf("wrong read count: expected %d, got %d", test.expectedCount, n)
			}
			if err != nil {
				t.Fatalf("received non-nil error: %v", err)
			}
		})
	}
}

var prefixBenchmarkText = `
Lorem ipsum dolor sit amet, consectetur adipiscing elit.
Morbi sit amet ante feugiat, sollicitudin leo iaculis, tincidunt sem.
Duis feugiat sem vel lobortis blandit.
Duis a sem lacinia, ultricies erat eu, pulvinar ligula.

Aliquam molestie erat mollis risus commodo, ac mollis lorem condimentum.
Donec quis dui ut tellus scelerisque auctor.
Vivamus ac est vel eros varius laoreet eget id metus.
Quisque nec nisl vitae augue posuere cursus eget fringilla augue.
Integer maximus ex eu nisl vulputate sagittis.

Maecenas at sapien faucibus, semper erat ac, tincidunt diam.
Praesent in diam sed ipsum eleifend facilisis ut et nisi.

Etiam accumsan mi a nulla fringilla ultricies.
Ut condimentum velit sed neque facilisis convallis.
Curabitur aliquam erat ut suscipit sagittis.
Vestibulum eget turpis lacinia, egestas eros eget, malesuada enim.
Etiam facilisis risus dictum, scelerisque ante non, porta turpis.

Sed luctus nibh et ante posuere placerat.
Nunc ullamcorper massa eu lectus condimentum eleifend.
Curabitur hendrerit tellus vitae nulla accumsan efficitur.

Donec at enim non est vestibulum scelerisque.
Nullam pharetra massa quis eros bibendum aliquet.
Quisque efficitur elit a tellus sollicitudin, ut ultricies nisl condimentum.
In auctor erat fringilla porttitor vestibulum.
Curabitur id felis sed velit condimentum ullamcorper non et lacus.
Mauris a nibh in metus vulputate fermentum.

Nullam aliquet purus quis dolor fermentum lobortis.
Maecenas a nulla ac est placerat pharetra nec quis magna.
Ut id tortor et tellus aliquet sollicitudin eu a odio.
Nam sit amet neque finibus, pellentesque neque at, eleifend ligula.

Sed vestibulum turpis non suscipit viverra.
Vestibulum et magna in elit sodales tincidunt sit amet sed tellus.
Suspendisse semper nunc sit amet dui ultricies, vitae efficitur enim pulvinar.
Sed id arcu et nisi tempus consectetur a sit amet magna.
Vivamus dictum lectus in maximus ullamcorper.

Ut at orci in sapien venenatis maximus.

Quisque vel odio eget nunc tempus tincidunt.
Morbi dapibus ex eget arcu varius, ac tristique lorem blandit.
Nam nec arcu nec magna viverra consequat dictum dapibus mauris.
Suspendisse quis lacus ac velit tempor pulvinar.
Maecenas sit amet ligula dictum, condimentum augue sed, elementum nisl.
`

var benchmarkPrefixMatching = `
Lorem ipsum dolor sit amet, consectetur adipiscing elit.
Morbi sit amet ante feugiat, sollicitudin leo iaculis, tincidunt sem.
Duis feugiat sem vel lobortis blandit.
Duis a sem lacinia, ultricies erat eu, pulvinar ligula.

Aliquam molestie erat mollis risus commodo, ac mollis lorem condimentum.
Donec quis dui ut tellus scelerisque auctor.
Vivamus ac est vel eros varius laoreet eget id metus.
Quisque nec nisl vitae augue posuere cursus eget fringilla augue.
Integer maximus ex eu nisl vulputate sagittis.
`

var benchmarkPrefixNotMatching = `
Etiam accumsan mi a nulla fringilla ultricies.
Ut condimentum velit sed neque facilisis convallis.
Curabitur aliquam erat ut suscipit sagittis.
Vestibulum eget turpis lacinia, egestas eros eget, malesuada enim.
Etiam facilisis risus dictum, scelerisque ante non, porta turpis.

Sed luctus nibh et ante posuere placerat.
Nunc ullamcorper massa eu lectus condimentum eleifend.
Curabitur hendrerit tellus vitae nulla accumsan efficitur.

Donec at enim non est vestibulum scelerisque.
`

func BenchmarkRemovePrefix(b *testing.B) {
	type benchmark struct {
		name       string
		prefix     string
		bufferSize int
	}

	type writerOnly struct {
		io.Writer
	}

	benchmarks := []benchmark{
		{name: "With prefix, big buffer", prefix: benchmarkPrefixMatching},
		{name: "No prefix, big buffer", prefix: benchmarkPrefixNotMatching},
		{name: "With prefix, small buffer", prefix: benchmarkPrefixMatching, bufferSize: 64},
		{name: "No prefix, small buffer", prefix: benchmarkPrefixNotMatching, bufferSize: 64},
		{name: "No removal"},
	}

	for _, benchmark := range benchmarks {
		w := io.Discard
		var buf []byte

		if benchmark.bufferSize > 0 {
			w = writerOnly{w}
			buf = make([]byte, benchmark.bufferSize)
		}

		b.Run(benchmark.name, func(b *testing.B) {
			b.ReportAllocs()

			for n := 0; n < b.N; n++ {
				r := parser.RemovePrefix(strings.NewReader(prefixBenchmarkText), benchmark.prefix)

				_, _ = io.CopyBuffer(w, r, buf)
			}
		})
	}
}
