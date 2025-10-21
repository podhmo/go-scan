package infra

import "fmt"

type repository struct{}

func NewRepository() *repository {
	return &repository{}
}

func (r *repository) Save(data string) {
	// This is the target function we want to trace
	Helper(data)
}

func Helper(data string) {
	fmt.Println("saving", data)
}
