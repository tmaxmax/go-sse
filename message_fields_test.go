package sse

import (
	"testing"

	"github.com/stretchr/testify/require"
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
	require.NoError(t, err, "field evaluated as invalid")
	require.True(t, id.IsSet(), "field is not set")
	require.Equal(t, "", id.String(), "field incorrectly set")

	id, err = newMessageField("in\nvalid")
	require.Error(t, err, "field evaluated as valid")
	require.Empty(t, id, "field isn't unset")
}

func TestMessageField_UnmarshalJSON(t *testing.T) {
	t.Parallel()

	type test struct {
		name      string
		input     []byte
		output    messageField
		expectErr bool
	}

	tests := []test{
		{name: "Valid input", input: []byte("\"\""), output: mustMessageField(t, "")},
		{name: "Null input", input: []byte("null")},
		{name: "Invalid JSON value", input: []byte("525482"), expectErr: true},
		{name: "Invalid input", input: []byte("\"multi\\nline\""), expectErr: true},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			id := messageField{}
			err := id.UnmarshalJSON(test.input)

			if test.expectErr {
				require.Error(t, err, "expected error")
			} else {
				require.NoError(t, err, "unexpected error")
			}

			require.Equal(t, test.output, id, "unexpected unmarshal result")
		})
	}
}

func TestMessageField_UnmarshalText(t *testing.T) {
	t.Parallel()

	var id messageField
	err := id.UnmarshalText([]byte(""))

	require.Equal(t, mustMessageField(t, ""), id, "unexpected unmarshal result")
	require.NoError(t, err, "unexpected error")

	err = id.UnmarshalText([]byte("in\nvalid"))

	require.Error(t, err, "expected error")
	require.Empty(t, id, "ID is not unset after invalid unmarshal")
}

func TestMessageField_MarshalJSON(t *testing.T) {
	t.Parallel()

	var id messageField
	v, err := id.MarshalJSON()

	require.NoError(t, err, "unexpected error")
	require.Equal(t, "null", string(v), "invalid JSON result")

	id = mustMessageField(t, "")
	v, err = id.MarshalJSON()

	require.NoError(t, err, "unexpected error")
	require.Equal(t, "\"\"", string(v), "invalid JSON result")
}

func TestMessageField_MarshalText(t *testing.T) {
	t.Parallel()

	var id messageField
	v, err := id.MarshalText()

	require.Error(t, err, "expected error")
	require.Nil(t, v, "invalid result")

	id = mustMessageField(t, "")
	v, err = id.MarshalText()

	require.NoError(t, err, "unexpected error")
	require.Equal(t, []byte{}, v, "unexpected result")
}

func TestMessageField_Scan(t *testing.T) {
	t.Parallel()

	var id messageField

	err := id.Scan(nil)
	require.NoError(t, err, "unexpected error")
	require.Empty(t, id, "unexpected result")

	err = id.Scan("")
	require.NoError(t, err, "unexpected error")
	require.Equal(t, mustMessageField(t, ""), id, "unexpected result")

	err = id.Scan([]byte(""))
	require.NoError(t, err, "unexpected error")
	require.Equal(t, mustMessageField(t, ""), id, "unexpected result")

	err = id.Scan(5)
	require.Error(t, err, "expected error")
	require.Empty(t, id, "invalid result")
}

func TestMessageField_Value(t *testing.T) {
	t.Parallel()

	var id messageField
	v, err := id.Value()
	require.NoError(t, err, "unexpected error")
	require.Nil(t, v, "unexpected value")

	id = mustMessageField(t, "")
	v, err = id.Value()
	require.NoError(t, err, "unexpected error")
	require.Equal(t, "", v, "unexpected value")
}

func TestFieldConstructors(t *testing.T) {
	t.Parallel()

	_, err := NewID("a\nb")
	require.EqualError(t, err, "invalid event ID: input is multiline")
	_, err = NewType("a\nb")
	require.EqualError(t, err, "invalid event type: input is multiline")

	require.Panics(t, func() { ID("a\nb") })
	require.Panics(t, func() { Type("a\nb") })
}
