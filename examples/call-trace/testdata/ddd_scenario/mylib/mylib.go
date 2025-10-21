package mylib

import "fmt"

type Repository interface {
	Get(id string) string
}

type ConcreteRepository struct{}

func (r *ConcreteRepository) Get(id string) string {
	return Helper(id)
}

func Helper(id string) string {
	return fmt.Sprintf("data for %s", id)
}
