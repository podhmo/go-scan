// Package sampleapi provides a sample net/http API for the docgen tool to analyze.
package sampleapi

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// User represents a user in the system.
type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// listUsers handles the GET /users endpoint.
// It returns a list of all users.
// It accepts 'limit' and 'offset' query parameters.
func listUsers(w http.ResponseWriter, r *http.Request) {
	_ = r.URL.Query().Get("limit")
	_ = r.URL.Query().Get("offset")

	users := []User{
		{ID: 1, Name: "Alice"},
		{ID: 2, Name: "Bob"},
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(users)
}

// getUser handles the GET /user endpoint.
// It returns a single user by ID.
func getUser(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, _ := strconv.Atoi(idStr) // error handling omitted for simplicity

	// In a real app, you'd fetch the user from a database.
	user := User{ID: id, Name: "Found User"}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(user)
}

// createUser handles the POST /users endpoint.
// It creates a new user.
func createUser(w http.ResponseWriter, r *http.Request) {
	var user User
	_ = json.NewDecoder(r.Body).Decode(&user) // simplified for example
	user.ID = 3                               // dummy ID
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(user)
}

// NewServeMux creates a new http.ServeMux and registers all the handlers.
func NewServeMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /users", listUsers)
	mux.HandleFunc("GET /user", getUser)
	mux.HandleFunc("POST /users", createUser)
	return mux
}
