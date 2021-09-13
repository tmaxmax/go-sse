package sse_test

import (
	"testing"

	"github.com/tmaxmax/go-sse"

	"github.com/stretchr/testify/require"
)

func TestNewID(t *testing.T) {
	t.Parallel()

	id, err := sse.NewEventID("")
	require.NoError(t, err, "ID deemed as invalid")
	require.True(t, id.IsSet(), "ID is not set")
	require.Equal(t, "", id.String(), "ID incorrectly set")

	id, err = sse.NewEventID("in\nvalid")
	require.Error(t, err, "ID deemed as valid")
	require.Empty(t, id, "ID isn't unset")
}

func TestMustID(t *testing.T) {
	t.Parallel()

	require.NotPanics(t, func() { sse.MustEventID("") }, "panicked on valid ID")
	require.Panics(t, func() { sse.MustEventID("in\nvalid") }, "no panic on invalid ID")
}

func TestID_UnmarshalJSON(t *testing.T) {
	t.Parallel()

	type test struct {
		name      string
		input     []byte
		output    sse.EventID
		expectErr bool
	}

	tests := []test{
		{name: "Valid input", input: []byte("\"\""), output: sse.MustEventID("")},
		{name: "Null input", input: []byte("null")},
		{name: "Invalid JSON value", input: []byte("525482"), expectErr: true},
		{name: "Invalid input", input: []byte("\"multi\\nline\""), expectErr: true},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			id := sse.EventID{}
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

func TestID_UnmarshalText(t *testing.T) {
	t.Parallel()

	var id sse.EventID
	err := id.UnmarshalText([]byte(""))

	require.Equal(t, sse.MustEventID(""), id, "unexpected unmarshal result")
	require.NoError(t, err, "unexpected error")

	err = id.UnmarshalText([]byte("in\nvalid"))

	require.Error(t, err, "expected error")
	require.Empty(t, id, "ID is not unset after invalid unmarshal")
}

func TestID_MarshalJSON(t *testing.T) {
	t.Parallel()

	var id sse.EventID
	v, err := id.MarshalJSON()

	require.NoError(t, err, "unexpected error")
	require.Equal(t, "null", string(v), "invalid JSON result")

	id = sse.MustEventID("")
	v, err = id.MarshalJSON()

	require.NoError(t, err, "unexpected error")
	require.Equal(t, "\"\"", string(v), "invalid JSON result")
}

func TestID_MarshalText(t *testing.T) {
	t.Parallel()

	var id sse.EventID
	v, err := id.MarshalText()

	require.ErrorIs(t, err, sse.ErrIDUnset, "invalid error")
	require.Nil(t, v, "invalid result")

	id = sse.MustEventID("")
	v, err = id.MarshalText()

	require.NoError(t, err, "unexpected error")
	require.Equal(t, []byte{}, v, "unexpected result")
}

func TestID_Scan(t *testing.T) {
	t.Parallel()

	var id sse.EventID

	err := id.Scan(nil)
	require.NoError(t, err, "unexpected error")
	require.Empty(t, id, "unexpected result")

	err = id.Scan("")
	require.NoError(t, err, "unexpected error")
	require.Equal(t, sse.MustEventID(""), id, "unexpected result")

	err = id.Scan([]byte(""))
	require.NoError(t, err, "unexpected error")
	require.Equal(t, sse.MustEventID(""), id, "unexpected result")

	err = id.Scan(5)
	require.Error(t, err, "expected error")
	require.Empty(t, id, "invalid result")
}

func TestID_Value(t *testing.T) {
	t.Parallel()

	var id sse.EventID
	v, err := id.Value()
	require.NoError(t, err, "unexpected error")
	require.Nil(t, v, "unexpected value")

	id = sse.MustEventID("")
	v, err = id.Value()
	require.NoError(t, err, "unexpected error")
	require.Equal(t, "", v, "unexpected value")
}
