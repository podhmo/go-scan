package main

import (
	"net/http"
	"new-features/helpers" // Use the correct module path
)

// User represents a user in the system.
type User struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Settings represents some configuration settings.
type Settings struct {
	Options map[string]any `json:"options"`
}

// GetSettingsHandler returns the current application settings.
// This handler is designed to test map[string]any support.
func GetSettingsHandler(w http.ResponseWriter, r *http.Request) {
	settings := Settings{
		Options: map[string]any{
			"feature_a": true,
			"retries":   3,
			"theme":     "dark",
		},
	}
	helpers.RenderJSON(w, http.StatusOK, settings)
}

// GetUserHandler returns a user by ID.
func GetUserHandler(w http.ResponseWriter, r *http.Request) {
	// This branch is for the error case, to be detected by our custom pattern.
	if r.URL.Query().Get("error") == "true" {
		helpers.RenderError(w, r, http.StatusNotFound, &helpers.ErrorResponse{Error: "User not found"})
		return
	}

	user := User{ID: "123", Name: "John Doe"}
	helpers.RenderJSON(w, http.StatusOK, user)
}

// CreateThingHandler is for testing custom status code responses.
func CreateThingHandler(w http.ResponseWriter, r *http.Request) {
	// some validation logic...
	if r.URL.Query().Get("fail") == "true" {
		helpers.RenderCustomError(w, r, helpers.ErrorResponse{Error: "Invalid input"})
		return
	}
	// Happy path not implemented for this example
}

// main is the entrypoint for the application.
// docgen will start its analysis from here.
func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /settings", GetSettingsHandler)
	mux.HandleFunc("GET /users/{id}", GetUserHandler) // Path parameter just for show
	mux.HandleFunc("POST /things", CreateThingHandler)
	http.ListenAndServe(":8080", mux)
}
