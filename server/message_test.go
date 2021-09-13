package server

import (
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tmaxmax/go-sse/internal/parser"
)

func TestNew(t *testing.T) {
	t.Parallel()

	e := Message{}
	e.AppendText("whatever", "input", "will\nbe\nchunked")
	e.AppendData([]byte("amazing"))
	e.SetRetry(30)
	e.SetRetry(time.Second)

	e.SetID(MustEventID("again"))
	e.SetID(MustEventID("lol"))

	require.Truef(t, e.SetName("whatever"), "name %q regarded as invalid", "whatever")
	require.Truef(t, e.SetName("x"), "name %q regarded as invalid", "x")
	require.Falsef(t, e.SetName("multi\nline"), "name %q regarded as invalid", "multi\nline")

	now := time.Now()
	e.SetExpiry(now)

	expected := Message{
		expiresAt: now,
		chunks: []chunk{
			{data: []byte("whatever")},
			{data: []byte("input")},
			{data: []byte("will\n"), endsInNewline: true},
			{data: []byte("be\n"), endsInNewline: true},
			{data: []byte("chunked")},
			{data: []byte("amazing")},
		},
		retryValue: []byte("1000\n"),
		name:       []byte("x"),
		id:         []byte("lol"),
	}

	require.Equal(t, expected, e, "invalid event")

	e.SetID(EventID{})

	require.Nil(t, e.id, "id was not unset")
}

func TestEvent_WriteTo(t *testing.T) {
	t.Parallel()

	e := Message{}
	e.AppendText("This is an example\nOf an event", "")
	e.AppendData([]byte("raw bytes here"))
	e.Comment("This test should pass")
	e.AppendText("Important data\nImportant again\r\rVery important\r\n")
	e.SetName("test_event")
	e.SetRetry(time.Second * 5)
	e.SetID(MustEventID("example_id"))

	output := "id: example_id\nevent: test_event\nretry: 5000\ndata: This is an example\ndata: Of an event\ndata: raw bytes here\n: This test should pass\ndata: Important data\ndata: Important again\rdata: \rdata: Very important\r\n\n"
	expectedWritten := int64(len(output))

	w := &strings.Builder{}

	written, _ := e.WriteTo(w)

	require.Equal(t, output, w.String(), "event written incorrectly")
	require.Equal(t, expectedWritten, written, "written byte count wrong")
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

	tests := []test{
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
			input: "data: raw bytes here\nretry: 500\nretry: 1000\nid: 1000\nid: 2000\n: no comments\ndata: again raw bytes\ndata: from multiple lines\nevent: overwritten name\nevent: my name here\n\ndata: I should be ignored",
			expected: Message{
				chunks: []chunk{
					{data: []byte("raw bytes here\n"), endsInNewline: true},
					{data: []byte("again raw bytes\n"), endsInNewline: true},
					{data: []byte("from multiple lines\n"), endsInNewline: true},
				},
				retryValue: []byte("1000\n"),
				name:       []byte("my name here"),
				id:         []byte("2000"),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			e := Message{}

			if err := e.UnmarshalText([]byte(test.input)); (test.expectedErr != nil && err.Error() != test.expectedErr.Error()) || (test.expectedErr == nil && err != nil) {
				t.Fatalf("Invalid unmarshal error: got %q, want %q", err, test.expectedErr)
			}
			require.Equal(t, test.expected, e, "invalid unmarshal")
		})
	}
}

func TestEvent_ID(t *testing.T) {
	e := Message{}
	require.Equal(t, EventID{}, e.ID(), "invalid default ID")
	id := MustEventID("idk")
	e.SetID(id)
	require.Equal(t, id, e.ID(), "invalid received ID")
}

func newBenchmarkEvent() *Message {
	e := Message{}
	e.AppendText("Example data\nWith multiple rows\r\nThis is interesting")
	e.Comment("An useless comment here that spans\non\n\nmultiple\nlines")
	e.SetName("This is the event's name")
	e.SetID(MustEventID("example_id"))
	e.SetRetry(time.Minute)
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
	ev.AppendText(benchmarkText...)

	for n := 0; n < b.N; n++ {
		_, _ = ev.WriteTo(io.Discard)
	}
}
