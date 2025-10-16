package toplevel

// Toplevel is the only function that should appear at the top level of the output.
func Toplevel() {
	calledFunction()
}

// calledFunction is called by Toplevel and should not be a top-level entry.
func calledFunction() {
	// no-op
}
