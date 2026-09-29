package main

// Good proves the package still compiled: functions without unsupported
// constructs run normally.
func Good() int { return 7 }

// Bad hits OpTrap at call time (defer is not supported yet) — never at
// parse or compile time.
func Bad() int {
	defer func() {}()
	return 0
}

// Channy traps on make(chan).
func Channy() int {
	ch := make(chan int)
	_ = ch
	return 1
}

func main() {}
