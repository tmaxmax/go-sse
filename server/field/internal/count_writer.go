package internal

import "io"

// CountWriter is a proxy that counts the bytes written to the wrapped writer.
type CountWriter struct {
	Count  int
	Writer io.Writer
}

func (c *CountWriter) Write(b []byte) (int, error) {
	n, err := c.Writer.Write(b)
	c.Count += n

	return n, err
}
