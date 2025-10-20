package mylib

import "fmt"

func Helper() {
	fmt.Println("Hello from Helper")
}

type Greeter struct {
	Name string
}

func (g *Greeter) Greet() {
	fmt.Printf("Hello, %s\n", g.Name)
}
