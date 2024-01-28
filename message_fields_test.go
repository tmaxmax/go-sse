package sse

import (
	"testing"

	"github.com/tmaxmax/go-sse/internal/tests"
)

func mustMessageField(tb testing.TB, value string) messageField { //nolint:unparam // May receive other values.
	tb.Helper()

	f, err := newMessageField(value)
	if err != nil {
		panic(err)
	}

	return f
}

func TestNewMessageField(t *testing.T) {
	t.Parallel()

	id, err := newMessageField("")
	tests.Equal(t, err, nil, "field evaluated as invalid")
	tests.Expect(t, id.IsSet(), "field is not set")
	tests.Equal(t, id.String(), "", "field incorrectly set")

	id, err = newMessageField("in\nvalid")
	tests.Expect(t, err != nil, "field evaluated as valid")
	tests.Expect(t, !id.IsSet() && id.String() == "", "field isn't unset")
}

func TestMessageField_UnmarshalJSON(t *testing.T) {
	t.Parallel()

	type test struct {
		name      string
		input     []byte
		output    messageField
		expectErr bool
	}

	tt := []test{
		{name: "Valid input", input: []byte("\"\""), output: mustMessageField(t, "")},
		{name: "Null input", input: []byte("null")},
		{name: "Invalid JSON value", input: []byte("525482"), expectErr: true},
		{name: "Invalid input", input: []byte("\"multi\\nline\""), expectErr: true},
	}

	for _, test := range tt {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			id := messageField{}
			err := id.UnmarshalJSON(test.input)

			if test.expectErr {
				tests.Expect(t, err != nil, "expected error")
			} else {
				tests.Equal(t, err, nil, "unexpected error")
			}

			tests.Equal(t, id, test.output, "unexpected unmarshal result")
		})
	}
}

func TestMessageField_UnmarshalText(t *testing.T) {
	t.Parallel()

	var id messageField
	err := id.UnmarshalText([]byte(""))

	tests.Equal(t, id, mustMessageField(t, ""), "unexpected unmarshal result")
	tests.Equal(t, err, nil, "unexpected error")

	err = id.UnmarshalText([]byte("in\nvalid"))

	tests.Expect(t, err != nil, "expected error")
	tests.Expect(t, !id.IsSet() && id.String() == "", "ID is not unset after invalid unmarshal")
}

func TestMessageField_MarshalJSON(t *testing.T) {
	t.Parallel()

	var id messageField
	v, err := id.MarshalJSON()

	tests.Equal(t, err, nil, "unexpected error")
	tests.Equal(t, string(v), "null", "invalid JSON result")

	id = mustMessageField(t, "")
	v, err = id.MarshalJSON()

	tests.Equal(t, err, nil, "unexpected error")
	tests.Equal(t, string(v), "\"\"", "invalid JSON result")
}

func TestMessageField_MarshalText(t *testing.T) {
	t.Parallel()

	var id messageField
	v, err := id.MarshalText()

	tests.Expect(t, err != nil, "expected error")
	tests.DeepEqual(t, v, nil, "invalid result")

	id = mustMessageField(t, "")
	v, err = id.MarshalText()

	tests.Equal(t, err, nil, "unexpected error")
	tests.DeepEqual(t, v, []byte{}, "unexpected result")
}

func TestMessageField_Scan(t *testing.T) {
	t.Parallel()

	var id messageField

	err := id.Scan(nil)
	tests.Equal(t, err, nil, "unexpected error")
	tests.Equal(t, id, messageField{}, "unexpected result")

	err = id.Scan("")
	tests.Equal(t, err, nil, "unexpected error")
	tests.Equal(t, id, mustMessageField(t, ""), "unexpected result")

	err = id.Scan([]byte(""))
	tests.Equal(t, err, nil, "unexpected error")
	tests.Equal(t, id, mustMessageField(t, ""), "unexpected result")

	err = id.Scan(5)
	tests.Expect(t, err != nil, "expected error")
	tests.Equal(t, id, messageField{}, "invalid result")
}

func TestMessageField_Value(t *testing.T) {
	t.Parallel()

	var id messageField
	v, err := id.Value()
	tests.Equal(t, err, nil, "unexpected error")
	tests.Equal(t, v, nil, "unexpected value")

	id = mustMessageField(t, "")
	v, err = id.Value()
	tests.Equal(t, err, nil, "unexpected error")
	tests.Equal(t, v, "", "unexpected value")
}

func TestFieldConstructors(t *testing.T) {
	t.Parallel()

	_, err := NewID("a\nb")
	tests.Equal(t, err.Error(), "invalid event ID: input is multiline", "unexpected error message")
	_, err = NewType("a\nb")
	tests.Equal(t, err.Error(), "invalid event type: input is multiline", "unexpected error message")

	tests.Panics(t, func() { ID("a\nb") }, "id creation should panic")
	tests.Panics(t, func() { Type("a\nb") }, "id creation should panic")
}
