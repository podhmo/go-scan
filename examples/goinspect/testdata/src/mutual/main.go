package mutual

import "fmt"

// Ping is a mutually recursive function with Pong.
func Ping(n int) {
	if n > 1 {
		return
	}
	fmt.Println("ping", n)
	Pong(n + 1)
}

// Pong is a mutually recursive function with Ping.
func Pong(n int) {
	if n > 1 {
		return
	}
	fmt.Println("pong", n)
	Ping(n + 1)
}
