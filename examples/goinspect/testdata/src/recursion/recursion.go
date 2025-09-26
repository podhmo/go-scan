package recursion

// Factorial calculates the factorial of a number recursively.
func Factorial(n int) int {
	if n <= 1 {
		return 1
	}
	return n * Factorial(n-1)
}

// Ping and Pong are mutually recursive functions.
func Ping(n int) {
	if n > 0 {
		Pong(n - 1)
	}
}

func Pong(n int) {
	if n > 0 {
		Ping(n - 1)
	}
}