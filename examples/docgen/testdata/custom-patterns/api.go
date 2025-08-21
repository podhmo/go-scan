package main

import (
	"encoding/json"
	"net/http"
)

// User represents a user in the system.
type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// SendJSON is a custom helper to send a JSON response.
func SendJSON(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(data)
}

// GetUser handles retrieving a user.
func GetUser(w http.ResponseWriter, r *http.Request) {
	user := User{ID: 1, Name: "John Doe"}
	SendJSON(w, http.StatusOK, user)
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /users/{id}", GetUser)
	http.ListenAndServe(":8080", mux)
}
