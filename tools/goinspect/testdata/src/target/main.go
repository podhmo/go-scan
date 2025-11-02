package target

func sharedFunc() {
	// A function called by both A and B.
}

// FuncA is one entry point.
func FuncA() {
	sharedFunc()
}

// FuncB is another entry point that should be ignored when -target=FuncA.
func FuncB() {
	sharedFunc()
}
