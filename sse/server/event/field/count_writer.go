package field

import "io"

// countWriter is a proxy that counts the bytes written to the wrapped writer.
type countWriter struct {
	count int
	w     io.Writer
}

func (c *countWriter) Write(b []byte) (int, error) {
	n, err := c.w.Write(b)
	c.count += n

	return n, err
}
