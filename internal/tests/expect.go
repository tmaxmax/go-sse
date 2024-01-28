package tests

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

func Equal[T comparable](tb testing.TB, got, expected T, format string, args ...any) {
	tb.Helper()

	if got != expected {
		output := fmt.Sprintf(format, args...)
		switch v := any(got).(type) {
		case string:
			output += fmt.Sprintf("\nreceived: %q\nexpected: %q", v, any(expected).(string))
		case fmt.Stringer:
			output += fmt.Sprintf("\nreceived: %q\nexpected: %q", v.String(), any(expected).(fmt.Stringer).String())
		default:
			a, b := fmt.Sprintf("%v", got), fmt.Sprintf("%v", expected)
			if len(a) >= 20 || len(b) >= 20 {
				output += fmt.Sprintf("\nreceived: %s\nexpected: %s", a, b)
			} else {
				output += fmt.Sprintf("\nreceived %s, expected %s", a, b)
			}
		}
		tb.Fatal(output)
	}
}

func DeepEqual[T any](tb testing.TB, got, expected T, format string, args ...any) {
	tb.Helper()

	if !reflect.DeepEqual(got, expected) {
		output := fmt.Sprintf(format, args...)
		output += fmt.Sprintf("\nreceived: %+v\nexpected %+v", got, expected)
		tb.Fatal(output)
	}
}

func ErrorIs(tb testing.TB, got, expected error, format string, args ...any) {
	tb.Helper()

	if !errors.Is(got, expected) {
		output := fmt.Sprintf(format, args...)
		output += fmt.Sprintf("\nreceived: %#v\nexpected: %#v", got, expected)
		tb.Fatal(output)
	}
}

func Expect(tb testing.TB, cond bool, format string, args ...any) {
	tb.Helper()

	if !cond {
		tb.Fatalf(format, args...)
	}
}

func NotPanics(tb testing.TB, fn func(), format string, args ...any) {
	tb.Helper()

	defer func() {
		tb.Helper()

		if r := recover(); r != nil {
			tb.Fatalf(format+"\npanic: %+v", append(args, r)...)
		}
	}()

	fn()
}

func Panics(tb testing.TB, fn func(), format string, args ...any) (recovered any) {
	tb.Helper()

	defer func() {
		tb.Helper()

		if recovered = recover(); recovered == nil {
			tb.Fatalf(format, args...)
		}
	}()

	fn()

	return nil
}
