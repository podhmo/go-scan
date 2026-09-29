package main

// DeferOrder runs defers LIFO on a named result: r becomes 321.
func DeferOrder() (r int) {
	defer func() { r = r*10 + 1 }()
	defer func() { r = r*10 + 2 }()
	defer func() { r = r*10 + 3 }()
	return 0
}

// DeferArgCaptured evaluates defer arguments at defer time, not call time:
// i is 1 when deferred, 99 when it fires — the captured 1 wins.
func DeferArgCaptured() (r int) {
	set := func(v int) { r = v }
	i := 1
	defer set(i)
	i = 99
	return 0
}

// NamedResultDefer mutates a named result in a deferred call.
func NamedResultDefer() (r int) {
	defer func() { r += 10 }()
	return 5
}

// Recovered recovers a panic inside a deferred function.
func Recovered() int {
	defer func() {
		if r := recover(); r != nil {
			// recovered: marker var proves we got here via RecoverAt
		}
	}()
	return 7
}

// RecoverValue captures the panic value via defer+recover.
func RecoverValue() (v any) {
	defer func() { v = recover() }()
	panic("boom")
}

// RecoverOutsideDefer returns nil (recover outside a deferred call).
func RecoverOutsideDefer() int {
	recover()
	return 1
}

// StillPanic re-panics from a defer: caller sees the panic.
func StillPanic() {
	defer func() { recover() }()
	panic("original")
}

// ReraiseReplace panics inside the deferred call itself.
func ReraiseReplace() {
	defer func() { panic("second") }()
	panic("first")
}

// ChanQueue: send then receive, FIFO.
func ChanQueue() int {
	ch := make(chan int)
	ch <- 1
	ch <- 2
	ch <- 3
	return <-ch + <-ch*10 + <-ch*100
}

// ChanCommaOk: receive with ok flag on a non-empty channel.
func ChanCommaOk() int {
	ch := make(chan int)
	ch <- 7
	v, ok := <-ch
	if ok && v == 7 {
		return 1
	}
	return 0
}

// ChanClosedRecv: receive on closed empty channel yields zero+false.
func ChanClosedRecv() int {
	ch := make(chan int)
	close(ch)
	v, ok := <-ch
	_ = v
	if ok {
		return 1
	}
	return 42
}

// ChanRange drains a channel after synchronous sends.
func ChanRange() int {
	ch := make(chan int)
	ch <- 10
	ch <- 20
	ch <- 30
	sum := 0
	for v := range ch {
		sum += v
	}
	return sum
}

// GoSync runs `go f()` synchronously: the side effect is visible at once.
func GoSync() int {
	x := 0
	add := func(n int) { x += n }
	go add(5)
	go add(7)
	return x
}

// GoChanRoundtrip approximates the classic goroutine+channel pattern
// synchronously: send then receive.
func GoChanRoundtrip() int {
	ch := make(chan int)
	go func() { ch <- 42 }()
	return <-ch
}

// SelectRecv picks the first ready receive case.
func SelectRecv() int {
	ch := make(chan int)
	ch <- 5
	r := 0
	select {
	case v := <-ch:
		r = v
	}
	return r
}

// SelectDefault falls to default when no case is ready.
func SelectDefault() int {
	ch := make(chan int)
	r := 0
	select {
	case v := <-ch:
		r = v
	default:
		r = 9
	}
	return r
}

// SelectCommaOk binds (v, ok) from a closed channel.
func SelectCommaOk() int {
	ch := make(chan int)
	close(ch)
	select {
	case _, ok := <-ch:
		if !ok {
			return 3
		}
	}
	return 0
}

// SelectSend: a send case is ready when the channel is open.
func SelectSend() int {
	ch := make(chan int)
	select {
	case ch <- 11:
	default:
		return 0
	}
	return <-ch
}

func main() {}
