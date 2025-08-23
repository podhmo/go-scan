package main

import (
	"net/http"

	"example.com/const-resolution/consts"
	"example.com/const-resolution/query"
)

func GetUser(w http.ResponseWriter, r *http.Request) {
	_ = query.Get(r, consts.QueryParamName)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"id": "user123", "name": "John Doe"}`))
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/users", GetUser)
	http.ListenAndServe(":8080", mux)
}
