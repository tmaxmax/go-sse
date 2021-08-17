package hub

func unboundedChannel(cap int, discardRemainingValues bool) (chan<- interface{}, <-chan interface{}) {
	in := make(chan interface{})
	out := make(chan interface{})
	queue := make([]interface{}, 0, cap)

	getOut := func() chan interface{} {
		if len(queue) == 0 {
			return nil
		}
		return out
	}
	// prevent index out of range error on empty queue
	getValue := func() interface{} {
		if len(queue) == 0 {
			return nil
		}
		return queue[0]
	}

	go func() {
		defer close(out)

	loop:
		for {
		mainSelect:
			select {
			case v, ok := <-in:
				if !ok {
					break loop
				}
				// try sending directly to receiver if queue is empty and receiver is ready
				if len(queue) == 0 {
					select {
					case out <- v:
						break mainSelect
					default:
						break
					}
				}
				queue = append(queue, v)
				// getValue returns nil, so we use getOut to return
				// a nil channel when the queue is empty in order to
				// not send nil values to the receiver.
			case getOut() <- getValue():
				queue = queue[1:]
			}
		}

		if discardRemainingValues {
			return
		}

		for _, v := range queue {
			out <- v
		}
	}()

	return in, out
}
