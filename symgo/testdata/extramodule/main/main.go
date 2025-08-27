package main

import (
	"errors"
)

func main() any {
	// This call should result in a SymbolicPlaceholder,
	// because errors.New is in an external (std lib) package
	// and is not a registered intrinsic.
	err := errors.New("a test error")
	return err
}
