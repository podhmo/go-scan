// Package sampleapi provides a sample net/http API for the docgen tool to analyze.
package sampleapi

import (
	"encoding/json"
	"net/http"
)

// User represents a user in the system.
type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// listUsers handles the GET /users endpoint.
// It returns a list of all users.
func listUsers(w http.ResponseWriter, r *http.Request) {
	users := []User{
		{ID: 1, Name: "Alice"},
		{ID: 2, Name: "Bob"},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(users)
}

// RegisterHandlers registers all the handlers for this sample API.
func RegisterHandlers() {
	http.HandleFunc("/users", listUsers)
}
