package mylib

import "fmt"

type Greeter interface {
	Hello()
}

type Person struct {
	Name string
}

func (p *Person) Hello() {
	Helper(p.Name)
}

type Dog struct {
	Name string
}

func (d *Dog) Hello() {
	Helper(d.Name)
}

func Helper(name string) {
	fmt.Printf("hello %s\n", name)
}
