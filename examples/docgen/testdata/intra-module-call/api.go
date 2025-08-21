package main

import (
	"net/http"

	"example.com/docgen_intra_module/handlers"
)

// main is the entrypoint for the application.
// docgen will start its analysis from here.
func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /user", handlers.GetUserHandler)
	http.ListenAndServe(":8080", mux)
}
