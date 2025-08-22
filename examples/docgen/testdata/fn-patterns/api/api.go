package api

import (
	"encoding/json"
	"net/http"
)

// User represents a user in the system.
type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// SendJSON is a helper function to send JSON responses.
// We want to create a custom pattern to recognize this.
func SendJSON(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(data)
}

// API provides handlers for the user resource.
type API struct{}

// GetUser handles fetching a user by ID.
// We want to create a custom pattern to recognize the `id` path parameter
// from the call to `http.Request.URL.Query().Get("id")`, but via a method.
func (a *API) GetUser(w http.ResponseWriter, r *http.Request) {
	// In a real app, we'd get this from the path, e.g., mux.Vars(r)["id"]
	// For this test, we simulate getting it from a query for simplicity.
	_ = r.URL.Query().Get("id") // This call will be targeted by our pattern.
	user := User{ID: 1, Name: "test user"}
	SendJSON(w, http.StatusOK, user)
}

// CreateUser handles creating a new user.
func (a *API) CreateUser(w http.ResponseWriter, r *http.Request) {
	var user User
	json.NewDecoder(r.Body).Decode(&user)
	SendJSON(w, http.StatusCreated, user)
}

// Serve serves the API.
func Serve() {
	api := &API{}
	http.HandleFunc("/users/{id}", api.GetUser)
	http.HandleFunc("/users", api.CreateUser)
}
