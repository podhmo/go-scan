package a

func F(s S) {
	log()
	F0()
	H()
}

func F0() {
	log()
	F1()
}

func F1() {
	H()
}

func G() {
	// G calls nothing
}

func H() {
	// H calls nothing, but is called by F and F1
}

func log() func() {
	return func() {}
}

type S struct{}

func (s S) M() {
	F()
}

// Recursive function
func Recur(n int) {
	if n > 0 {
		Recur(n - 1)
	}
}