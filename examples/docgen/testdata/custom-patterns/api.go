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
	mux.HandleFunc("GET /pets/{petID}", GetPet)
	http.ListenAndServe(":8080", mux)
}

// GetPet is a handler that uses a custom helper to get a path parameter.
func GetPet(w http.ResponseWriter, r *http.Request) {
	_ = GetPetID(r) // The important part for the analyzer
	w.Write([]byte("ok"))
}

// GetPetID is a custom helper function to extract a path parameter.
func GetPetID(r *http.Request) string {
	// In a real app, this would parse the URL.
	return "pet-id"
}
