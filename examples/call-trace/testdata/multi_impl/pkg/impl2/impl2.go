package impl2

import "fmt"

type speaker struct{}

func New() *speaker {
	return &speaker{}
}

func (s *speaker) Speak() {
	// target 2
	Bye()
}

func Bye() {
	fmt.Println("Bye")
}
