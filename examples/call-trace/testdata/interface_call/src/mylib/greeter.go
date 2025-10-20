package mylib

// Greeter is an interface for greeting.
type Greeter interface {
	Greet() string
}

// greeter is a concrete implementation of Greeter.
type greeter struct{}

// NewGreeter creates a new Greeter.
func NewGreeter() Greeter {
	return &greeter{}
}

// Greet returns a greeting message.
func (g *greeter) Greet() string {
	return "Hello, from implementation!"
}
