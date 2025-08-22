package main

import (
	"net/http"

	"my-test-module/helpers"
)

type User struct {
	Name string `json:"name"`
}

// listUsers is a simple http handler that uses our custom response helper.
// The docgen tool should be able to identify that `helpers.RespondJSON`
// is being called and use the custom pattern to generate the OpenAPI response.
func listUsers(w http.ResponseWriter, r *http.Request) {
	users := []User{{Name: "foo"}, {Name: "bar"}}
	helpers.RespondJSON(w, users)
}

func main() {
	http.HandleFunc("/users", listUsers)
	http.ListenAndServe(":8080", nil)
}
