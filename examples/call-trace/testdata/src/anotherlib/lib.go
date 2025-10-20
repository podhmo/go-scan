package anotherlib

import "fmt"

// Runner is an interface.
type Runner interface {
	Run()
}

// NewRunner creates a new runner.
func NewRunner() Runner {
	return &runner{}
}

type runner struct{}

func (r *runner) Run() {
	fmt.Println("runner is running")
}
