package api

import "net/http"

type Foo struct {
	Name string
}

func GetFoo(w http.ResponseWriter, r *http.Request, foo Foo) {}
