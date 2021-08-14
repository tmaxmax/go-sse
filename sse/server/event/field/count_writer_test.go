package field

import (
	"io"
	"testing"
)

func TestCountWriter(t *testing.T) {
	t.Parallel()

	chunks := []string{"sarmale", "cu", "ghimbir"}
	expectedCount := 0
	cw := &countWriter{w: io.Discard}

	for _, chunk := range chunks {
		l := len(chunk)
		expectedCount += l

		_, _ = cw.Write([]byte(chunk))
	}

	if expectedCount != cw.count {
		t.Fatalf("Counting written bytes failed: expected %d, got %d", expectedCount, cw.count)
	}
}
