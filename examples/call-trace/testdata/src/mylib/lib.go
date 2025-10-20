package mylib

import "fmt"

// Helper is a simple helper function.
func Helper() {
	fmt.Println("This is a helper function.")
}

// Greeter is a struct with a method.
type Greeter struct {
	Name string
}

// Greet prints a greeting.
func (g *Greeter) Greet() {
	fmt.Printf("Hello, %s!\n", g.Name)
}
