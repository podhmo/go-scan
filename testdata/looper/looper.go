package looper

// This file is designed to test the recursive dependency resolution in the symgo interpreter.
// The variable V depends on the function F for its type and value.
// The function F, in turn, depends on the variable V.
// This creates a circular dependency that can lead to infinite recursion if not handled correctly.

type T struct {
	Field int
}

// F must be declared before V for the single-pass interpreter to find it.
func F() T {
	// This line creates the dependency on V. To resolve F, we must know about V.
	_ = V.Field
	return T{Field: 42}
}

// V is a package-level variable whose value is determined by calling F.
// When the interpreter tries to determine the type of V, it must evaluate F.
var V = F()
