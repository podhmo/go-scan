package impl1

import "fmt"

type speaker struct{}

func New() *speaker {
	return &speaker{}
}

func (s *speaker) Speak() {
	// target 1
	Hello()
}

func Hello() {
	fmt.Println("Hello")
}
