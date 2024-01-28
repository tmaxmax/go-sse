package sse

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/tmaxmax/go-sse/internal/parser"
	"github.com/tmaxmax/go-sse/internal/tests"
)

func TestNew(t *testing.T) {
	t.Parallel()

	e := Message{Type: Type("x"), ID: ID("lol"), Retry: time.Second}
	e.AppendData("whatever", "input", "will\nbe\nchunked", "amazing")

	expected := Message{
		chunks: []chunk{
			{content: "whatever"},
			{content: "input"},
			{content: "will"},
			{content: "be"},
			{content: "chunked"},
			{content: "amazing"},
		},
		Retry: time.Second,
		Type:  Type("x"),
		ID:    ID("lol"),
	}

	tests.DeepEqual(t, e, expected, "invalid event")
}

func TestEvent_WriteTo(t *testing.T) {
	t.Parallel()

	t.Run("Empty", func(t *testing.T) {
		e := &Message{}
		w := &strings.Builder{}
		n, _ := e.WriteTo(w)
		tests.Equal(t, n, 0, "bytes were written")
		tests.Equal(t, w.String(), "", "message should produce no output")
	})

	t.Run("Valid", func(t *testing.T) {
		e := &Message{Type: Type("test_event"), ID: ID("example_id"), Retry: time.Second * 5}
		e.AppendData("This is an example\nOf an event", "", "a string here")
		e.AppendComment("This test should pass")
		e.AppendData("Important data\nImportant again\r\rVery important\r\n")

		output := "id: example_id\nevent: test_event\nretry: 5000\ndata: This is an example\ndata: Of an event\ndata: a string here\n: This test should pass\ndata: Important data\ndata: Important again\ndata: \ndata: Very important\n\n"
		expectedWritten := int64(len(output))

		w := &strings.Builder{}

		written, _ := e.WriteTo(w)

		tests.Equal(t, w.String(), output, "event written incorrectly")
		tests.Equal(t, written, expectedWritten, "written byte count wrong")
	})

	type retryTest struct {
		expected string
		value    time.Duration
	}

	retryTests := []retryTest{
		{value: -1},
		{value: 0},
		{value: time.Microsecond},
		{value: time.Millisecond, expected: "retry: 1\n\n"},
	}
	for _, v := range retryTests {
		t.Run(fmt.Sprintf("Retry/%s", v.value), func(t *testing.T) {
			e := &Message{Retry: v.value}
			tests.Equal(t, e.String(), v.expected, "incorrect output")
		})
	}
}

func TestEvent_UnmarshalText(t *testing.T) {
	t.Parallel()

	type test struct {
		name        string
		input       string
		expectedErr error
		expected    Message
	}

	nilEvent := Message{}
	nilEvent.reset()

	tt := []test{
		{
			name:        "No input",
			expected:    nilEvent,
			expectedErr: &UnmarshalError{Reason: ErrUnexpectedEOF},
		},
		{
			name:     "Invalid retry field",
			input:    "retry: sigma male\n",
			expected: nilEvent,
			expectedErr: &UnmarshalError{
				FieldName:  string(parser.FieldNameRetry),
				FieldValue: "sigma male",
				Reason:     fmt.Errorf("contains character %q, which is not an ASCII digit", 's'),
			},
		},
		{
			name:        "Valid input, no final newline",
			input:       "data: first\ndata:second\ndata:third",
			expected:    nilEvent,
			expectedErr: &UnmarshalError{Reason: ErrUnexpectedEOF},
		},
		{
			name:  "Valid input",
			input: "data: raw bytes here\nretry: 500\nretry: 1000\nid: 1000\nid: 2000\nid: \x001\n: with comments\ndata: again raw bytes\ndata: from multiple lines\nevent: overwritten name\nevent: my name here\n\ndata: I should be ignored",
			expected: Message{
				chunks: []chunk{
					{content: "raw bytes here"},
					{content: "with comments", isComment: true},
					{content: "again raw bytes"},
					{content: "from multiple lines"},
				},
				Retry: time.Second,
				Type:  Type("my name here"),
				ID:    ID("2000"),
			},
		},
	}

	for _, test := range tt {
		t.Run(test.name, func(t *testing.T) {
			e := Message{}

			if err := e.UnmarshalText([]byte(test.input)); (test.expectedErr != nil && err.Error() != test.expectedErr.Error()) || (test.expectedErr == nil && err != nil) {
				t.Fatalf("Invalid unmarshal error: got %q, want %q", err, test.expectedErr)
			}
			tests.DeepEqual(t, e, test.expected, "invalid unmarshal")
		})
	}
}

//nolint:all
func Example_messageWriter() {
	e := Message{
		Type: Type("test"),
		ID:   ID("1"),
	}
	w := &strings.Builder{}

	bw := base64.NewEncoder(base64.StdEncoding, w)
	binary.Write(bw, binary.BigEndian, []byte{6, 9, 4, 2, 0})
	binary.Write(bw, binary.BigEndian, []byte("data from sensor"))
	bw.Close()
	w.WriteByte('\n') // Ensures that the data written above will be a distinct `data` field.

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(map[string]string{"hello": "world"})
	// Not necessary to add a newline here – json.Encoder.Encode adds a newline at the end.

	// io.CopyN(hex.NewEncoder(w), rand.Reader, 8)
	io.Copy(hex.NewEncoder(w), bytes.NewReader([]byte{5, 1, 6, 34, 234, 12, 143, 91}))

	mw := io.MultiWriter(os.Stdout, w)
	// The first newline adds the data written above as a `data field`.
	io.WriteString(mw, "\nYou'll see me both in console and in event\n\n")

	// Add the data to the event. It will be split into fields here,
	// according to the newlines present in the input.
	e.AppendData(w.String())
	e.WriteTo(os.Stdout)
	// Output:
	// You'll see me both in console and in event
	//
	// id: 1
	// event: test
	// data: BgkEAgBkYXRhIGZyb20gc2Vuc29y
	// data: {
	// data:   "hello": "world"
	// data: }
	// data: 05010622ea0c8f5b
	// data: You'll see me both in console and in event
	// data:
}

func newBenchmarkEvent() *Message {
	e := Message{Type: Type("This is the event's name"), ID: ID("example_id"), Retry: time.Minute}
	e.AppendData("Example data\nWith multiple rows\r\nThis is interesting")
	e.AppendComment("An useless comment here that spans\non\n\nmultiple\nlines")
	return &e
}

var benchmarkEvent = newBenchmarkEvent()

func BenchmarkEvent_WriteTo(b *testing.B) {
	b.ReportAllocs()

	for n := 0; n < b.N; n++ {
		_, _ = benchmarkEvent.WriteTo(io.Discard)
	}
}

var benchmarkText = []string{
	"Lorem ipsum dolor sit amet, consectetur adipiscing elit.",
	"Pellentesque at dui non quam faucibus ultricies.",
	"Quisque non sem gravida, sodales lorem eget, lobortis est.",
	"Quisque porttitor nunc eu mollis congue.",
	"Vivamus sollicitudin tellus ut mi malesuada lacinia.",
	"Aenean aliquet tortor non urna sodales dignissim.",
	"Sed quis diam sed dui feugiat aliquam.",
	"Etiam sit amet neque cursus, semper nibh non, ornare nunc.",
	"Phasellus dignissim lacus vitae felis interdum, eget pharetra augue bibendum.",
	"Sed euismod enim sed ante laoreet, non ullamcorper enim dapibus.",
	"Ut accumsan arcu venenatis, egestas nisi consectetur, dignissim felis.",
	"Praesent lacinia elit ut tristique molestie.",
	"Mauris ut nibh id ante ultricies egestas.",
	"Mauris porttitor augue quis maximus efficitur.",
	"Fusce auctor enim viverra elit imperdiet, non dignissim dolor condimentum.",
	"Fusce scelerisque quam vel erat tempor elementum.",
	"Nullam ac velit in nisl hendrerit rhoncus sed ut dui.",
	"Pellentesque laoreet arcu vitae commodo gravida.",
	"Pellentesque sagittis enim quis sapien mollis tempor.",
	"Phasellus fermentum leo vitae odio efficitur, eu lacinia enim elementum.",
	"Morbi faucibus nisi a velit dictum eleifend.",
}

func BenchmarkEvent_WriteTo_text(b *testing.B) {
	ev := Message{}
	ev.AppendData(benchmarkText...)

	for n := 0; n < b.N; n++ {
		_, _ = ev.WriteTo(io.Discard)
	}
}
