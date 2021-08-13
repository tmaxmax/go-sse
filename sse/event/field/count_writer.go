package field

import "io"

type countWriter struct {
	count int
	w     io.Writer
}

func (c *countWriter) Write(b []byte) (n int, err error) {
	n, err = c.w.Write(b)
	c.count += n

	return
}
