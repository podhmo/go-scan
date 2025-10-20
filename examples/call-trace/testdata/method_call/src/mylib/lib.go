package mylib

import "fmt"

type Greeter struct {
	Name string
}

func (g *Greeter) Greet() {
	fmt.Printf("Hello, %s\n", g.Name)
}
