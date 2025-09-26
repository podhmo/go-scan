package indirect

// cont is a helper function to create an indirect call.
func cont(f func(int), n int) {
	f(n)
}

// Ping is an indirectly mutually recursive function with Pong.
func Ping(n int) {
	if n > 1 {
		return
	}
	// Calls Pong via the cont helper
	cont(Pong, n+1)
}

// Pong is an indirectly mutually recursive function with Ping.
func Pong(n int) {
	if n > 1 {
		return
	}
	// Calls Ping via the cont helper
	cont(Ping, n+1)
}