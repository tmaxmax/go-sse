package tests

import "time"

type Time struct {
	now   time.Time
	added time.Duration
}

func (t *Time) Now() time.Time {
	if t == nil || t.now.IsZero() {
		return time.Now()
	}

	return t.now.Add(t.added)
}

func (t *Time) Fixed() (time.Time, bool) {
	if t == nil || t.now.IsZero() {
		return time.Time{}, false
	}

	return t.now.Add(t.added), true
}

func (t *Time) Set(ts time.Time) {
	if t != nil {
		t.now = ts
		t.added = 0
	}
}

func (t *Time) Add(d time.Duration) {
	if t == nil {
		return
	}

	if t.now.IsZero() {
		t.now = time.Now()
	}

	t.added += d
}

func (t *Time) Reset() {
	t.Set(time.Time{})
}

func (t *Time) Rewind() {
	if t != nil {
		t.added = 0
	}
}
