package main

// Good proves the package still compiled: functions without unsupported
// constructs run normally.
func Good() int { return 7 }

// Bad hits OpTrap at call time (fallthrough is not supported) — never at
// parse or compile time.
func Bad() int {
	switch 1 {
	case 1:
		fallthrough
	default:
	}
	return 0
}

// Channy traps on a type assertion (interfaces are deferred this phase).
func Channy() int {
	var x any = 1
	_ = x.(int)
	return 1
}

func main() {}
