package main

import (
	"github.com/google/uuid"
)

func main() string {
	// This call should result in a SymbolicPlaceholder,
	// because uuid.NewString is in an external module.
	return uuid.NewString()
}
