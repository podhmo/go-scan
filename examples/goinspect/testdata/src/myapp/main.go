package main

import (
	"fmt"

	"github.com/podhmo/go-scan/examples/goinspect/testdata/src/another"
)

// Greeter is an interface for greeting.
type Greeter interface {
	Greet()
}

// Person represents a person.
type Person struct {
	Name string
}

// Greet prints a greeting from the person.
func (p *Person) Greet() {
	fmt.Printf("Hello, my name is %s\n", p.Name)
	another.Helper()
	privateFunc()
}

// String returns the string representation of a Person.
func (p *Person) String() string {
	return p.Name // Accessor
}

// Service provides some service.
type Service struct{}

// Do calls the greeter.
func (s *Service) Do(g Greeter) {
	g.Greet()
}

func main() {
	p := &Person{Name: "Alice"}
	s := &Service{}
	s.Do(p)
	Recursive(0)
}

func privateFunc() {
	fmt.Println("This is a private function.")
}

// Recursive is a recursive function.
func Recursive(n int) {
	if n > 2 {
		return
	}
	fmt.Println("Recursive call", n)
	Recursive(n + 1)
}
